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
			"name":      "mcp-searxng-go",
			"transport": "http",
			"mcp_path":  "/mcp",
			"healthz":   "/healthz",
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
				"name":      "mcp-searxng-go",
				"transport": "http",
				"message":   "POST JSON-RPC requests to this endpoint",
				"methods":   []string{"initialize", "tools/list", "tools/call"},
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
		result, err := s.runSearch(ctx, input)
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

func (s *Server) runSearch(ctx context.Context, req types.SearchRequest) (types.SearchResponse, error) {
	keyBytes, _ := json.Marshal(req)
	key := string(keyBytes)
	if s.cfg.Cache.Enabled {
		if cached, ok := s.searchCache.Get(key); ok {
			cached.Cached = true
			s.logger.Info("cache hit", "tool", "web_search")
			return cached, nil
		}
	}
	s.logger.Info("tool start", "tool", "web_search", "query", req.Query)
	resp, err := s.search.Search(ctx, req)
	if err != nil {
		s.logger.Error("tool failure", "tool", "web_search", "error", err)
		return types.SearchResponse{}, err
	}
	if s.cfg.Cache.Enabled {
		s.searchCache.Set(key, resp, s.cfg.Cache.TTLSearch)
	}
	s.logger.Info("tool end", "tool", "web_search", "count", resp.ResultCount)
	return resp, nil
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
