package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// loadFixtures reads testdata/searxng_responses.json: real SearXNG
// 2026.11.5 JSON responses captured from a live instance, trimmed to a few
// results per template.
func loadFixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("testdata/searxng_responses.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	return fixtures
}

func decodeFixture(t *testing.T, fixtures map[string]json.RawMessage, name string) decodedResults {
	t.Helper()
	raw, ok := fixtures[name]
	if !ok {
		t.Fatalf("fixture %q missing", name)
	}
	var resp searxResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode fixture %q: %v", name, err)
	}
	return decodeResponse(resp, "", 0)
}

func findKind(t *testing.T, results []types.SearchResult, kind string) types.SearchResult {
	t.Helper()
	for _, result := range results {
		if result.Kind == kind {
			return result
		}
	}
	t.Fatalf("no result of kind %q in %d results", kind, len(results))
	return types.SearchResult{}
}

func TestDecodeRealResponses(t *testing.T) {
	t.Parallel()
	fixtures := loadFixtures(t)

	cases := []struct {
		name  string
		check func(t *testing.T, got decodedResults)
	}{
		{"general_infobox", func(t *testing.T, got decodedResults) {
			if len(got.infoboxes) != 1 {
				t.Fatalf("infoboxes = %d, want 1", len(got.infoboxes))
			}
			box := got.infoboxes[0]
			if box.Title != "Albert Einstein" || box.Content == "" || box.ImageURL == "" {
				t.Fatalf("infobox header = %+v", box)
			}
			if !reflect.DeepEqual(box.Engines, []string{"wikidata", "wikipedia"}) {
				t.Fatalf("infobox engines = %v", box.Engines)
			}
			wantAttr := types.InfoboxAttribute{Label: "Date of birth", Value: "Friday, March 14, 1879", Entity: "P569"}
			if box.Attributes[0] != wantAttr {
				t.Fatalf("first attribute = %+v, want %+v", box.Attributes[0], wantAttr)
			}
			official := false
			for _, link := range box.URLs {
				if link.Official && link.URL == "https://alberteinstein.com" {
					official = true
				}
			}
			if !official {
				t.Fatalf("official website link missing from %+v", box.URLs)
			}
			if len(got.suggestions) == 0 || len(got.suggestions) > maxSuggestions {
				t.Fatalf("suggestions = %v", got.suggestions)
			}
			wantDown := types.UnresponsiveEngine{Engine: "searchmysite", Reason: "access denied"}
			if !containsUnresponsive(got.unresponsive, wantDown) {
				t.Fatalf("unresponsive = %+v, want %+v", got.unresponsive, wantDown)
			}
		}},
		{"answer", func(t *testing.T, got decodedResults) {
			if len(got.answers) != 1 {
				t.Fatalf("answers = %+v", got.answers)
			}
			if !strings.Contains(got.answers[0].Answer, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824") {
				t.Fatalf("answer = %q", got.answers[0].Answer)
			}
			if got.answers[0].Engine != "plugin: hash_plugin" || got.answers[0].URL != "" {
				t.Fatalf("answer meta = %+v", got.answers[0])
			}
		}},
		{"videos", func(t *testing.T, got decodedResults) {
			video := got.results[0]
			if video.Kind != "video" || video.Duration != "08:03" {
				t.Fatalf("video kind/duration = %q/%q", video.Kind, video.Duration)
			}
			if !reflect.DeepEqual(video.Engines, []string{"bing videos", "youtube", "duckduckgo videos"}) {
				t.Fatalf("video engines = %v", video.Engines)
			}
			if video.Score != 6.1364 {
				t.Fatalf("score = %v, want 6.1364 (rounded)", video.Score)
			}
			if video.Views != "" {
				t.Fatalf("null views should decode empty, got %q", video.Views)
			}
		}},
		{"images", func(t *testing.T, got decodedResults) {
			image := got.results[0]
			if image.Kind != "image" || image.Width != 2464 || image.Height != 1643 {
				t.Fatalf("image kind/size = %q %dx%d", image.Kind, image.Width, image.Height)
			}
			if image.Format != "image/jpeg" || image.FileSize != "3.86 MB" || image.ContentURL == "" {
				t.Fatalf("image detail = %+v", image)
			}
		}},
		{"packages", func(t *testing.T, got decodedResults) {
			pkg := findKind(t, got.results, "package")
			want := &types.PackageInfo{Name: "Valkey", Maintainer: "dhi", Popularity: "1M+ pulls, 0 stars"}
			if pkg.URL != "https://hub.docker.com/r/dhi/valkey" || !reflect.DeepEqual(pkg.Package, want) {
				t.Fatalf("package = %s %+v", pkg.URL, pkg.Package)
			}
			if !reflect.DeepEqual(pkg.Tags, []string{"amd64", "arm64"}) {
				t.Fatalf("tags = %v", pkg.Tags)
			}
			if web := findKind(t, got.results, "web"); web.Package != nil {
				t.Fatalf("web result must not carry package block: %+v", web.Package)
			}
		}},
		{"papers", func(t *testing.T, got decodedResults) {
			paper := findKind(t, got.results, "paper")
			if paper.Paper == nil {
				t.Fatal("paper block missing")
			}
			if !reflect.DeepEqual(paper.Paper.Authors, []string{"Ali Hassani", "Humphrey Shi"}) {
				t.Fatalf("authors = %v", paper.Paper.Authors)
			}
			if paper.Paper.PDFURL != "https://arxiv.org/pdf/2209.15001v3" || paper.Paper.DOI != "" {
				t.Fatalf("paper = %+v", paper.Paper)
			}
			if len(paper.Paper.ISSN) != 0 {
				t.Fatalf("empty issn list should stay empty, got %v", paper.Paper.ISSN)
			}
		}},
		{"torrents", func(t *testing.T, got decodedResults) {
			torrent := findKind(t, got.results, "torrent")
			if torrent.Torrent == nil || torrent.Torrent.Seeders != 51 || torrent.Torrent.Leechers != 8 {
				t.Fatalf("torrent = %+v", torrent.Torrent)
			}
			if !strings.HasPrefix(torrent.Torrent.MagnetLink, "magnet:?xt=urn:btih:") || torrent.FileSize != "5.78 GB" {
				t.Fatalf("torrent detail = %+v size=%q", torrent.Torrent, torrent.FileSize)
			}
			file := findKind(t, got.results, "file")
			if file.Format != "application/pdf" || file.FileSize != "26.10 MB" || !strings.HasPrefix(file.ContentURL, "https://upload.wikimedia.org/") {
				t.Fatalf("file detail = %+v", file)
			}
		}},
		{"map", func(t *testing.T, got decodedResults) {
			place := findKind(t, got.results, "map")
			loc := place.Location
			if loc == nil || loc.Latitude != 39.7392364 || loc.Longitude != -104.984862 {
				t.Fatalf("location = %+v", loc)
			}
			if loc.OSMType != "relation" || loc.OSMID != 1411339 || len(loc.BoundingBox) != 4 {
				t.Fatalf("osm/bbox = %+v", loc)
			}
			if len(loc.Links) == 0 || loc.Links[0].URL == "" {
				t.Fatalf("links = %+v", loc.Links)
			}
			encoded, err := json.Marshal(place)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "coordinates") {
				t.Fatalf("geojson must not be passed through: %s", encoded)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decodeFixture(t, fixtures, tc.name)
			if len(got.results) == 0 || got.available != len(got.results) {
				t.Fatalf("results=%d available=%d", len(got.results), got.available)
			}
			for _, result := range got.results {
				if result.Kind == "" || len(result.Engines) == 0 || result.Category == "" {
					t.Fatalf("result missing kind/engines/category: %+v", result)
				}
			}
			tc.check(t, got)
		})
	}
}

func containsUnresponsive(list []types.UnresponsiveEngine, want types.UnresponsiveEngine) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestFlexDecoding(t *testing.T) {
	t.Parallel()

	var got struct {
		S1 flexString  `json:"s1"`
		S2 flexString  `json:"s2"`
		S3 flexString  `json:"s3"`
		S4 flexString  `json:"s4"`
		I1 flexInt     `json:"i1"`
		I2 flexInt     `json:"i2"`
		I3 flexInt     `json:"i3"`
		F1 flexFloat   `json:"f1"`
		L1 flexStrings `json:"l1"`
		L2 flexStrings `json:"l2"`
		L3 flexStrings `json:"l3"`
		B1 flexFloats  `json:"b1"`
		B2 flexFloats  `json:"b2"`
	}
	input := `{
		"s1": "  padded  ", "s2": 228, "s3": null, "s4": {"nested": true},
		"i1": "51", "i2": 8, "i3": "n/a",
		"f1": "39.73",
		"l1": ["a", "", 3, null], "l2": "single", "l3": {"x": 1},
		"b1": ["39.6", 39.9, "-105.1", -104.5], "b2": [1, "x"]
	}`
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("flex decode must never fail: %v", err)
	}
	if got.S1 != "padded" || got.S2 != "228" || got.S3 != "" || got.S4 != "" {
		t.Fatalf("strings = %q %q %q %q", got.S1, got.S2, got.S3, got.S4)
	}
	if got.I1 != 51 || got.I2 != 8 || got.I3 != 0 || got.F1 != 39.73 {
		t.Fatalf("numbers = %d %d %d %v", got.I1, got.I2, got.I3, got.F1)
	}
	if !reflect.DeepEqual([]string(got.L1), []string{"a", "3"}) || !reflect.DeepEqual([]string(got.L2), []string{"single"}) || got.L3 != nil {
		t.Fatalf("lists = %v %v %v", got.L1, got.L2, got.L3)
	}
	if !reflect.DeepEqual([]float64(got.B1), []float64{39.6, 39.9, -105.1, -104.5}) || got.B2 != nil {
		t.Fatalf("floats = %v %v", got.B1, got.B2)
	}
}

func TestNormalizeHelpers(t *testing.T) {
	t.Parallel()

	durations := map[string]string{
		"":      "",
		"08:47": "08:47",
		"228":   "3:48",
		"3725":  "1:02:05",
		"59.6":  "1:00",
		"PT3M":  "PT3M",
	}
	for in, want := range durations {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%q) = %q, want %q", in, got, want)
		}
	}

	kinds := map[string]string{
		"":              "web",
		"default.html":  "web",
		"images.html":   "image",
		"videos.html":   "video",
		"packages.html": "package",
		"paper.html":    "paper",
		"torrent.html":  "torrent",
		"map.html":      "map",
		"code.html":     "code",
	}
	for in, want := range kinds {
		if got := kindFromTemplate(in); got != want {
			t.Errorf("kindFromTemplate(%q) = %q, want %q", in, got, want)
		}
	}

	resolutions := []struct {
		in   string
		w, h int
	}{
		{"2464 x 1643", 2464, 1643},
		{"800×600", 800, 600},
		{"large", 0, 0},
		{"", 0, 0},
	}
	for _, tc := range resolutions {
		if w, h := parseResolution(tc.in); w != tc.w || h != tc.h {
			t.Errorf("parseResolution(%q) = %dx%d, want %dx%d", tc.in, w, h, tc.w, tc.h)
		}
	}
}

func TestDecodeSkipsMalformedEntries(t *testing.T) {
	t.Parallel()

	raw := `{
		"results": [
			"not an object",
			{"title": {"oops": 1}, "url": "https://bad.example"},
			{"title": "Good", "url": "https://good.example", "engine": "bing", "seed": [1]}
		],
		"answers": ["legacy string answer", {"answer": ""}, 42],
		"infoboxes": [{"infobox": "", "content": ""}, "junk", {"infobox": "Q30249683", "engine": "wikidata", "urls": [{"title": "Wikidata", "url": "https://www.wikidata.org/wiki/Q30249683"}]}],
		"unresponsive_engines": [["qwant", "CAPTCHA"], ["qwant", "CAPTCHA"], "junk", []],
		"suggestions": ["a", "A", " b "],
		"corrections": null
	}`
	var resp searxResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := decodeResponse(resp, "", 0)
	if len(got.results) != 1 || got.results[0].Title != "Good" {
		t.Fatalf("results = %+v", got.results)
	}
	if !reflect.DeepEqual(got.results[0].Engines, []string{"bing"}) {
		t.Fatalf("engines fallback = %v", got.results[0].Engines)
	}
	if len(got.answers) != 2 || got.answers[0].Answer != "legacy string answer" || got.answers[1].Answer != "42" {
		t.Fatalf("answers = %+v", got.answers)
	}
	if len(got.infoboxes) != 0 {
		t.Fatalf("infoboxes = %+v", got.infoboxes)
	}
	if !reflect.DeepEqual(got.unresponsive, []types.UnresponsiveEngine{{Engine: "qwant", Reason: "CAPTCHA"}}) {
		t.Fatalf("unresponsive = %+v", got.unresponsive)
	}
	if !reflect.DeepEqual(got.suggestions, []string{"a", "b"}) || got.corrections != nil {
		t.Fatalf("suggestions=%v corrections=%v", got.suggestions, got.corrections)
	}
}

func TestSearchPassesThroughRichFields(t *testing.T) {
	t.Parallel()
	fixtures := loadFixtures(t)

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(fixtures["general_infobox"]); err != nil {
			t.Errorf("write fixture: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(config.SearXNGConfig{
		BaseURL:  server.URL,
		Timeout:  config.Default().SearXNG.Timeout,
		MaxLimit: 1,
	}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Search(context.Background(), types.SearchRequest{Query: "albert einstein"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ResultCount != 1 || resp.AvailableResults != 2 {
		t.Fatalf("result_count=%d available_results=%d, want 1/2", resp.ResultCount, resp.AvailableResults)
	}
	if len(resp.Infoboxes) != 1 || len(resp.Suggestions) == 0 || len(resp.UnresponsiveEngines) == 0 {
		t.Fatalf("aux blocks missing: infoboxes=%d suggestions=%d unresponsive=%d",
			len(resp.Infoboxes), len(resp.Suggestions), len(resp.UnresponsiveEngines))
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"available_results"`, `"infoboxes"`, `"suggestions"`, `"unresponsive_engines"`, `"engines"`, `"kind"`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("encoded response lacks %s: %s", key, encoded)
		}
	}
}

func TestSearchReportsNonOKStatus(t *testing.T) {
	t.Parallel()

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		if _, err := w.Write([]byte("<html>502 Bad Gateway</html>")); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(config.SearXNGConfig{
		BaseURL:  server.URL,
		Timeout:  config.Default().SearXNG.Timeout,
		MaxLimit: 5,
	}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Search(context.Background(), types.SearchRequest{Query: "golang"})
	if err == nil || !strings.Contains(err.Error(), "searxng status 502") {
		t.Fatalf("err = %v, want searxng status 502", err)
	}
}

func TestMergeResultsUnionsEngines(t *testing.T) {
	t.Parallel()

	primary := []types.SearchResult{
		{URL: "https://one.example/", Engine: "bing", Engines: []string{"bing", "yahoo"}},
	}
	secondary := []types.SearchResult{
		{URL: "https://www.one.example", Engine: "brave", Engines: []string{"brave", "bing"}},
		{URL: "https://two.example", Engine: "brave", Engines: []string{"brave"}},
	}
	merged := mergeResults(primary, secondary, 1)
	if len(merged) != 1 {
		t.Fatalf("limit 1 not honored: %+v", merged)
	}
	if !reflect.DeepEqual(merged[0].Engines, []string{"bing", "yahoo", "brave"}) {
		t.Fatalf("engines = %v, want union in order", merged[0].Engines)
	}
	if !reflect.DeepEqual(primary[0].Engines, []string{"bing", "yahoo"}) {
		t.Fatalf("merge mutated the input slice: %v", primary[0].Engines)
	}
}

func TestPromotePackageMatches(t *testing.T) {
	t.Parallel()

	pkg := func(name, engine string) types.SearchResult {
		return types.SearchResult{Title: name, URL: "https://" + engine + ".example/" + name, Kind: "package", Engine: engine,
			Package: &types.PackageInfo{Name: name}}
	}
	web := types.SearchResult{Title: "tokio tutorial", URL: "https://blog.example/tokio", Kind: "web"}
	results := []types.SearchResult{
		pkg("imegateleport/tokio", "docker hub"),
		web,
		pkg("tokio-util", "crates.io"),
		pkg("noworkers", "lib.rs"),
		pkg("Tokio", "crates.io"),
	}

	got := promotePackageMatches(results, " tokio ")
	var names []string
	for _, result := range got {
		names = append(names, result.Title)
	}
	// exact (case-insensitive), last path segment, substring, then the rest in
	// their original order.
	want := []string{"Tokio", "imegateleport/tokio", "tokio-util", "tokio tutorial", "noworkers"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
	if results[0].Title != "imegateleport/tokio" {
		t.Fatalf("input slice was reordered in place: %v", results[0].Title)
	}

	plain := []types.SearchResult{web, {Title: "Other", URL: "https://other.example", Kind: "web"}}
	if got := promotePackageMatches(plain, "tokio"); !reflect.DeepEqual(got, plain) {
		t.Fatalf("non-package results must keep their order: %+v", got)
	}
}

func TestDecodePromotesExactPackageFromRealResponse(t *testing.T) {
	t.Parallel()
	fixtures := loadFixtures(t)

	var resp searxResponse
	if err := json.Unmarshal(fixtures["packages"], &resp); err != nil {
		t.Fatal(err)
	}
	if got := decodeResponse(resp, "", 0); got.results[0].Kind != "web" {
		t.Fatalf("fixture premise: SearXNG lists a web result first, got %q", got.results[0].Kind)
	}
	got := decodeResponse(resp, "valkey", 1)
	if got.results[0].Kind != "package" || got.results[0].Package.Name != "Valkey" {
		t.Fatalf("expected the Valkey package first, got %+v", got.results[0])
	}
	if got.available != 4 {
		t.Fatalf("available = %d, want 4 (promotion must not drop results)", got.available)
	}
}
