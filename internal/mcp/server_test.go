package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"log/slog"

	"mcp-searxng-go/internal/config"
	"mcp-searxng-go/internal/fetch"
	"mcp-searxng-go/internal/search"
	"mcp-searxng-go/internal/security"
	"mcp-searxng-go/pkg/types"
)

func TestHTTPEndToEndSearch(t *testing.T) {
	t.Parallel()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": "Go", "url": "https://go.dev", "content": "Go site"},
			},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	httpServer := newHTTPTestServer(t, server.HTTPHandler())
	defer httpServer.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"golang","limit":1}}}`
	resp, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var rpcResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatal(err)
	}
	if rpcResp["error"] != nil {
		t.Fatalf("unexpected rpc error: %#v", rpcResp["error"])
	}
}

func TestToolsListIncludesImageAndVideoSearch(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "https://example.com")
	resp := server.handle(context.Background(), mapRequest("tools/list", nil))
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	payload, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte(`"image_search"`)) {
		t.Fatalf("expected image_search in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"video_search"`)) {
		t.Fatalf("expected video_search in tools/list response: %s", string(payload))
	}
}

func TestStdioInvalidMethod(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "https://example.com")
	var out bytes.Buffer
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"missing"}`)
	input := []byte("Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + string(payload))
	if err := server.ServeStdio(context.Background(), bytes.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"code":-32601`)) {
		t.Fatalf("expected method not found, got %s", out.String())
	}
}

func newTestServer(t *testing.T, searxURL string) *Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	cfg := config.Default()
	cfg.SearXNG.BaseURL = searxURL
	cfg.Fetch.Timeout = time.Second
	cfg.Security.BlockPrivateNetworks = false

	searchClient, err := search.NewClient(cfg.SearXNG, logger)
	if err != nil {
		t.Fatal(err)
	}
	reader := fetch.NewReader(cfg.Fetch, fetch.NewURLValidator(cfg.Fetch.AllowedSchemes, security.NetworkGuard{
		BlockPrivateNetworks: false,
		Policy:               security.NewDomainPolicy(nil, nil),
	}), logger)
	return NewServer(cfg, searchClient, reader, logger)
}

func mapRequest(method string, params any) types.JSONRPCRequest {
	var raw json.RawMessage
	if params != nil {
		data, _ := json.Marshal(params)
		raw = data
	}
	return types.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  raw,
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type localHTTPServer struct {
	URL   string
	Close func()
}

func newHTTPTestServer(t *testing.T, handler http.Handler) *localHTTPServer {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ipv4 loopback: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(listener)
	}()
	return &localHTTPServer{
		URL: "http://" + listener.Addr().String(),
		Close: func() {
			_ = srv.Shutdown(context.Background())
			_ = listener.Close()
		},
	}
}
