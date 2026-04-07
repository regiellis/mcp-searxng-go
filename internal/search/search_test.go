package search

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"log/slog"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

func TestNormalizeResultsFiltersInvalidItems(t *testing.T) {
	t.Parallel()

	results := normalizeResults([]searxResult{
		{Title: "One", URL: "https://one.example", Content: "a"},
		{Title: "", URL: "https://bad.example"},
		{Title: "Two", URL: "https://two.example", Content: "b"},
	}, 1)
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Title != "One" {
		t.Fatalf("unexpected result %#v", results[0])
	}
}

func TestSearchClientRespectsLimit(t *testing.T) {
	t.Parallel()

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("categories"); got != "images" {
			t.Fatalf("expected categories=images, got %q", got)
		}
		if got := r.URL.Query().Get("engines"); got != "google,duckduckgo" {
			t.Fatalf("expected engines list, got %q", got)
		}
		if got := r.URL.Query().Get("q"); got != "site:go.dev golang" {
			t.Fatalf("expected site-filtered query, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": "One", "url": "https://one.example", "content": "one"},
				{"title": "Two", "url": "https://two.example", "content": "two"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(config.SearXNGConfig{
		BaseURL:          server.URL,
		Timeout:          config.Default().SearXNG.Timeout,
		DefaultLanguage:  "all",
		DefaultTimeRange: "",
		MaxLimit:         1,
	}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Search(context.Background(), types.SearchRequest{
		Query:    "golang",
		Category: "images",
		Engines:  []string{"google", "duckduckgo"},
		Site:     "go.dev",
		Limit:    20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Limit != 1 {
		t.Fatalf("expected enforced limit 1, got %d", resp.Limit)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(resp.Results))
	}
	if resp.Category != "images" {
		t.Fatalf("expected image category, got %q", resp.Category)
	}
	if len(resp.Engines) != 2 {
		t.Fatalf("expected engines to round-trip, got %#v", resp.Engines)
	}
	if resp.Site != "go.dev" {
		t.Fatalf("expected site to round-trip, got %q", resp.Site)
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
