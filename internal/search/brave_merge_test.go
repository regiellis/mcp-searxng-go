package search

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type stubBrave struct {
	results []types.SearchResult
	err     error
	calls   int
}

func (s *stubBrave) Search(_ context.Context, _ string, _ types.SearchRequest, _ int) ([]types.SearchResult, error) {
	s.calls++
	return s.results, s.err
}

func newSearXNGStub(t *testing.T) *localHTTPServer {
	t.Helper()
	return newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": "SearXNG One", "url": "https://one.example", "content": "s1"},
				{"title": "SearXNG Two", "url": "https://two.example", "content": "s2"},
			},
		})
	}))
}

func newClientWithBrave(t *testing.T, baseURL string, b BraveSearcher) *Client {
	t.Helper()
	client, err := NewClient(config.SearXNGConfig{
		BaseURL:  baseURL,
		Timeout:  config.Default().SearXNG.Timeout,
		MaxLimit: 4,
	}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)), WithBrave(b))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSearchMergesBraveResults(t *testing.T) {
	t.Parallel()

	server := newSearXNGStub(t)
	defer server.Close()

	brave := &stubBrave{results: []types.SearchResult{
		{Title: "Brave One", URL: "https://brave-a.example", Engine: "brave"},
		// Duplicate of a SearXNG URL (with trailing slash + www) must be dropped.
		{Title: "Dup Two", URL: "https://www.two.example/", Engine: "brave"},
	}}

	client := newClientWithBrave(t, server.URL, brave)
	resp, err := client.Search(context.Background(), types.SearchRequest{Query: "golang", Category: "general", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if brave.calls != 1 {
		t.Fatalf("expected brave to be called once, got %d", brave.calls)
	}

	// Expect interleaved order: searxng[0], brave[0], searxng[1]; duplicate dropped.
	wantURLs := []string{"https://one.example", "https://brave-a.example", "https://two.example"}
	if len(resp.Results) != len(wantURLs) {
		t.Fatalf("expected %d merged results, got %d (%#v)", len(wantURLs), len(resp.Results), resp.Results)
	}
	for i, want := range wantURLs {
		if resp.Results[i].URL != want {
			t.Fatalf("result %d: expected %q, got %q", i, want, resp.Results[i].URL)
		}
	}
	if resp.ResultCount != len(wantURLs) {
		t.Fatalf("expected result_count %d, got %d", len(wantURLs), resp.ResultCount)
	}
}

func TestSearchFailsOpenWhenBraveErrors(t *testing.T) {
	t.Parallel()

	server := newSearXNGStub(t)
	defer server.Close()

	brave := &stubBrave{err: errors.New("brave unreachable")}
	client := newClientWithBrave(t, server.URL, brave)

	resp, err := client.Search(context.Background(), types.SearchRequest{Query: "golang", Category: "general", Limit: 4})
	if err != nil {
		t.Fatalf("expected fail-open (nil error), got %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 searxng results when brave fails, got %d", len(resp.Results))
	}
}

func TestMergeResultsRespectsLimit(t *testing.T) {
	t.Parallel()

	primary := []types.SearchResult{
		{URL: "https://a.example"},
		{URL: "https://b.example"},
	}
	secondary := []types.SearchResult{
		{URL: "https://c.example"},
		{URL: "https://d.example"},
	}
	merged := mergeResults(primary, secondary, 3)
	if len(merged) != 3 {
		t.Fatalf("expected limit of 3, got %d", len(merged))
	}
	// Interleaved: a, c, b
	want := []string{"https://a.example", "https://c.example", "https://b.example"}
	for i, w := range want {
		if merged[i].URL != w {
			t.Fatalf("result %d: expected %q, got %q", i, w, merged[i].URL)
		}
	}
}

func TestDedupeKeyNormalizes(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://www.example.com/path/": "example.com/path",
		"http://Example.com/path":       "example.com/path",
		"https://example.com/path?q=1":  "example.com/path?q=1",
	}
	base := dedupeKey("https://www.example.com/path/")
	for raw, want := range cases {
		if got := dedupeKey(raw); got != want {
			t.Fatalf("dedupeKey(%q) = %q, want %q", raw, got, want)
		}
	}
	if base != "example.com/path" {
		t.Fatalf("unexpected base key %q", base)
	}
}
