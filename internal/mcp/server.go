package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"mcp-searxng-go/internal/cache"
	"mcp-searxng-go/internal/config"
	"mcp-searxng-go/internal/fetch"
	"mcp-searxng-go/internal/search"
	"mcp-searxng-go/pkg/types"
)

// Server handles stdio and HTTP MCP requests.
type Server struct {
	cfg         config.Config
	search      *search.Client
	reader      *fetch.Reader
	logger      *slog.Logger
	searchCache *cache.TTLCache[types.SearchResponse]
	readCache   *cache.TTLCache[types.URLReadResponse]
	sem         chan struct{}
}

// NewServer returns a configured MCP server.
func NewServer(cfg config.Config, searchClient *search.Client, reader *fetch.Reader, logger *slog.Logger) *Server {
	return &Server{
		cfg:         cfg,
		search:      searchClient,
		reader:      reader,
		logger:      logger,
		searchCache: cache.New[types.SearchResponse](cfg.Cache.MaxEntries),
		readCache:   cache.New[types.URLReadResponse](cfg.Cache.MaxEntries),
		sem:         make(chan struct{}, 8),
	}
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
	categories := req.Categories
	if len(categories) == 0 {
		categories = []string{"general", "images", "videos", "news"}
	}

	results := make(map[string]types.SearchResponse, len(categories))
	seen := make(map[string]struct{}, len(categories))
	ordered := make([]string, 0, len(categories))
	for _, category := range categories {
		category = strings.TrimSpace(category)
		if !validCategory(category) {
			return types.MultiSearchResponse{}, fmt.Errorf("unsupported category %q", category)
		}
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		ordered = append(ordered, category)
		resp, err := s.runSearch(ctx, "multi_search", category, types.SearchRequest{
			Query:     req.Query,
			Category:  category,
			Language:  req.Language,
			TimeRange: req.TimeRange,
			Page:      req.Page,
			Limit:     req.Limit,
		})
		if err != nil {
			return types.MultiSearchResponse{}, err
		}
		results[category] = resp
	}
	return types.MultiSearchResponse{
		Query:      strings.TrimSpace(req.Query),
		Categories: ordered,
		Results:    results,
	}, nil
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
