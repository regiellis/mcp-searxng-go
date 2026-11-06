package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
)

type searxCall struct {
	Categories string
	Engines    string
	Query      string
}

// recordingSearXNG answers every search with one result and records the
// category, engines, and query SearXNG was asked for, in arrival order.
type recordingSearXNG struct {
	mu    sync.Mutex
	calls []searxCall
}

func (r *recordingSearXNG) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	values := req.URL.Query()
	r.mu.Lock()
	r.calls = append(r.calls, searxCall{
		Categories: values.Get("categories"),
		Engines:    values.Get("engines"),
		Query:      values.Get("q"),
	})
	r.mu.Unlock()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"query":   values.Get("q"),
		"results": []map[string]any{{"title": "Hit", "url": "https://hit.example", "content": "hit"}},
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *recordingSearXNG) recorded() []searxCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func callTool(t *testing.T, server *Server, name string, args map[string]any) (bool, string) {
	t.Helper()
	resp := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	}))
	if resp.Error != nil {
		payload, err := json.Marshal(resp.Error)
		if err != nil {
			t.Fatal(err)
		}
		return false, string(payload)
	}
	return true, ""
}

func TestValidCategory(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"general":                 true,
		"packages":                true,
		" map ":                   true,
		"q&a":                     true,
		"scientific publications": true,
		"social media":            true,
		"shopping":                false,
		"IT":                      false,
		"":                        false,
	}
	for category, want := range cases {
		if got := validCategory(category); got != want {
			t.Errorf("validCategory(%q) = %v, want %v", category, got, want)
		}
	}
}

func TestCategoryToolsSendExpectedSearXNGRequests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tool string
		args map[string]any
		want []searxCall
	}{
		{
			tool: "category_search",
			args: map[string]any{"query": "context deadline", "category": "q&a"},
			want: []searxCall{{Categories: "q&a", Query: "context deadline"}},
		},
		{
			tool: "package_search",
			args: map[string]any{"query": "errgroup"},
			want: []searxCall{{Categories: "packages", Query: "errgroup"}},
		},
		{
			// No keyword padding: the science engines get the bare query.
			tool: "scholar_search",
			args: map[string]any{"query": "crispr off-target"},
			want: []searxCall{{Categories: "science", Query: "crispr off-target"}},
		},
		{
			tool: "local_search",
			args: map[string]any{"query": "coffee", "location": "denver"},
			want: []searxCall{
				{Categories: "map", Query: "coffee near denver"},
				{Categories: "general", Query: "coffee near denver map hours address reviews"},
			},
		},
		{
			tool: "smart_search",
			args: map[string]any{"query": "tokio", "mode": "packages"},
			want: []searxCall{{Categories: "packages", Query: "tokio"}},
		},
		{
			tool: "multi_search",
			args: map[string]any{"query": "searxng", "categories": []string{"repos", "social media"}},
			want: []searxCall{
				{Categories: "repos", Query: "searxng"},
				{Categories: "social media", Query: "searxng"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			searx := &recordingSearXNG{}
			stub := newHTTPTestServer(t, searx)
			defer stub.Close()
			server := newTestServer(t, stub.URL)

			if ok, detail := callTool(t, server, tc.tool, tc.args); !ok {
				t.Fatalf("%s failed: %s", tc.tool, detail)
			}
			if got := searx.recorded(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("searxng calls = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCategorySearchRejectsUnsupportedCategories(t *testing.T) {
	t.Parallel()

	searx := &recordingSearXNG{}
	stub := newHTTPTestServer(t, searx)
	defer stub.Close()
	server := newTestServer(t, stub.URL)

	for _, args := range []map[string]any{
		{"query": "keyboard"},
		{"query": "keyboard", "category": "shopping"},
	} {
		ok, detail := callTool(t, server, "category_search", args)
		if ok {
			t.Fatalf("category_search %v should fail", args)
		}
		var rpcErr struct {
			Data struct {
				Supported []string `json:"supported"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(detail), &rpcErr); err != nil {
			t.Fatalf("decode error %s: %v", detail, err)
		}
		if !reflect.DeepEqual(rpcErr.Data.Supported, supportedCategories) {
			t.Fatalf("error should list supported categories, got %s", detail)
		}
	}
	if calls := searx.recorded(); len(calls) != 0 {
		t.Fatalf("rejected calls must not reach searxng: %+v", calls)
	}
}

func TestToolSchemasAdvertiseSupportedCategories(t *testing.T) {
	t.Parallel()

	for _, tool := range toolDefinitions() {
		properties, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"category", "categories"} {
			property, ok := properties[key].(map[string]any)
			if !ok {
				continue
			}
			if items, ok := property["items"].(map[string]any); ok {
				property = items
			}
			enum, ok := property["enum"].([]string)
			if !ok {
				t.Fatalf("%s.%s has no category enum: %#v", tool.Name, key, property)
			}
			if !reflect.DeepEqual(enum, supportedCategories) {
				t.Fatalf("%s.%s enum = %v, want supportedCategories", tool.Name, key, enum)
			}
		}
	}
}
