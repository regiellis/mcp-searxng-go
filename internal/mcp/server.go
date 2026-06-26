package mcp

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	urlpkg "net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/regiellis/mcp-searxng-go/internal/cache"
	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/internal/fetch"
	"github.com/regiellis/mcp-searxng-go/internal/media"
	"github.com/regiellis/mcp-searxng-go/internal/search"
	"github.com/regiellis/mcp-searxng-go/internal/transcript"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Synthesizer composes a written answer from source text using an LLM. It is nil
// when no API key is configured, in which case answer_search with
// synthesize=true reports that synthesis is disabled.
type Synthesizer interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Model() string
}

// Server handles stdio and HTTP MCP requests.
type Server struct {
	cfg         config.Config
	search      *search.Client
	reader      *fetch.Reader
	media       *media.Runner
	jobs        *media.JobManager
	cleaner     *transcript.Cleaner
	synth       Synthesizer
	logger      *slog.Logger
	searchCache *cache.TTLCache[types.SearchResponse]
	readCache   *cache.TTLCache[types.URLReadResponse]
	sem         chan struct{}
}

// NewServer returns a configured MCP server. mediaRunner and cleaner may each be
// nil, in which case the tools depending on them are advertised but report that
// they are disabled.
func NewServer(cfg config.Config, searchClient *search.Client, reader *fetch.Reader, mediaRunner *media.Runner, cleaner *transcript.Cleaner, synth Synthesizer, logger *slog.Logger) *Server {
	s := &Server{
		cfg:         cfg,
		search:      searchClient,
		reader:      reader,
		media:       mediaRunner,
		cleaner:     cleaner,
		synth:       synth,
		logger:      logger,
		searchCache: cache.New[types.SearchResponse](cfg.Cache.MaxEntries),
		readCache:   cache.New[types.URLReadResponse](cfg.Cache.MaxEntries),
		sem:         make(chan struct{}, 8),
	}
	// Background-job support is only meaningful when the media tools exist.
	if mediaRunner != nil {
		s.jobs = media.NewJobManager(cfg.Media.Timeout, logger)
	}
	return s
}

// ServeStdio serves framed MCP messages over stdio.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	for {
		req, err := readFramedRequest(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			resp := responseError(nil, errParse, "failed to parse request", map[string]any{"detail": err.Error()})
			if writeErr := writeFramedResponse(writer, resp); writeErr != nil {
				return writeErr
			}
			continue
		}
		resp := s.handle(ctx, req)
		if err := writeFramedResponse(writer, resp); err != nil {
			return err
		}
	}
}

// HTTPHandler returns an HTTP transport for the MCP server.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		applyCORSHeaders(w)
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"name":            "mcp-searxng-go",
			"transport":       "http",
			"mcp_path":        "/mcp",
			"healthz":         "/healthz",
			"tools":           "/tools",
			"debug":           "/debug",
			"public_base_url": s.cfg.Server.PublicBaseURL,
		})
	})
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		applyCORSHeaders(w)
		writeJSON(w, http.StatusOK, map[string]any{"tools": toolDefinitions()})
	})
	mux.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		applyCORSHeaders(w)
		writeJSON(w, http.StatusOK, map[string]any{
			"name":            "mcp-searxng-go",
			"transport":       "http",
			"mcp_path":        "/mcp",
			"healthz":         "/healthz",
			"tools":           toolDefinitions(),
			"public_base_url": s.cfg.Server.PublicBaseURL,
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		applyCORSHeaders(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		applyCORSHeaders(w)
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{
				"name":            "mcp-searxng-go",
				"transport":       "http",
				"message":         "POST JSON-RPC requests to this endpoint",
				"methods":         []string{"initialize", "tools/list", "tools/call"},
				"tools_url":       "/tools",
				"public_base_url": s.cfg.Server.PublicBaseURL,
			})
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req types.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, responseError(nil, errParse, "failed to parse request", nil))
			return
		}
		writeJSON(w, http.StatusOK, s.handle(r.Context(), req))
	})
	return mux
}

func (s *Server) handle(ctx context.Context, req types.JSONRPCRequest) types.JSONRPCResponse {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return responseError(req.ID, errInvalidRequest, "jsonrpc must be 2.0", nil)
	}
	if req.Method == "" {
		return responseError(req.ID, errInvalidRequest, "method is required", nil)
	}

	switch req.Method {
	case "initialize":
		return types.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2025-03-26",
				"serverInfo": map[string]any{
					"name":    "mcp-searxng-go",
					"version": "0.1.0",
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
			},
		}
	case "tools/list":
		return types.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": toolDefinitions()},
		}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return responseError(req.ID, errMethodNotFound, "method not found", nil)
	}
}

func (s *Server) handleToolCall(ctx context.Context, req types.JSONRPCRequest) types.JSONRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return responseError(req.ID, errInvalidParams, "invalid tool params", map[string]any{"detail": err.Error()})
	}
	if params.Name == "" {
		return responseError(req.ID, errInvalidParams, "tool name is required", nil)
	}

	s.acquire()
	defer s.release()

	switch params.Name {
	case "web_search":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid web_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearch(ctx, "web_search", "general", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "image_search":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid image_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearch(ctx, "image_search", "images", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "video_search":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid video_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearch(ctx, "video_search", "videos", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "news_search":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid news_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearch(ctx, "news_search", "news", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "search_with_engines":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid search_with_engines arguments", map[string]any{"detail": err.Error()})
		}
		if len(input.Engines) == 0 {
			return responseError(req.ID, errInvalidParams, "engines is required", nil)
		}
		category := firstNonEmpty(input.Category, "general")
		result, err := s.runSearch(ctx, "search_with_engines", category, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "search_with_site_filter":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid search_with_site_filter arguments", map[string]any{"detail": err.Error()})
		}
		if strings.TrimSpace(input.Site) == "" {
			return responseError(req.ID, errInvalidParams, "site is required", nil)
		}
		category := firstNonEmpty(input.Category, "general")
		result, err := s.runSearch(ctx, "search_with_site_filter", category, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "multi_search":
		var input types.MultiSearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid multi_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runMultiSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "quick_look":
		var input types.QuickLookRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid quick_look arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runQuickLook(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "deep_research":
		var input types.DeepResearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid deep_research arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runDeepResearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "scholar_search":
		var input types.CategorySearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid scholar_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runScholarSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "local_search":
		var input types.CategorySearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid local_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runLocalSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "shopping_search":
		var input types.CategorySearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid shopping_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runShoppingSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "recent_search":
		var input types.SearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid recent_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runRecentSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "answer_search":
		var input types.AnswerSearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid answer_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runAnswerSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "compare_sources":
		var input types.CompareSourcesRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid compare_sources arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runCompareSources(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "fact_pack":
		var input types.FactPackRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid fact_pack arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runFactPack(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "monitor_query":
		var input types.MonitorQueryRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid monitor_query arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runMonitorQuery(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "search_then_extract":
		var input types.SearchThenExtractRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid search_then_extract arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearchThenExtract(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "search_then_rank":
		var input types.SearchThenRankRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid search_then_rank arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearchThenRank(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "image_quick_look":
		var input types.VisualQuickLookRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid image_quick_look arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runVisualQuickLook(ctx, "images", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "video_quick_look":
		var input types.VisualQuickLookRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid video_quick_look arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runVisualQuickLook(ctx, "videos", input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "find_official_docs":
		var input types.SearchPresetRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid find_official_docs arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runPresetRank(ctx, input, "official_docs")
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "find_latest_news":
		var input types.SearchPresetRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid find_latest_news arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runPresetRank(ctx, input, "latest_news")
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "find_examples":
		var input types.SearchPresetRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid find_examples arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runPresetRank(ctx, input, "examples")
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "find_primary_sources":
		var input types.SearchPresetRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid find_primary_sources arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runPresetRank(ctx, input, "primary_sources")
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "smart_search":
		var input types.SmartSearchRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid smart_search arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSmartSearch(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "search_and_read":
		var input types.SearchAndReadRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid search_and_read arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runSearchAndRead(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "download_video":
		var input types.DownloadVideoRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid download_video arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		return s.runMediaOp(ctx, req.ID, "download_video", input.Async, func(c context.Context) (any, error) {
			return s.media.Download(c, input)
		})
	case "transcode_media":
		var input types.TranscodeRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid transcode_media arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		return s.runMediaOp(ctx, req.ID, "transcode_media", input.Async, func(c context.Context) (any, error) {
			return s.media.Transcode(c, input)
		})
	case "download_subtitles":
		var input types.SubtitlesRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid download_subtitles arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		return s.runMediaOp(ctx, req.ID, "download_subtitles", input.Async, func(c context.Context) (any, error) {
			return s.media.Subtitles(c, input)
		})
	case "media_job_status":
		var input types.MediaJobStatusRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid media_job_status arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil || s.jobs == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		if strings.TrimSpace(input.JobID) == "" {
			return responseError(req.ID, errInvalidParams, "job_id is required", nil)
		}
		view, ok := s.jobs.Get(input.JobID)
		if !ok {
			return responseError(req.ID, errInvalidParams, "unknown job_id", map[string]any{"job_id": input.JobID})
		}
		return s.toolResult(req.ID, view)
	case "transcript_chapters":
		var input types.TranscriptChaptersRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid transcript_chapters arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runTranscriptChapters(ctx, input)
		if err != nil {
			return responseError(req.ID, errInternal, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "probe_media":
		var input types.ProbeMediaRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid probe_media arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		result, err := s.media.Probe(ctx, input)
		if err != nil {
			return responseError(req.ID, errInternal, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "read_media_file":
		var input types.ReadMediaFileRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid read_media_file arguments", map[string]any{"detail": err.Error()})
		}
		if s.media == nil {
			return responseError(req.ID, errInvalidRequest, "media tools are disabled", nil)
		}
		result, err := s.media.ReadFile(ctx, input)
		if err != nil {
			return responseError(req.ID, errInternal, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "clean_subtitles":
		var input types.CleanSubtitlesRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid clean_subtitles arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runCleanSubtitles(ctx, input)
		if err != nil {
			return responseError(req.ID, errInternal, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	case "url_read":
		var input types.URLReadRequest
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return responseError(req.ID, errInvalidParams, "invalid url_read arguments", map[string]any{"detail": err.Error()})
		}
		result, err := s.runRead(ctx, input)
		if err != nil {
			return responseError(req.ID, errInvalidParams, err.Error(), nil)
		}
		return s.toolResult(req.ID, result)
	default:
		return responseError(req.ID, errMethodNotFound, "unknown tool", nil)
	}
}

func (s *Server) runSearch(ctx context.Context, toolName, category string, req types.SearchRequest) (types.SearchResponse, error) {
	if !validCategory(category) {
		return types.SearchResponse{}, fmt.Errorf("unsupported category %q", category)
	}
	req.Category = category
	keyBytes, _ := json.Marshal(req)
	key := string(keyBytes)
	if s.cfg.Cache.Enabled {
		if cached, ok := s.searchCache.Get(key); ok {
			cached.Cached = true
			s.logger.Info("cache hit", "tool", toolName, "category", category)
			return cached, nil
		}
	}
	s.logger.Info("tool start", "tool", toolName, "category", category, "query", req.Query)
	resp, err := s.search.Search(ctx, req)
	if err != nil {
		s.logger.Error("tool failure", "tool", toolName, "category", category, "error", err)
		return types.SearchResponse{}, err
	}
	if s.cfg.Cache.Enabled {
		s.searchCache.Set(key, resp, s.cfg.Cache.TTLSearch)
	}
	s.logger.Info("tool end", "tool", toolName, "category", category, "count", resp.ResultCount)
	return resp, nil
}

func (s *Server) runMultiSearch(ctx context.Context, req types.MultiSearchRequest) (types.MultiSearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return types.MultiSearchResponse{}, fmt.Errorf("query is required")
	}
	ordered, results, err := s.searchCategories(ctx, "multi_search", req.Query, req.Categories, req.Language, req.TimeRange, req.Page, req.Limit)
	if err != nil {
		return types.MultiSearchResponse{}, err
	}
	return types.MultiSearchResponse{
		Query:      strings.TrimSpace(req.Query),
		Categories: ordered,
		Results:    results,
	}, nil
}

func (s *Server) runQuickLook(ctx context.Context, req types.QuickLookRequest) (types.QuickLookResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return types.QuickLookResponse{}, fmt.Errorf("query is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 3
	}
	ordered, results, err := s.searchCategories(ctx, "quick_look", req.Query, defaultCategories(req.Categories), req.Language, req.TimeRange, 1, limit)
	if err != nil {
		return types.QuickLookResponse{}, err
	}
	return types.QuickLookResponse{
		Query:      strings.TrimSpace(req.Query),
		Categories: ordered,
		Limit:      limit,
		Results:    results,
	}, nil
}

func (s *Server) runDeepResearch(ctx context.Context, req types.DeepResearchRequest) (types.DeepResearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return types.DeepResearchResponse{}, fmt.Errorf("query is required")
	}
	generalLimit := req.GeneralLimit
	if generalLimit <= 0 {
		generalLimit = 5
	}
	newsLimit := req.NewsLimit
	if newsLimit <= 0 {
		newsLimit = 3
	}
	maxSources := req.MaxSources
	if maxSources <= 0 {
		maxSources = 3
	}

	general, err := s.runSearch(ctx, "deep_research", "general", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Page:      1,
		Limit:     generalLimit,
	})
	if err != nil {
		return types.DeepResearchResponse{}, err
	}
	news, err := s.runSearch(ctx, "deep_research", "news", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Page:      1,
		Limit:     newsLimit,
	})
	if err != nil {
		return types.DeepResearchResponse{}, err
	}

	sources := make([]types.DeepResearchSource, 0, maxSources)
	seen := make(map[string]struct{}, maxSources)
	for _, result := range general.Results {
		if len(sources) >= maxSources {
			break
		}
		if _, ok := seen[result.URL]; ok {
			continue
		}
		seen[result.URL] = struct{}{}
		source := types.DeepResearchSource{Result: result}
		read, err := s.runRead(ctx, types.URLReadRequest{URL: result.URL})
		if err != nil {
			source.Error = err.Error()
		} else {
			source.Read = &read
		}
		sources = append(sources, source)
	}

	return types.DeepResearchResponse{
		Query:      strings.TrimSpace(req.Query),
		General:    general,
		News:       news,
		MaxSources: maxSources,
		Sources:    sources,
	}, nil
}

func (s *Server) runScholarSearch(ctx context.Context, req types.CategorySearchRequest) (types.MultiSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return types.MultiSearchResponse{}, fmt.Errorf("query is required")
	}
	query = query + " research paper journal preprint OR site:arxiv.org OR site:doi.org OR site:scholar.google.com"
	return s.runMultiSearch(ctx, types.MultiSearchRequest{
		Query:      query,
		Categories: pickCategories(req.Categories, []string{"general"}),
		Language:   req.Language,
		TimeRange:  req.TimeRange,
		Page:       req.Page,
		Limit:      req.Limit,
	})
}

func (s *Server) runLocalSearch(ctx context.Context, req types.CategorySearchRequest) (types.MultiSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return types.MultiSearchResponse{}, fmt.Errorf("query is required")
	}
	if location := strings.TrimSpace(req.Location); location != "" {
		query += " near " + location
	}
	query += " map hours address reviews"
	return s.runMultiSearch(ctx, types.MultiSearchRequest{
		Query:      query,
		Categories: pickCategories(req.Categories, []string{"general", "images"}),
		Language:   req.Language,
		TimeRange:  req.TimeRange,
		Page:       req.Page,
		Limit:      req.Limit,
	})
}

func (s *Server) runShoppingSearch(ctx context.Context, req types.CategorySearchRequest) (types.MultiSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return types.MultiSearchResponse{}, fmt.Errorf("query is required")
	}
	query += " buy price review compare"
	return s.runMultiSearch(ctx, types.MultiSearchRequest{
		Query:      query,
		Categories: pickCategories(req.Categories, []string{"general", "images"}),
		Language:   req.Language,
		TimeRange:  req.TimeRange,
		Page:       req.Page,
		Limit:      req.Limit,
	})
}

func (s *Server) runRecentSearch(ctx context.Context, req types.SearchRequest) (types.SearchResponse, error) {
	req.TimeRange = firstNonEmpty(req.TimeRange, "day")
	category := firstNonEmpty(req.Category, "general")
	return s.runSearch(ctx, "recent_search", category, req)
}

func (s *Server) runAnswerSearch(ctx context.Context, req types.AnswerSearchRequest) (types.AnswerSearchResponse, error) {
	searchResp, sources, err := s.searchAndCapture(ctx, "answer_search", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     req.Limit,
	}, req.ReadTopN)
	if err != nil {
		return types.AnswerSearchResponse{}, err
	}
	notes := notesOf(sources)
	maxSummary := req.MaxSummary
	if maxSummary <= 0 {
		maxSummary = 3
	}
	resp := types.AnswerSearchResponse{
		Query:    strings.TrimSpace(req.Query),
		Summary:  summarizeNotes(notes, maxSummary),
		Search:   searchResp,
		ReadTopN: normalizeReadCount(req.ReadTopN, 3),
		Sources:  notes,
	}

	// Synthesis is an explicit opt-in (see the LLM-opt-in rule): the default
	// response above is fully deterministic and needs no API key.
	if req.Synthesize {
		if s.synth == nil {
			return types.AnswerSearchResponse{}, fmt.Errorf("synthesis is disabled: set DEEPSEEK_API_KEY to enable synthesize=true")
		}
		s.logger.Info("tool start", "tool", "answer_search.synthesize", "sources", len(sources))
		answer, model, synthErr := s.synthesizeAnswer(ctx, req.Query, sources)
		if synthErr != nil {
			s.logger.Error("tool failure", "tool", "answer_search.synthesize", "error", synthErr)
			return types.AnswerSearchResponse{}, synthErr
		}
		resp.Answer = answer
		resp.AnswerModel = model
	}
	return resp, nil
}

// answerSystemPrompt instructs the model to answer strictly from the supplied
// sources with inline citations and no fabrication.
const answerSystemPrompt = `You are a research assistant. Answer the user's question using only the information in the provided numbered sources. Cite the sources you use inline with bracketed numbers like [1] or [2]. If the sources disagree, note the disagreement. If the sources do not contain enough information to answer, say so plainly rather than guessing. Do not invent facts, URLs, or citations. Respond with a clear, direct answer in plain prose with no preamble, headings, or markdown.`

// synthesizeAnswer composes a cited answer from the already-read sources. The
// bracketed citation numbers match the 1-based position of each source, which is
// the same order returned to the caller in Sources.
func (s *Server) synthesizeAnswer(ctx context.Context, query string, sources []readSource) (string, string, error) {
	if len(sources) == 0 {
		return "", "", fmt.Errorf("no sources available to synthesize an answer")
	}
	var b strings.Builder
	for i, src := range sources {
		body := strings.TrimSpace(src.content)
		if body == "" {
			body = strings.TrimSpace(src.note.Summary)
		}
		fmt.Fprintf(&b, "[%d] %s", i+1, strings.TrimSpace(src.note.Result.Title))
		if domain := src.note.Result.Domain; domain != "" {
			fmt.Fprintf(&b, " (%s)", domain)
		}
		if url := src.note.Result.URL; url != "" {
			fmt.Fprintf(&b, "\nURL: %s", url)
		}
		if body != "" {
			fmt.Fprintf(&b, "\n%s", body)
		}
		b.WriteString("\n\n")
	}
	user := "Question: " + strings.TrimSpace(query) + "\n\nSources:\n" + strings.TrimSpace(b.String())
	answer, err := s.synth.Complete(ctx, answerSystemPrompt, user)
	if err != nil {
		return "", "", fmt.Errorf("synthesize answer: %w", err)
	}
	return strings.TrimSpace(answer), s.synth.Model(), nil
}

func (s *Server) runCompareSources(ctx context.Context, req types.CompareSourcesRequest) (types.CompareSourcesResponse, error) {
	searchResp, notes, err := s.searchAndReadNotes(ctx, "compare_sources", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     req.Limit,
	}, req.ReadTopN)
	if err != nil {
		return types.CompareSourcesResponse{}, err
	}
	agreements, differences := compareSourceNotes(notes)
	return types.CompareSourcesResponse{
		Query:  strings.TrimSpace(req.Query),
		Search: searchResp,
		Comparison: types.SourceComparison{
			Agreements:    agreements,
			Differences:   differences,
			SourceNotes:   notes,
			ComparedCount: len(notes),
		},
	}, nil
}

func (s *Server) runFactPack(ctx context.Context, req types.FactPackRequest) (types.FactPackResponse, error) {
	quick, err := s.runQuickLook(ctx, types.QuickLookRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     3,
	})
	if err != nil {
		return types.FactPackResponse{}, err
	}
	_, notes, err := s.searchAndReadNotes(ctx, "fact_pack", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     maxInt(normalizeReadCount(req.ReadTopN, 3), 3),
	}, req.ReadTopN)
	if err != nil {
		return types.FactPackResponse{}, err
	}
	extracted := extractFactPack(notes, normalizeReadCount(req.QuoteLimit, 5))
	return types.FactPackResponse{
		Query:       strings.TrimSpace(req.Query),
		QuickLook:   quick,
		Sources:     notes,
		Extracted:   extracted,
		SourceCount: len(notes),
	}, nil
}

func (s *Server) runMonitorQuery(ctx context.Context, req types.MonitorQueryRequest) (types.MonitorQueryResponse, error) {
	ordered, results, err := s.searchCategories(ctx, "monitor_query", req.Query, req.Categories, req.Language, req.TimeRange, 1, req.Limit)
	if err != nil {
		return types.MonitorQueryResponse{}, err
	}
	monitors := make([]types.CategoryMonitor, 0, len(ordered))
	hashBuilder := strings.Builder{}
	for _, category := range ordered {
		resp := results[category]
		fp := fingerprintResults(resp.Results)
		monitors = append(monitors, types.CategoryMonitor{
			Category:    category,
			Fingerprint: fp,
			TopResults:  resp.Results,
		})
		hashBuilder.WriteString(category)
		hashBuilder.WriteString(":")
		hashBuilder.WriteString(fp)
		hashBuilder.WriteByte('|')
	}
	return types.MonitorQueryResponse{
		Query:       strings.TrimSpace(req.Query),
		Categories:  ordered,
		Fingerprint: hashString(hashBuilder.String()),
		Results:     monitors,
	}, nil
}

func (s *Server) runSearchThenExtract(ctx context.Context, req types.SearchThenExtractRequest) (types.SearchThenExtractResponse, error) {
	if len(req.Fields) == 0 {
		return types.SearchThenExtractResponse{}, fmt.Errorf("fields is required")
	}
	searchResp, notes, err := s.searchAndReadNotes(ctx, "search_then_extract", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     req.Limit,
	}, req.ReadTopN)
	if err != nil {
		return types.SearchThenExtractResponse{}, err
	}
	docs := make([]types.ExtractedDocument, 0, len(notes))
	for _, note := range notes {
		fields := extractFieldsFromText(note.Summary, req.Fields)
		docs = append(docs, types.ExtractedDocument{
			Result: note.Result,
			Fields: fields,
		})
	}
	return types.SearchThenExtractResponse{
		Query:     strings.TrimSpace(req.Query),
		Fields:    cleanList(req.Fields),
		Search:    searchResp,
		Documents: docs,
	}, nil
}

func (s *Server) runSearchThenRank(ctx context.Context, req types.SearchThenRankRequest) (types.SearchThenRankResponse, error) {
	searchResp, err := s.runSearch(ctx, "search_then_rank", "general", types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     req.Limit,
	})
	if err != nil {
		return types.SearchThenRankResponse{}, err
	}
	ranked := rankResults(searchResp.Results, req.Intent)
	return types.SearchThenRankResponse{
		Query:  strings.TrimSpace(req.Query),
		Intent: normalizeIntent(req.Intent),
		Search: searchResp,
		Ranked: ranked,
	}, nil
}

func (s *Server) runVisualQuickLook(ctx context.Context, category string, req types.VisualQuickLookRequest) (types.VisualQuickLookResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 6
	}
	searchResp, err := s.runSearch(ctx, category+"_quick_look", category, types.SearchRequest{
		Query:     req.Query,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Limit:     limit,
	})
	if err != nil {
		return types.VisualQuickLookResponse{}, err
	}
	return types.VisualQuickLookResponse{
		Query:    strings.TrimSpace(req.Query),
		Category: category,
		Limit:    limit,
		Results:  searchResp.Results,
	}, nil
}

func (s *Server) runPresetRank(ctx context.Context, req types.SearchPresetRequest, intent string) (types.SearchThenRankResponse, error) {
	timeRange := req.TimeRange
	if intent == "latest_news" && strings.TrimSpace(timeRange) == "" {
		timeRange = "day"
	}
	return s.runSearchThenRank(ctx, types.SearchThenRankRequest{
		Query:     req.Query,
		Intent:    intent,
		Language:  req.Language,
		TimeRange: timeRange,
		Limit:     req.Limit,
	})
}

func (s *Server) runSmartSearch(ctx context.Context, req types.SmartSearchRequest) (any, error) {
	switch strings.TrimSpace(req.Mode) {
	case "general", "images", "videos", "news":
		return s.runSearch(ctx, "smart_search", req.Mode, types.SearchRequest{
			Query:     req.Query,
			Language:  req.Language,
			TimeRange: req.TimeRange,
			Limit:     req.Limit,
		})
	case "quick_look":
		return s.runQuickLook(ctx, types.QuickLookRequest{
			Query:      req.Query,
			Categories: req.Categories,
			Language:   req.Language,
			TimeRange:  req.TimeRange,
			Limit:      req.Limit,
		})
	case "deep_research":
		return s.runDeepResearch(ctx, types.DeepResearchRequest{
			Query:      req.Query,
			Language:   req.Language,
			TimeRange:  req.TimeRange,
			MaxSources: req.MaxSources,
		})
	case "scholar":
		return s.runScholarSearch(ctx, types.CategorySearchRequest{
			Query:      req.Query,
			Categories: req.Categories,
			Language:   req.Language,
			TimeRange:  req.TimeRange,
			Limit:      req.Limit,
		})
	case "local":
		return s.runLocalSearch(ctx, types.CategorySearchRequest{
			Query:      req.Query,
			Categories: req.Categories,
			Language:   req.Language,
			TimeRange:  req.TimeRange,
			Limit:      req.Limit,
			Location:   req.Location,
		})
	case "shopping":
		return s.runShoppingSearch(ctx, types.CategorySearchRequest{
			Query:      req.Query,
			Categories: req.Categories,
			Language:   req.Language,
			TimeRange:  req.TimeRange,
			Limit:      req.Limit,
		})
	case "recent":
		return s.runRecentSearch(ctx, types.SearchRequest{
			Query:     req.Query,
			Language:  req.Language,
			TimeRange: req.TimeRange,
			Limit:     req.Limit,
		})
	case "extract":
		return s.runSearchThenExtract(ctx, types.SearchThenExtractRequest{
			Query:     req.Query,
			Fields:    req.Fields,
			Language:  req.Language,
			TimeRange: req.TimeRange,
			Limit:     req.Limit,
			ReadTopN:  req.ReadTopN,
		})
	case "rank":
		return s.runSearchThenRank(ctx, types.SearchThenRankRequest{
			Query:     req.Query,
			Intent:    req.Intent,
			Language:  req.Language,
			TimeRange: req.TimeRange,
			Limit:     req.Limit,
		})
	default:
		return nil, fmt.Errorf("unsupported smart_search mode %q", req.Mode)
	}
}

func (s *Server) runSearchAndRead(ctx context.Context, req types.SearchAndReadRequest) (types.SearchAndReadResponse, error) {
	category := firstNonEmpty(req.Category, "general")
	searchResp, err := s.runSearch(ctx, "search_and_read", category, types.SearchRequest{
		Query:     req.Query,
		Category:  category,
		Engines:   req.Engines,
		Site:      req.Site,
		Language:  req.Language,
		TimeRange: req.TimeRange,
		Page:      req.Page,
		Limit:     req.Limit,
	})
	if err != nil {
		return types.SearchAndReadResponse{}, err
	}
	index := req.ResultIndex
	if index < 0 {
		return types.SearchAndReadResponse{}, fmt.Errorf("result_index must be zero or positive")
	}
	if len(searchResp.Results) == 0 {
		return types.SearchAndReadResponse{Search: searchResp, SelectedIndex: index}, nil
	}
	if index >= len(searchResp.Results) {
		return types.SearchAndReadResponse{}, fmt.Errorf("result_index %d out of range", index)
	}
	selected := searchResp.Results[index]
	readResp, err := s.runRead(ctx, types.URLReadRequest{URL: selected.URL})
	if err != nil {
		return types.SearchAndReadResponse{}, err
	}
	return types.SearchAndReadResponse{
		Search:        searchResp,
		SelectedIndex: index,
		Selected:      &selected,
		Read:          &readResp,
	}, nil
}

func (s *Server) runRead(ctx context.Context, req types.URLReadRequest) (types.URLReadResponse, error) {
	key := req.URL
	if s.cfg.Cache.Enabled {
		if cached, ok := s.readCache.Get(key); ok {
			cached.Cached = true
			s.logger.Info("cache hit", "tool", "url_read")
			return cached, nil
		}
	}
	s.logger.Info("tool start", "tool", "url_read", "url", req.URL)
	resp, err := s.reader.Read(ctx, req)
	if err != nil {
		s.logger.Error("tool failure", "tool", "url_read", "error", err)
		return types.URLReadResponse{}, err
	}
	if s.cfg.Cache.Enabled {
		s.readCache.Set(key, resp, s.cfg.Cache.TTLURLRead)
	}
	s.logger.Info("tool end", "tool", "url_read", "status_code", resp.StatusCode)
	return resp, nil
}

// maxSubtitleBytes caps how much of a subtitle file is read before cleaning.
// Even multi-hour transcripts sit well under this; it is a guard, not a target.
const maxSubtitleBytes int64 = 8 << 20 // 8 MiB

// runCleanSubtitles reads a subtitle file from the media sandbox and runs it
// through the DeepSeek-backed cleaner, optionally saving the result alongside the
// source. Both the media runner (for file access) and the cleaner (for the LLM)
// must be configured.
func (s *Server) runCleanSubtitles(ctx context.Context, req types.CleanSubtitlesRequest) (types.CleanSubtitlesResponse, error) {
	if s.media == nil {
		return types.CleanSubtitlesResponse{}, fmt.Errorf("media tools are disabled")
	}
	if s.cleaner == nil {
		return types.CleanSubtitlesResponse{}, fmt.Errorf("transcript cleaning is disabled: set DEEPSEEK_API_KEY to enable it")
	}

	file, err := s.media.ReadFile(ctx, types.ReadMediaFileRequest{Path: req.Path, MaxBytes: maxSubtitleBytes})
	if err != nil {
		return types.CleanSubtitlesResponse{}, err
	}
	if file.Encoding != "text" {
		return types.CleanSubtitlesResponse{}, fmt.Errorf("%q is not a text subtitle file", req.Path)
	}
	if file.Truncated {
		s.logger.Warn("subtitle truncated before cleaning", "path", file.Path, "limit", maxSubtitleBytes)
	}

	s.logger.Info("tool start", "tool", "clean_subtitles", "path", file.Path, "chars", len(file.Content))
	result, err := s.cleaner.Clean(ctx, file.Content, req.Topic)
	if err != nil {
		s.logger.Error("tool failure", "tool", "clean_subtitles", "error", err)
		return types.CleanSubtitlesResponse{}, err
	}
	result.SourcePath = file.Path

	if req.Save {
		saved, saveErr := s.media.WriteDerived(req.Path, ".clean.txt", []byte(result.Content))
		if saveErr != nil {
			return types.CleanSubtitlesResponse{}, fmt.Errorf("save cleaned transcript: %w", saveErr)
		}
		result.SavedPath = saved.Path
	}

	s.logger.Info("tool end", "tool", "clean_subtitles", "chunks", result.Chunks, "input_chars", result.InputChars, "output_chars", result.OutputChars)
	return result, nil
}

// runTranscriptChapters reads a timestamped subtitle file from the media sandbox
// and segments it into time-bounded chapters by caption timing. It needs only
// the media runner for file access — no LLM — so it works without an API key.
func (s *Server) runTranscriptChapters(ctx context.Context, req types.TranscriptChaptersRequest) (types.TranscriptChaptersResponse, error) {
	if s.media == nil {
		return types.TranscriptChaptersResponse{}, fmt.Errorf("media tools are disabled")
	}

	file, err := s.media.ReadFile(ctx, types.ReadMediaFileRequest{Path: req.Path, MaxBytes: maxSubtitleBytes})
	if err != nil {
		return types.TranscriptChaptersResponse{}, err
	}
	if file.Encoding != "text" {
		return types.TranscriptChaptersResponse{}, fmt.Errorf("%q is not a text subtitle file", req.Path)
	}
	if file.Truncated {
		s.logger.Warn("subtitle truncated before chaptering", "path", file.Path, "limit", maxSubtitleBytes)
	}

	s.logger.Info("tool start", "tool", "transcript_chapters", "path", file.Path)
	result, err := transcript.Chapters(file.Content, transcript.ChapterOptions{
		MinSeconds: req.MinSeconds,
		GapSeconds: req.GapSeconds,
		MaxSeconds: req.MaxSeconds,
	})
	if err != nil {
		s.logger.Error("tool failure", "tool", "transcript_chapters", "error", err)
		return types.TranscriptChaptersResponse{}, err
	}
	result.SourcePath = file.Path

	s.logger.Info("tool end", "tool", "transcript_chapters", "cues", result.CueCount, "chapters", result.ChapterCount)
	return result, nil
}

// runMediaOp runs a media operation synchronously, or — when async is set —
// submits it as a background job and returns a job view to poll via
// media_job_status. The background job uses its own context, so it continues
// after this tool call returns.
func (s *Server) runMediaOp(ctx context.Context, id any, kind string, async bool, fn func(context.Context) (any, error)) types.JSONRPCResponse {
	if async {
		view, err := s.jobs.Submit(kind, fn)
		if err != nil {
			return responseError(id, errInternal, err.Error(), nil)
		}
		s.logger.Info("media job submitted", "tool", kind, "job_id", view.JobID)
		return s.toolResult(id, view)
	}
	result, err := fn(ctx)
	if err != nil {
		return responseError(id, errInternal, err.Error(), nil)
	}
	return s.toolResult(id, result)
}

func (s *Server) toolResult(id any, payload any) types.JSONRPCResponse {
	text, _ := json.Marshal(payload)
	return types.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": string(text),
				},
			},
			"structuredContent": payload,
		},
	}
}

func (s *Server) acquire() {
	s.sem <- struct{}{}
}

func (s *Server) release() {
	<-s.sem
}

func readFramedRequest(reader *bufio.Reader) (types.JSONRPCRequest, error) {
	length := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return types.JSONRPCRequest{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		const prefix = "Content-Length:"
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return types.JSONRPCRequest{}, err
			}
			length = parsed
		}
	}
	if length <= 0 {
		return types.JSONRPCRequest{}, fmt.Errorf("missing content length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return types.JSONRPCRequest{}, err
	}
	var req types.JSONRPCRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return types.JSONRPCRequest{}, err
	}
	return req, nil
}

func writeFramedResponse(writer *bufio.Writer, resp types.JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	return writer.Flush()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	applyCORSHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func applyCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, Mcp-Session-Id, Last-Event-ID, MCP-Protocol-Version")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	w.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
}

func validCategory(category string) bool {
	switch strings.TrimSpace(category) {
	case "general", "images", "videos", "news":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) searchCategories(ctx context.Context, toolName, query string, categories []string, language, timeRange string, page, limit int) ([]string, map[string]types.SearchResponse, error) {
	categories = defaultCategories(categories)
	results := make(map[string]types.SearchResponse, len(categories))
	seen := make(map[string]struct{}, len(categories))
	ordered := make([]string, 0, len(categories))
	for _, category := range categories {
		category = strings.TrimSpace(category)
		if !validCategory(category) {
			return nil, nil, fmt.Errorf("unsupported category %q", category)
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		ordered = append(ordered, category)
		resp, err := s.runSearch(ctx, toolName, category, types.SearchRequest{
			Query:     query,
			Category:  category,
			Language:  language,
			TimeRange: timeRange,
			Page:      page,
			Limit:     limit,
		})
		if err != nil {
			return nil, nil, err
		}
		results[category] = resp
	}
	return ordered, results, nil
}

func defaultCategories(categories []string) []string {
	if len(categories) == 0 {
		return []string{"general", "images", "videos", "news"}
	}
	return categories
}

func pickCategories(categories, fallback []string) []string {
	if len(categories) == 0 {
		return fallback
	}
	return categories
}

func normalizeReadCount(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

// synthSourceCapChars bounds how much of each read source is carried into an
// answer synthesis prompt, keeping requests within a reasonable size.
const synthSourceCapChars = 4000

// readSource pairs a source note with the (capped) read text behind it, so an
// optional synthesis step can use real content without re-fetching.
type readSource struct {
	note    types.SourceNote
	content string
}

// searchAndCapture searches, reads the top results, and returns both the compact
// notes and the capped read text per source. It is the shared engine behind the
// read-and-summarize tools.
func (s *Server) searchAndCapture(ctx context.Context, toolName string, req types.SearchRequest, readTopN int) (types.SearchResponse, []readSource, error) {
	searchResp, err := s.runSearch(ctx, toolName, firstNonEmpty(req.Category, "general"), req)
	if err != nil {
		return types.SearchResponse{}, nil, err
	}
	readTopN = normalizeReadCount(readTopN, 3)
	if readTopN > len(searchResp.Results) {
		readTopN = len(searchResp.Results)
	}
	sources := make([]readSource, 0, readTopN)
	for i := 0; i < readTopN; i++ {
		result := searchResp.Results[i]
		note := types.SourceNote{Result: result, Summary: compactResultSummary(result)}
		var content string
		readResp, err := s.runRead(ctx, types.URLReadRequest{URL: result.URL})
		if err == nil {
			note.Summary = compactReadSummary(result, readResp.Content)
			content = strings.TrimSpace(readResp.Content)
			if len(content) > synthSourceCapChars {
				content = content[:synthSourceCapChars]
			}
		}
		sources = append(sources, readSource{note: note, content: content})
	}
	return searchResp, sources, nil
}

// notesOf extracts just the source notes, discarding captured content.
func notesOf(sources []readSource) []types.SourceNote {
	notes := make([]types.SourceNote, len(sources))
	for i, src := range sources {
		notes[i] = src.note
	}
	return notes
}

func (s *Server) searchAndReadNotes(ctx context.Context, toolName string, req types.SearchRequest, readTopN int) (types.SearchResponse, []types.SourceNote, error) {
	searchResp, sources, err := s.searchAndCapture(ctx, toolName, req, readTopN)
	if err != nil {
		return types.SearchResponse{}, nil, err
	}
	return searchResp, notesOf(sources), nil
}

func summarizeNotes(notes []types.SourceNote, maxSummary int) []string {
	if maxSummary <= 0 {
		maxSummary = 3
	}
	summary := make([]string, 0, maxSummary)
	for _, note := range notes {
		if len(summary) >= maxSummary {
			break
		}
		if note.Summary != "" {
			summary = append(summary, note.Summary)
		}
	}
	return summary
}

func compactResultSummary(result types.SearchResult) string {
	parts := []string{result.Title}
	if result.Domain != "" {
		parts = append(parts, "("+result.Domain+")")
	}
	if result.Snippet != "" {
		parts = append(parts, trimText(result.Snippet, 200))
	}
	return strings.Join(parts, " ")
}

func compactReadSummary(result types.SearchResult, content string) string {
	base := compactResultSummary(result)
	content = strings.TrimSpace(content)
	if content == "" {
		return base
	}
	return strings.TrimSpace(base + " " + trimText(firstParagraph(content), 240))
}

func compareSourceNotes(notes []types.SourceNote) ([]string, []string) {
	if len(notes) == 0 {
		return nil, nil
	}
	freq := map[string]int{}
	byDomain := map[string]int{}
	for _, note := range notes {
		for _, token := range topKeywords(note.Summary, 6) {
			freq[token]++
		}
		if note.Result.Domain != "" {
			byDomain[note.Result.Domain]++
		}
	}
	agreements := make([]string, 0, 3)
	for token, count := range freq {
		if count >= minInt(2, len(notes)) {
			agreements = append(agreements, token)
		}
	}
	sort.Strings(agreements)
	if len(agreements) > 3 {
		agreements = agreements[:3]
	}
	differences := make([]string, 0, len(byDomain))
	for domain, count := range byDomain {
		differences = append(differences, fmt.Sprintf("%s contributed %d source(s)", domain, count))
	}
	sort.Strings(differences)
	return agreements, differences
}

func extractFactPack(notes []types.SourceNote, quoteLimit int) types.ExtractedFactPack {
	dateSet := map[string]struct{}{}
	entitySet := map[string]struct{}{}
	quotes := make([]string, 0, quoteLimit)
	for _, note := range notes {
		for _, date := range extractDates(note.Summary) {
			dateSet[date] = struct{}{}
		}
		for _, entity := range extractEntities(note.Summary) {
			entitySet[entity] = struct{}{}
		}
		for _, quote := range extractQuotes(note.Summary) {
			if len(quotes) >= quoteLimit {
				break
			}
			quotes = append(quotes, quote)
		}
	}
	return types.ExtractedFactPack{
		Dates:    sortedKeys(dateSet),
		Entities: sortedKeys(entitySet),
		Quotes:   quotes,
	}
}

func extractFieldsFromText(text string, fields []string) map[string][]string {
	out := make(map[string][]string, len(fields))
	for _, field := range cleanList(fields) {
		switch strings.ToLower(field) {
		case "dates":
			out[field] = extractDates(text)
		case "entities":
			out[field] = extractEntities(text)
		case "quotes":
			out[field] = extractQuotes(text)
		case "domains":
			if domain := extractDomainMention(text); domain != "" {
				out[field] = []string{domain}
			}
		default:
			out[field] = topKeywords(text, 5)
		}
	}
	return out
}

func rankResults(results []types.SearchResult, intent string) []types.RankedResult {
	intent = normalizeIntent(intent)
	ranked := make([]types.RankedResult, 0, len(results))
	for _, result := range results {
		score, reasons := scoreResult(result, intent)
		ranked = append(ranked, types.RankedResult{
			Result:  result,
			Score:   score,
			Reasons: reasons,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	return ranked
}

func scoreResult(result types.SearchResult, intent string) (int, []string) {
	score := 0
	reasons := []string{}
	text := strings.ToLower(result.Title + " " + result.Snippet + " " + result.URL)
	domain := strings.ToLower(result.Domain)
	if domain == "" {
		if parsed, err := urlpkg.Parse(result.URL); err == nil {
			domain = strings.ToLower(parsed.Hostname())
		}
	}
	switch intent {
	case "official_docs":
		if strings.Contains(text, "official") || strings.Contains(text, "documentation") || strings.Contains(text, "docs") {
			score += 5
			reasons = append(reasons, "docs keywords")
		}
		if strings.HasPrefix(domain, "docs.") || strings.Contains(domain, "developer.") || strings.HasSuffix(domain, ".org") {
			score += 4
			reasons = append(reasons, "official-looking domain")
		}
	case "latest_news":
		if result.PublishedAt != "" {
			score += 5
			reasons = append(reasons, "published timestamp")
		}
		if strings.Contains(text, "news") || strings.Contains(text, "breaking") || strings.Contains(text, "update") {
			score += 3
			reasons = append(reasons, "news keywords")
		}
	case "examples":
		if strings.Contains(text, "example") || strings.Contains(text, "tutorial") || strings.Contains(text, "sample") || strings.Contains(text, "demo") {
			score += 5
			reasons = append(reasons, "example keywords")
		}
		if strings.Contains(domain, "github.com") || strings.Contains(domain, "gitlab.com") {
			score += 3
			reasons = append(reasons, "code host")
		}
	case "primary_sources":
		if strings.Contains(text, "press release") || strings.Contains(text, "announcement") || strings.Contains(text, "transcript") || strings.Contains(text, "filing") {
			score += 4
			reasons = append(reasons, "source-of-record keywords")
		}
		if strings.HasSuffix(domain, ".gov") || strings.HasSuffix(domain, ".edu") || strings.HasSuffix(domain, ".org") {
			score += 3
			reasons = append(reasons, "institutional domain")
		}
	}
	if result.Domain != "" {
		score++
	}
	return score, reasons
}

func normalizeIntent(intent string) string {
	intent = strings.TrimSpace(strings.ToLower(intent))
	intent = strings.ReplaceAll(intent, " ", "_")
	return intent
}

func extractDates(text string) []string {
	re := regexp.MustCompile(`\b(?:\d{4}-\d{2}-\d{2}|[A-Z][a-z]+ \d{1,2}, \d{4})\b`)
	return uniqueStrings(re.FindAllString(text, -1))
}

func extractEntities(text string) []string {
	re := regexp.MustCompile(`\b(?:[A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,3})\b`)
	candidates := re.FindAllString(text, -1)
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate) < 4 {
			continue
		}
		out = append(out, candidate)
		if len(out) >= 8 {
			break
		}
	}
	return uniqueStrings(out)
}

func extractQuotes(text string) []string {
	re := regexp.MustCompile(`"([^"\n]{8,160})"`)
	matches := re.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, match[1])
		}
	}
	return uniqueStrings(out)
}

func extractDomainMention(text string) string {
	re := regexp.MustCompile(`\b[a-z0-9.-]+\.[a-z]{2,}\b`)
	return re.FindString(strings.ToLower(text))
}

func topKeywords(text string, limit int) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	stop := map[string]struct{}{
		"the": {}, "and": {}, "that": {}, "with": {}, "from": {}, "this": {}, "have": {}, "about": {}, "http": {}, "https": {}, "www": {},
	}
	freq := map[string]int{}
	for _, word := range words {
		if len(word) < 4 {
			continue
		}
		if _, ok := stop[word]; ok {
			continue
		}
		freq[word]++
	}
	type token struct {
		word  string
		count int
	}
	tokens := make([]token, 0, len(freq))
	for word, count := range freq {
		tokens = append(tokens, token{word: word, count: count})
	}
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].count == tokens[j].count {
			return tokens[i].word < tokens[j].word
		}
		return tokens[i].count > tokens[j].count
	})
	if limit > len(tokens) {
		limit = len(tokens)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, tokens[i].word)
	}
	return out
}

func firstParagraph(text string) string {
	for _, part := range strings.Split(text, "\n") {
		part = strings.TrimSpace(part)
		if part != "" {
			return part
		}
	}
	return ""
}

func trimText(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func fingerprintResults(results []types.SearchResult) string {
	builder := strings.Builder{}
	for _, result := range results {
		builder.WriteString(result.Title)
		builder.WriteByte('|')
		builder.WriteString(result.URL)
		builder.WriteByte('|')
	}
	return hashString(builder.String())
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:8])
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
