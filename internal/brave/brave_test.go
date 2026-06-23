package brave

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type localServer struct {
	URL   string
	Close func()
}

func newServer(t *testing.T, handler http.Handler) *localServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(listener) }()
	return &localServer{
		URL:   "http://" + listener.Addr().String(),
		Close: func() { _ = srv.Shutdown(context.Background()); _ = listener.Close() },
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := NewClient(config.BraveConfig{
		APIKey:  "test-token",
		BaseURL: baseURL,
		Timeout: 5 * time.Second,
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSearchWebNormalizesAndSendsToken(t *testing.T) {
	t.Parallel()

	server := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Subscription-Token"); got != "test-token" {
			t.Errorf("expected subscription token header, got %q", got)
		}
		if got := r.URL.Path; got != "/web/search" {
			t.Errorf("expected /web/search, got %q", got)
		}
		if got := r.URL.Query().Get("count"); got != "3" {
			t.Errorf("expected count=3, got %q", got)
		}
		if got := r.URL.Query().Get("freshness"); got != "pd" {
			t.Errorf("expected freshness=pd, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{
						"title":       "Result One",
						"url":         "https://one.example",
						"description": "first",
						"page_age":    "2026-06-01",
						"meta_url":    map[string]any{"hostname": "one.example"},
						"profile":     map[string]any{"name": "One Profile"},
						"thumbnail":   map[string]any{"src": "https://cdn.example/1.png"},
					},
					{"title": "", "url": "https://skip.example"},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	results, err := client.Search(context.Background(), "general", types.SearchRequest{
		Query:     "golang",
		TimeRange: "day",
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 normalized result (invalid filtered), got %d", len(results))
	}
	got := results[0]
	if got.URL != "https://one.example" || got.Engine != Engine {
		t.Fatalf("unexpected result: %#v", got)
	}
	if got.Snippet != "first" || got.Domain != "one.example" || got.PublishedAt != "2026-06-01" {
		t.Fatalf("fields not mapped: %#v", got)
	}
}

func TestSearchImagesUsesImageEndpoint(t *testing.T) {
	t.Parallel()

	server := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/search" {
			t.Errorf("expected /images/search, got %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"title":      "An Image",
					"url":        "https://page.example/img",
					"source":     "page.example",
					"thumbnail":  map[string]any{"src": "https://cdn.example/t.png"},
					"properties": map[string]any{"url": "https://cdn.example/full.png", "width": 800, "height": 600},
					"meta_url":   map[string]any{"hostname": "page.example"},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	results, err := client.Search(context.Background(), "images", types.SearchRequest{Query: "cats"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 image result, got %d", len(results))
	}
	got := results[0]
	if got.ContentURL != "https://cdn.example/full.png" || got.Width != 800 || got.Height != 600 {
		t.Fatalf("image fields not mapped: %#v", got)
	}
}

func TestSearchUnmappedCategoryReturnsNothing(t *testing.T) {
	t.Parallel()

	server := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("brave endpoint must not be called for unmapped category")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	results, err := client.Search(context.Background(), "music", types.SearchRequest{Query: "x"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Fatalf("expected nil results for unmapped category, got %#v", results)
	}
}

func TestSearchWithoutAPIKeyIsNoop(t *testing.T) {
	t.Parallel()

	client, err := NewClient(config.BraveConfig{BaseURL: "https://api.search.brave.com/res/v1"})
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Search(context.Background(), "general", types.SearchRequest{Query: "x"}, 5)
	if err != nil || results != nil {
		t.Fatalf("expected no-op for empty key, got results=%#v err=%v", results, err)
	}
}
