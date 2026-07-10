package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/internal/fetch"
	"github.com/regiellis/mcp-searxng-go/internal/media"
	"github.com/regiellis/mcp-searxng-go/internal/search"
	"github.com/regiellis/mcp-searxng-go/internal/security"
	"github.com/regiellis/mcp-searxng-go/internal/store"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
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

func TestToolsListIncludesNewSearchTools(t *testing.T) {
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
	if !bytes.Contains(payload, []byte(`"news_search"`)) {
		t.Fatalf("expected news_search in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"search_with_engines"`)) {
		t.Fatalf("expected search_with_engines in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"search_with_site_filter"`)) {
		t.Fatalf("expected search_with_site_filter in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"multi_search"`)) {
		t.Fatalf("expected multi_search in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"quick_look"`)) {
		t.Fatalf("expected quick_look in tools/list response: %s", string(payload))
	}
	if !bytes.Contains(payload, []byte(`"deep_research"`)) {
		t.Fatalf("expected deep_research in tools/list response: %s", string(payload))
	}
	for _, tool := range []string{
		`"scholar_search"`,
		`"local_search"`,
		`"shopping_search"`,
		`"recent_search"`,
		`"answer_search"`,
		`"compare_sources"`,
		`"fact_pack"`,
		`"monitor_query"`,
		`"search_then_extract"`,
		`"search_then_rank"`,
		`"image_quick_look"`,
		`"video_quick_look"`,
		`"find_official_docs"`,
		`"find_latest_news"`,
		`"find_examples"`,
		`"find_primary_sources"`,
		`"smart_search"`,
	} {
		if !bytes.Contains(payload, []byte(tool)) {
			t.Fatalf("expected %s in tools/list response: %s", tool, string(payload))
		}
	}
	if !bytes.Contains(payload, []byte(`"search_and_read"`)) {
		t.Fatalf("expected search_and_read in tools/list response: %s", string(payload))
	}
}

func TestSearchAndReadReturnsSelectedResult(t *testing.T) {
	t.Parallel()

	readURL := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Go</title></head><body>Go docs</body></html>`))
	}))
	defer readURL.Close()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": "Go", "url": readURL.URL, "content": "The Go programming language"},
			},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	result, err := server.runSearchAndRead(context.Background(), types.SearchAndReadRequest{
		Query: "golang",
	})
	if err != nil {
		t.Fatalf("search_and_read: %v", err)
	}
	if result.Selected == nil || result.Selected.URL != readURL.URL {
		t.Fatalf("expected selected result URL %q, got %#v", readURL.URL, result.Selected)
	}
	if result.Read == nil || result.Read.Title != "Go" {
		t.Fatalf("expected read result title Go, got %#v", result.Read)
	}
}

func TestRunMultiSearchRejectsInvalidCategory(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "https://example.com")
	_, err := server.runMultiSearch(context.Background(), types.MultiSearchRequest{
		Query:      "golang",
		Categories: []string{"general", "bogus"},
	})
	if err == nil {
		t.Fatal("expected invalid category error")
	}
}

func TestQuickLookReturnsDefaultCategories(t *testing.T) {
	t.Parallel()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("categories")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": category, "url": "https://" + category + ".example", "content": category + " result"},
			},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	result, err := server.runQuickLook(context.Background(), types.QuickLookRequest{Query: "golang"})
	if err != nil {
		t.Fatalf("quick_look: %v", err)
	}
	if len(result.Categories) != 4 {
		t.Fatalf("expected 4 categories, got %#v", result.Categories)
	}
	if result.Limit != 3 {
		t.Fatalf("expected default limit 3, got %d", result.Limit)
	}
	for _, category := range []string{"general", "images", "videos", "news"} {
		if _, ok := result.Results[category]; !ok {
			t.Fatalf("missing category %q in %#v", category, result.Results)
		}
	}
}

func TestDeepResearchReadsTopSources(t *testing.T) {
	t.Parallel()

	readURL := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Deep Dive</title></head><body>Analysis</body></html>`))
	}))
	defer readURL.Close()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("categories")
		results := []map[string]any{}
		switch category {
		case "general":
			results = []map[string]any{
				{"title": "Primary", "url": readURL.URL, "content": "Primary source"},
				{"title": "Secondary", "url": "https://secondary.example", "content": "Secondary source"},
			}
		case "news":
			results = []map[string]any{
				{"title": "Headline", "url": "https://news.example", "content": "Breaking"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "golang",
			"results": results,
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	result, err := server.runDeepResearch(context.Background(), types.DeepResearchRequest{
		Query:      "golang",
		MaxSources: 1,
	})
	if err != nil {
		t.Fatalf("deep_research: %v", err)
	}
	if result.General.Category != "general" {
		t.Fatalf("expected general category, got %q", result.General.Category)
	}
	if result.News.Category != "news" {
		t.Fatalf("expected news category, got %q", result.News.Category)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(result.Sources))
	}
	if result.Sources[0].Read == nil || result.Sources[0].Read.Title != "Deep Dive" {
		t.Fatalf("expected read title Deep Dive, got %#v", result.Sources[0].Read)
	}
}

func TestSearchThenRankPromotesDocs(t *testing.T) {
	t.Parallel()

	ranked := rankResults([]types.SearchResult{
		{Title: "Community post", URL: "https://blog.example/dev", Domain: "blog.example", Snippet: "notes"},
		{Title: "Official documentation", URL: "https://docs.example.com/api", Domain: "docs.example.com", Snippet: "API docs"},
	}, "official_docs")
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked results, got %d", len(ranked))
	}
	if ranked[0].Result.Domain != "docs.example.com" {
		t.Fatalf("expected docs result first, got %#v", ranked[0])
	}
}

func TestSearchThenExtractReturnsRequestedFields(t *testing.T) {
	t.Parallel()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "golang",
			"results": []map[string]any{
				{"title": "Go Release", "url": "https://go.dev/blog", "content": `Released on May 7, 2026 by Go Team "Faster builds"`},
			},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	result, err := server.runSearchThenExtract(context.Background(), types.SearchThenExtractRequest{
		Query:    "golang",
		Fields:   []string{"dates", "entities", "quotes"},
		ReadTopN: 1,
	})
	if err != nil {
		t.Fatalf("search_then_extract: %v", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(result.Documents))
	}
	if len(result.Documents[0].Fields["dates"]) == 0 {
		t.Fatalf("expected dates extraction, got %#v", result.Documents[0].Fields)
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

func TestDownloadSubtitlesAsyncJob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub not supported on windows")
	}

	mediaDir := t.TempDir()
	// Stub yt-dlp: find the -P output dir and drop a subtitle file there.
	stub := filepath.Join(t.TempDir(), "yt-dlp")
	script := "#!/bin/sh\n" + `outdir="."
prev=""
for a in "$@"; do
  if [ "$prev" = "-P" ]; then outdir="$a"; fi
  prev="$a"
done
printf 'WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nhello\n' > "$outdir/video.en.srt"
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// A local server stands in for the video URL so the SSRF guard resolves a
	// loopback host instead of reaching the network; the stub ignores the URL.
	src := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer src.Close()

	server := newMediaTestServer(t, mediaDir, stub)

	submit := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "download_subtitles",
		"arguments": map[string]any{"url": src.URL, "async": true},
	}))
	view := mediaJobView(t, submit)
	if view.JobID == "" || view.Status != "running" || view.Message == "" {
		t.Fatalf("expected a running job with poll hint, got %#v", view)
	}

	// Poll media_job_status until the background job completes.
	deadline := time.Now().Add(3 * time.Second)
	var final types.MediaJobView
	for time.Now().Before(deadline) {
		status := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
			"name":      "media_job_status",
			"arguments": map[string]any{"job_id": view.JobID},
		}))
		final = mediaJobView(t, status)
		if final.Status != "running" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if final.Status != "completed" {
		t.Fatalf("expected completed job, got %#v", final)
	}

	// The completed result carries the same payload the sync call would return.
	resultBytes, _ := json.Marshal(final.Result)
	var subs types.SubtitlesResponse
	if err := json.Unmarshal(resultBytes, &subs); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(subs.Files) != 1 || subs.Files[0].Filename != "video.en.srt" {
		t.Fatalf("unexpected subtitle files: %#v", subs.Files)
	}

	// An unknown job id is reported as an error.
	unknown := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "media_job_status",
		"arguments": map[string]any{"job_id": "j_nope"},
	}))
	if unknown.Error == nil {
		t.Fatal("expected error for unknown job_id")
	}
}

// mediaJobView pulls the MediaJobView out of a tools/call response.
func mediaJobView(t *testing.T, resp types.JSONRPCResponse) types.MediaJobView {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %#v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %#v", resp.Result)
	}
	view, ok := result["structuredContent"].(types.MediaJobView)
	if !ok {
		t.Fatalf("structuredContent is not a MediaJobView: %#v", result["structuredContent"])
	}
	return view
}

func newMediaTestServer(t *testing.T, mediaDir, ytDlpPath string) *Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	cfg := config.Default()
	cfg.SearXNG.BaseURL = "https://example.com"
	cfg.Fetch.Timeout = time.Second
	cfg.Security.BlockPrivateNetworks = false
	cfg.Media.OutputDir = mediaDir
	cfg.Media.YtDlpPath = ytDlpPath
	cfg.Media.Timeout = 30 * time.Second

	searchClient, err := search.NewClient(cfg.SearXNG, logger)
	if err != nil {
		t.Fatal(err)
	}
	validator := fetch.NewURLValidator(cfg.Fetch.AllowedSchemes, security.NetworkGuard{
		BlockPrivateNetworks: false,
		Policy:               security.NewDomainPolicy(nil, nil),
	})
	reader := fetch.NewReader(cfg.Fetch, validator, logger)
	runner, err := media.NewRunner(cfg.Media, validator, logger)
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(cfg, searchClient, reader, runner, nil, nil, nil, logger)
}

// fakeSynth is a stub Synthesizer that records its prompt and returns a fixed reply.
type fakeSynth struct {
	model     string
	reply     string
	gotSystem string
	gotUser   string
}

func (f *fakeSynth) Complete(_ context.Context, system, user string) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	return f.reply, nil
}

func (f *fakeSynth) Model() string { return f.model }

func TestAnswerSearchSynthesizeComposesCitedAnswer(t *testing.T) {
	t.Parallel()

	readURL := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>France facts</title></head><body>The capital of France is Paris.</body></html>`))
	}))
	defer readURL.Close()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": "capital of france",
			"results": []map[string]any{
				{"title": "France facts", "url": readURL.URL, "content": "About France"},
			},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	fake := &fakeSynth{model: "deepseek-test", reply: "The capital of France is Paris [1]."}
	server.synth = fake

	resp, err := server.runAnswerSearch(context.Background(), types.AnswerSearchRequest{
		Query:      "capital of france",
		Limit:      1,
		ReadTopN:   1,
		Synthesize: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answer != "The capital of France is Paris [1]." || resp.AnswerModel != "deepseek-test" {
		t.Fatalf("unexpected synthesis: answer=%q model=%q", resp.Answer, resp.AnswerModel)
	}
	// The deterministic packet is still present alongside the synthesized answer.
	if len(resp.Sources) != 1 || len(resp.Summary) == 0 {
		t.Fatalf("expected deterministic packet retained: %#v", resp)
	}
	// The prompt carried the question and the read source content.
	if !strings.Contains(fake.gotUser, "Question: capital of france") {
		t.Fatalf("prompt missing question: %q", fake.gotUser)
	}
	if !strings.Contains(fake.gotUser, "[1] France facts") || !strings.Contains(fake.gotUser, "capital of France is Paris") {
		t.Fatalf("prompt missing cited source content: %q", fake.gotUser)
	}
}

func TestAnswerSearchSynthesizeDisabledWithoutLLM(t *testing.T) {
	t.Parallel()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "anything",
			"results": []map[string]any{{"title": "X", "url": "https://x.example", "content": "x"}},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL) // synth is nil
	_, err := server.runAnswerSearch(context.Background(), types.AnswerSearchRequest{
		Query:      "anything",
		Limit:      1,
		ReadTopN:   1,
		Synthesize: true,
	})
	if err == nil {
		t.Fatal("expected synthesize=true without an LLM to error")
	}
	if !strings.Contains(err.Error(), "synthesis is disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnswerSearchDefaultStaysDeterministic(t *testing.T) {
	t.Parallel()

	searx := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "anything",
			"results": []map[string]any{{"title": "X", "url": "https://x.example", "content": "x"}},
		})
	}))
	defer searx.Close()

	server := newTestServer(t, searx.URL)
	// No synth set; without synthesize the call must succeed and add no answer.
	resp, err := server.runAnswerSearch(context.Background(), types.AnswerSearchRequest{
		Query:    "anything",
		Limit:    1,
		ReadTopN: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answer != "" || resp.AnswerModel != "" {
		t.Fatalf("expected no synthesized answer by default, got %#v", resp)
	}
}

func TestLanguageSlug(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Spanish":              "spanish",
		"  French  ":           "french",
		"zh-Hant":              "zh-hant",
		"Brazilian Portuguese": "brazilian-portuguese",
		"!!!":                  "translated",
		"":                     "translated",
	}
	for in, want := range cases {
		if got := languageSlug(in); got != want {
			t.Fatalf("languageSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchFeedParsesAndLimits(t *testing.T) {
	t.Parallel()

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Feed</title><link>https://ex.example</link>
<item><title>A</title><link>https://ex.example/a</link></item>
<item><title>B</title><link>https://ex.example/b</link></item>
<item><title>C</title><link>https://ex.example/c</link></item>
</channel></rss>`))
	}))
	defer server.Close()

	srv := newTestServer(t, "https://example.com")
	resp, err := srv.runFetchFeed(context.Background(), types.FetchFeedRequest{URL: server.URL, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Format != "rss" || resp.FeedTitle != "Feed" {
		t.Fatalf("unexpected feed header: %#v", resp)
	}
	if resp.ItemCount != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected limit of 2 applied, got %d", resp.ItemCount)
	}
	if resp.Items[0].Title != "A" {
		t.Fatalf("unexpected first item: %#v", resp.Items[0])
	}
}

func TestResearchStoreRoundTrip(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "https://example.com")
	st, err := store.NewStore(t.TempDir(), slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server.store = st

	// Create a session.
	created, err := server.store.SaveResearch(types.SaveResearchRequest{Title: "Roundtrip", Note: "one"})
	if err != nil {
		t.Fatal(err)
	}

	// Append via the handler path.
	resp := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "save_research",
		"arguments": map[string]any{"id": created.ID, "note": "two"},
	}))
	if resp.Error != nil {
		t.Fatalf("save_research append errored: %#v", resp.Error)
	}

	// get_research returns both notes.
	getResp := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "get_research",
		"arguments": map[string]any{"id": created.ID},
	}))
	payload, _ := json.Marshal(getResp.Result)
	if !bytes.Contains(payload, []byte(`"one"`)) || !bytes.Contains(payload, []byte(`"two"`)) {
		t.Fatalf("expected both notes in get_research: %s", payload)
	}

	// list_research includes the session.
	listResp := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "list_research",
		"arguments": map[string]any{},
	}))
	listPayload, _ := json.Marshal(listResp.Result)
	if !bytes.Contains(listPayload, []byte(created.ID)) {
		t.Fatalf("expected session id in list_research: %s", listPayload)
	}
}

func TestResearchToolsDisabledWithoutStore(t *testing.T) {
	t.Parallel()
	server := newTestServer(t, "https://example.com") // store is nil
	resp := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "list_research",
		"arguments": map[string]any{},
	}))
	if resp.Error == nil {
		t.Fatal("expected error when storage is disabled")
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
	return NewServer(cfg, searchClient, reader, nil, nil, nil, nil, logger)
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

func TestFilesEndpointServesMediaFiles(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	subDir := filepath.Join(mediaDir, "subs-test")
	if err := os.Mkdir(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("1\n00:00:00,000 --> 00:00:02,000\nhello\n")
	if err := os.WriteFile(filepath.Join(subDir, "video.en.srt"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	// A file outside the sandbox that traversal must never reach.
	outside := filepath.Join(filepath.Dir(mediaDir), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o640); err != nil {
		t.Fatal(err)
	}

	server := newMediaTestServer(t, mediaDir, "yt-dlp")
	httpServer := newHTTPTestServer(t, server.HTTPHandler())
	defer httpServer.Close()

	resp, err := http.Get(httpServer.URL + "/files/subs-test/video.en.srt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("expected file contents, got status %d body %q", resp.StatusCode, body)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "video.en.srt") {
		t.Fatalf("expected attachment disposition with filename, got %q", cd)
	}

	for name, check := range map[string]struct {
		method string
		path   string
	}{
		"missing file":   {http.MethodGet, "/files/nope.srt"},
		"directory":      {http.MethodGet, "/files/subs-test"},
		"empty path":     {http.MethodGet, "/files/"},
		"traversal":      {http.MethodGet, "/files/..%2Fsecret.txt"},
		"method not GET": {http.MethodPost, "/files/subs-test/video.en.srt"},
	} {
		req, reqErr := http.NewRequest(check.method, httpServer.URL+check.path, nil)
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		res, doErr := http.DefaultTransport.RoundTrip(req) // no client-side path cleaning or redirects
		if doErr != nil {
			t.Fatalf("%s: %v", name, doErr)
		}
		leaked, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode == http.StatusOK && bytes.Contains(leaked, []byte("secret")) {
			t.Fatalf("%s: escaped the sandbox: %q", name, leaked)
		}
		if res.StatusCode < 400 && res.StatusCode != http.StatusMovedPermanently {
			t.Fatalf("%s: expected an error status, got %d", name, res.StatusCode)
		}
	}
}

func TestReadMediaFileIncludesDownloadURL(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	content := []byte("1\n00:00:00,000 --> 00:00:02,000\nhola\n")
	if err := os.WriteFile(filepath.Join(mediaDir, "clip.en.srt"), content, 0o640); err != nil {
		t.Fatal(err)
	}

	server := newMediaTestServer(t, mediaDir, "yt-dlp")
	httpServer := newHTTPTestServer(t, server.HTTPHandler())
	defer httpServer.Close()

	// Over HTTP with no public_base_url configured, download_url is minted from
	// the request origin.
	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"read_media_file","arguments":{"path":"clip.en.srt"}}}`
	resp, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	var rpc struct {
		Result struct {
			StructuredContent types.ReadMediaFileResponse `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if rpc.Error != nil {
		t.Fatalf("unexpected rpc error: %#v", rpc.Error)
	}
	want := httpServer.URL + "/files/clip.en.srt"
	if rpc.Result.StructuredContent.DownloadURL != want {
		t.Fatalf("download_url = %q, want %q", rpc.Result.StructuredContent.DownloadURL, want)
	}

	// The minted URL actually serves the file.
	fileResp, err := http.Get(rpc.Result.StructuredContent.DownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(fileResp.Body)
	_ = fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK || !bytes.Equal(got, content) {
		t.Fatalf("fetching download_url: status %d body %q", fileResp.StatusCode, got)
	}

	// A configured public_base_url wins over the request origin.
	server.cfg.Server.PublicBaseURL = "https://mcp.example.com/"
	resp2, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(resp2.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if rpc.Result.StructuredContent.DownloadURL != "https://mcp.example.com/files/clip.en.srt" {
		t.Fatalf("public_base_url download_url = %q", rpc.Result.StructuredContent.DownloadURL)
	}

	// Without an HTTP request context (stdio mode), no URL is minted.
	server.cfg.Server.PublicBaseURL = ""
	direct := server.handle(context.Background(), mapRequest("tools/call", map[string]any{
		"name":      "read_media_file",
		"arguments": map[string]any{"path": "clip.en.srt"},
	}))
	stdioResult, ok := direct.Result.(map[string]any)["structuredContent"].(types.ReadMediaFileResponse)
	if !ok {
		t.Fatalf("unexpected structuredContent: %#v", direct.Result)
	}
	if stdioResult.DownloadURL != "" {
		t.Fatalf("expected empty download_url in stdio mode, got %q", stdioResult.DownloadURL)
	}
}

func TestDownloadSubtitlesIncludesDownloadURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub not supported on windows")
	}

	mediaDir := t.TempDir()
	stub := filepath.Join(t.TempDir(), "yt-dlp")
	script := "#!/bin/sh\n" + `outdir="."
prev=""
for a in "$@"; do
  if [ "$prev" = "-P" ]; then outdir="$a"; fi
  prev="$a"
done
printf '1\n00:00:00,000 --> 00:00:02,000\nhello\n' > "$outdir/video.en.srt"
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	src := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer src.Close()

	server := newMediaTestServer(t, mediaDir, stub)
	httpServer := newHTTPTestServer(t, server.HTTPHandler())
	defer httpServer.Close()

	body := `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"download_subtitles","arguments":{"url":"` + src.URL + `"}}}`
	resp, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	var rpc struct {
		Result struct {
			StructuredContent types.SubtitlesResponse `json:"structuredContent"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if rpc.Error != nil {
		t.Fatalf("unexpected rpc error: %#v", rpc.Error)
	}
	files := rpc.Result.StructuredContent.Files
	if len(files) != 1 || files[0].DownloadURL == "" {
		t.Fatalf("expected one file with a download_url, got %#v", files)
	}
	if !strings.HasPrefix(files[0].DownloadURL, httpServer.URL+"/files/subs-") {
		t.Fatalf("unexpected download_url %q", files[0].DownloadURL)
	}

	fileResp, err := http.Get(files[0].DownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(fileResp.Body)
	_ = fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK || !strings.Contains(string(got), "hello") {
		t.Fatalf("fetching subtitle download_url: status %d body %q", fileResp.StatusCode, got)
	}
}
