package types

// SearchRequest is the input for the web_search tool.
type SearchRequest struct {
	Query     string   `json:"query"`
	Category  string   `json:"category,omitempty"`
	Engines   []string `json:"engines,omitempty"`
	Site      string   `json:"site,omitempty"`
	Language  string   `json:"language,omitempty"`
	TimeRange string   `json:"time_range,omitempty"`
	Page      int      `json:"page,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

// SearchResult is a normalized SearXNG result.
//
// Kind names the result's shape ("web", "image", "video", "package", "paper",
// "torrent", "file", "map"); the matching detail block (Package, Paper,
// Torrent, Location) is set only for that kind. Engines lists every engine
// that returned this URL, so len(Engines) is a cross-engine agreement signal;
// Score is SearXNG's own relevance score (higher is better, comparable only
// within one response).
type SearchResult struct {
	Title        string        `json:"title"`
	URL          string        `json:"url"`
	Snippet      string        `json:"snippet,omitempty"`
	Kind         string        `json:"kind,omitempty"`
	Category     string        `json:"category,omitempty"`
	Engine       string        `json:"engine,omitempty"`
	Engines      []string      `json:"engines,omitempty"`
	Score        float64       `json:"score,omitempty"`
	Source       string        `json:"source,omitempty"`
	Domain       string        `json:"domain,omitempty"`
	ThumbnailURL string        `json:"thumbnail_url,omitempty"`
	ContentURL   string        `json:"content_url,omitempty"`
	EmbedURL     string        `json:"embed_url,omitempty"`
	AudioURL     string        `json:"audio_url,omitempty"`
	Width        int           `json:"width,omitempty"`
	Height       int           `json:"height,omitempty"`
	Resolution   string        `json:"resolution,omitempty"`
	Format       string        `json:"format,omitempty"`
	FileSize     string        `json:"file_size,omitempty"`
	Duration     string        `json:"duration,omitempty"`
	Views        string        `json:"views,omitempty"`
	PublishedAt  string        `json:"published_at,omitempty"`
	Channel      string        `json:"channel,omitempty"`
	Author       string        `json:"author,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	Package      *PackageInfo  `json:"package,omitempty"`
	Paper        *PaperInfo    `json:"paper,omitempty"`
	Torrent      *TorrentInfo  `json:"torrent,omitempty"`
	Location     *LocationInfo `json:"location,omitempty"`
}

// PackageInfo carries software-package metadata (pypi, npm, docker hub, ...).
type PackageInfo struct {
	Name          string `json:"name,omitempty"`
	Version       string `json:"version,omitempty"`
	Maintainer    string `json:"maintainer,omitempty"`
	Popularity    string `json:"popularity,omitempty"`
	Homepage      string `json:"homepage,omitempty"`
	SourceCodeURL string `json:"source_code_url,omitempty"`
	LicenseName   string `json:"license_name,omitempty"`
	LicenseURL    string `json:"license_url,omitempty"`
}

// PaperInfo carries scholarly-paper metadata (arxiv, crossref, pubmed, ...).
type PaperInfo struct {
	DOI       string   `json:"doi,omitempty"`
	Authors   []string `json:"authors,omitempty"`
	Journal   string   `json:"journal,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Editor    string   `json:"editor,omitempty"`
	Type      string   `json:"type,omitempty"`
	Volume    string   `json:"volume,omitempty"`
	Number    string   `json:"number,omitempty"`
	Pages     string   `json:"pages,omitempty"`
	ISSN      []string `json:"issn,omitempty"`
	ISBN      []string `json:"isbn,omitempty"`
	PDFURL    string   `json:"pdf_url,omitempty"`
	HTMLURL   string   `json:"html_url,omitempty"`
	Comments  string   `json:"comments,omitempty"`
}

// TorrentInfo carries torrent swarm metadata.
type TorrentInfo struct {
	MagnetLink string `json:"magnet_link,omitempty"`
	Seeders    int    `json:"seeders"`
	Leechers   int    `json:"leechers"`
}

// LocationInfo carries map/geocoding metadata (OpenStreetMap, photon, ...).
// BoundingBox is [min_lat, max_lat, min_lon, max_lon] as SearXNG reports it.
type LocationInfo struct {
	Latitude    float64           `json:"latitude"`
	Longitude   float64           `json:"longitude"`
	BoundingBox []float64         `json:"bounding_box,omitempty"`
	OSMType     string            `json:"osm_type,omitempty"`
	OSMID       int64             `json:"osm_id,omitempty"`
	Address     map[string]string `json:"address,omitempty"`
	Links       []Link            `json:"links,omitempty"`
}

// Link is a titled URL.
type Link struct {
	Title    string `json:"title,omitempty"`
	URL      string `json:"url"`
	Official bool   `json:"official,omitempty"`
}

// Answer is a direct answer computed by a SearXNG answerer or plugin
// (hashes, unit math, random values, ...), not a web page.
type Answer struct {
	Answer string `json:"answer"`
	URL    string `json:"url,omitempty"`
	Engine string `json:"engine,omitempty"`
}

// Infobox is an entity card (typically Wikipedia/Wikidata) for the query.
type Infobox struct {
	Title      string             `json:"title"`
	ID         string             `json:"id,omitempty"`
	Content    string             `json:"content,omitempty"`
	ImageURL   string             `json:"image_url,omitempty"`
	Engine     string             `json:"engine,omitempty"`
	Engines    []string           `json:"engines,omitempty"`
	URLs       []Link             `json:"urls,omitempty"`
	Attributes []InfoboxAttribute `json:"attributes,omitempty"`
}

// InfoboxAttribute is one labeled fact on an infobox. Entity is the Wikidata
// property ID when the source provides one (e.g. "P569" for date of birth).
type InfoboxAttribute struct {
	Label    string `json:"label"`
	Value    string `json:"value,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Entity   string `json:"entity,omitempty"`
}

// UnresponsiveEngine is an engine that failed or was suspended for this query.
type UnresponsiveEngine struct {
	Engine string `json:"engine"`
	Reason string `json:"reason,omitempty"`
}

// SearchResponse is returned by the web_search tool.
//
// AvailableResults counts the SearXNG results on this page before limit was
// applied, so an agent can tell a capped page from a thin one. Suggestions are
// related queries and Corrections are spelling fixes, both ready to reissue.
// UnresponsiveEngines lists engines that failed for this query; a long list
// means results may be thinner than usual.
type SearchResponse struct {
	Query               string               `json:"query"`
	Category            string               `json:"category,omitempty"`
	Engines             []string             `json:"engines,omitempty"`
	Site                string               `json:"site,omitempty"`
	Page                int                  `json:"page"`
	Limit               int                  `json:"limit"`
	ResultCount         int                  `json:"result_count"`
	AvailableResults    int                  `json:"available_results"`
	Results             []SearchResult       `json:"results"`
	Answers             []Answer             `json:"answers,omitempty"`
	Infoboxes           []Infobox            `json:"infoboxes,omitempty"`
	Suggestions         []string             `json:"suggestions,omitempty"`
	Corrections         []string             `json:"corrections,omitempty"`
	UnresponsiveEngines []UnresponsiveEngine `json:"unresponsive_engines,omitempty"`
	Cached              bool                 `json:"cached"`
}

// MultiSearchRequest runs one query against multiple categories.
type MultiSearchRequest struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Language   string   `json:"language,omitempty"`
	TimeRange  string   `json:"time_range,omitempty"`
	Page       int      `json:"page,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// MultiSearchResponse groups results by category.
type MultiSearchResponse struct {
	Query      string                    `json:"query"`
	Categories []string                  `json:"categories"`
	Results    map[string]SearchResponse `json:"results"`
}

// CategorySearchRequest is a generic category-oriented search request.
type CategorySearchRequest struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Language   string   `json:"language,omitempty"`
	TimeRange  string   `json:"time_range,omitempty"`
	Page       int      `json:"page,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Location   string   `json:"location,omitempty"`
}

// QuickLookRequest returns a compact cross-category snapshot.
type QuickLookRequest struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Language   string   `json:"language,omitempty"`
	TimeRange  string   `json:"time_range,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// QuickLookResponse is a compact cross-category snapshot.
type QuickLookResponse struct {
	Query      string                    `json:"query"`
	Categories []string                  `json:"categories"`
	Limit      int                       `json:"limit"`
	Results    map[string]SearchResponse `json:"results"`
}

// DeepResearchRequest expands a query and reads the top sources.
type DeepResearchRequest struct {
	Query        string `json:"query"`
	Language     string `json:"language,omitempty"`
	TimeRange    string `json:"time_range,omitempty"`
	GeneralLimit int    `json:"general_limit,omitempty"`
	NewsLimit    int    `json:"news_limit,omitempty"`
	MaxSources   int    `json:"max_sources,omitempty"`
}

// DeepResearchSource captures a selected search result and optional read output.
type DeepResearchSource struct {
	Result SearchResult     `json:"result"`
	Read   *URLReadResponse `json:"read,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// DeepResearchResponse combines broader search results with readbacks from top sources.
type DeepResearchResponse struct {
	Query      string               `json:"query"`
	General    SearchResponse       `json:"general"`
	News       SearchResponse       `json:"news"`
	MaxSources int                  `json:"max_sources"`
	Sources    []DeepResearchSource `json:"sources"`
}

// AnswerSearchRequest searches then synthesizes a compact answer packet.
type AnswerSearchRequest struct {
	Query      string `json:"query"`
	Language   string `json:"language,omitempty"`
	TimeRange  string `json:"time_range,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	ReadTopN   int    `json:"read_top_n,omitempty"`
	MaxSummary int    `json:"max_summary,omitempty"`
	// Synthesize, when true, additionally sends the read sources through the LLM
	// to compose a written, cited answer. Off by default: the deterministic
	// packet needs no API key, and synthesis is an explicit opt-in.
	Synthesize bool `json:"synthesize,omitempty"`
}

// SourceNote summarizes one result and optional read.
type SourceNote struct {
	Result  SearchResult `json:"result"`
	Summary string       `json:"summary,omitempty"`
}

// AnswerSearchResponse returns a compact summary with backing sources. Answer
// and AnswerModel are populated only when synthesize=true was requested and an
// LLM is configured; the bracketed citations in Answer refer to Sources by
// 1-based position.
type AnswerSearchResponse struct {
	Query       string         `json:"query"`
	Summary     []string       `json:"summary"`
	Search      SearchResponse `json:"search"`
	ReadTopN    int            `json:"read_top_n"`
	Sources     []SourceNote   `json:"sources"`
	Answer      string         `json:"answer,omitempty"`
	AnswerModel string         `json:"answer_model,omitempty"`
}

// CompareSourcesRequest searches then compares multiple source reads.
type CompareSourcesRequest struct {
	Query     string `json:"query"`
	Language  string `json:"language,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	ReadTopN  int    `json:"read_top_n,omitempty"`
}

// SourceComparison summarizes agreement and disagreement across reads.
type SourceComparison struct {
	Agreements    []string     `json:"agreements"`
	Differences   []string     `json:"differences"`
	SourceNotes   []SourceNote `json:"source_notes"`
	ComparedCount int          `json:"compared_count"`
}

// CompareSourcesResponse returns search results plus comparison notes.
type CompareSourcesResponse struct {
	Query      string           `json:"query"`
	Search     SearchResponse   `json:"search"`
	Comparison SourceComparison `json:"comparison"`
}

// FactPackRequest gathers broader context and extracts fact-like details.
type FactPackRequest struct {
	Query      string `json:"query"`
	Language   string `json:"language,omitempty"`
	TimeRange  string `json:"time_range,omitempty"`
	ReadTopN   int    `json:"read_top_n,omitempty"`
	QuoteLimit int    `json:"quote_limit,omitempty"`
}

// ExtractedFactPack is a structured collection of extracted details.
type ExtractedFactPack struct {
	Dates    []string `json:"dates"`
	Entities []string `json:"entities"`
	Quotes   []string `json:"quotes"`
}

// FactPackResponse combines multi-search, source reads, and extracted details.
type FactPackResponse struct {
	Query       string            `json:"query"`
	QuickLook   QuickLookResponse `json:"quick_look"`
	Sources     []SourceNote      `json:"sources"`
	Extracted   ExtractedFactPack `json:"extracted"`
	SourceCount int               `json:"source_count"`
}

// MonitorQueryRequest polls a query and returns a stable fingerprint.
type MonitorQueryRequest struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories,omitempty"`
	Language   string   `json:"language,omitempty"`
	TimeRange  string   `json:"time_range,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// CategoryMonitor summarizes a monitored category.
type CategoryMonitor struct {
	Category    string         `json:"category"`
	Fingerprint string         `json:"fingerprint"`
	TopResults  []SearchResult `json:"top_results"`
}

// MonitorQueryResponse returns per-category signatures for polling.
type MonitorQueryResponse struct {
	Query       string            `json:"query"`
	Categories  []string          `json:"categories"`
	Fingerprint string            `json:"fingerprint"`
	Results     []CategoryMonitor `json:"results"`
}

// SearchThenExtractRequest searches, reads, and extracts requested fields.
type SearchThenExtractRequest struct {
	Query     string   `json:"query"`
	Fields    []string `json:"fields"`
	Language  string   `json:"language,omitempty"`
	TimeRange string   `json:"time_range,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	ReadTopN  int      `json:"read_top_n,omitempty"`
}

// ExtractedDocument captures extracted values from one source.
type ExtractedDocument struct {
	Result SearchResult        `json:"result"`
	Fields map[string][]string `json:"fields"`
}

// SearchThenExtractResponse returns extracted values from read results.
type SearchThenExtractResponse struct {
	Query     string              `json:"query"`
	Fields    []string            `json:"fields"`
	Search    SearchResponse      `json:"search"`
	Documents []ExtractedDocument `json:"documents"`
}

// SearchThenRankRequest searches then reranks results for an intent.
type SearchThenRankRequest struct {
	Query     string `json:"query"`
	Intent    string `json:"intent"`
	Language  string `json:"language,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// RankedResult adds heuristic ranking metadata.
type RankedResult struct {
	Result  SearchResult `json:"result"`
	Score   int          `json:"score"`
	Reasons []string     `json:"reasons,omitempty"`
}

// SearchThenRankResponse returns heuristic reranking for an intent.
type SearchThenRankResponse struct {
	Query  string         `json:"query"`
	Intent string         `json:"intent"`
	Search SearchResponse `json:"search"`
	Ranked []RankedResult `json:"ranked"`
}

// VisualQuickLookRequest is a compact media-first snapshot request.
type VisualQuickLookRequest struct {
	Query     string `json:"query"`
	Language  string `json:"language,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// VisualQuickLookResponse is a compact media-focused response.
type VisualQuickLookResponse struct {
	Query    string         `json:"query"`
	Category string         `json:"category"`
	Limit    int            `json:"limit"`
	Results  []SearchResult `json:"results"`
}

// SmartSearchRequest unifies multiple search workflows behind one mode.
type SmartSearchRequest struct {
	Query      string   `json:"query"`
	Mode       string   `json:"mode"`
	Categories []string `json:"categories,omitempty"`
	Intent     string   `json:"intent,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Language   string   `json:"language,omitempty"`
	TimeRange  string   `json:"time_range,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	ReadTopN   int      `json:"read_top_n,omitempty"`
	MaxSources int      `json:"max_sources,omitempty"`
	Location   string   `json:"location,omitempty"`
}

// SearchPresetRequest runs a named preset for a query.
type SearchPresetRequest struct {
	Query     string `json:"query"`
	Language  string `json:"language,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// SearchAndReadRequest searches and then reads a selected result.
type SearchAndReadRequest struct {
	Query       string   `json:"query"`
	Category    string   `json:"category,omitempty"`
	Engines     []string `json:"engines,omitempty"`
	Site        string   `json:"site,omitempty"`
	Language    string   `json:"language,omitempty"`
	TimeRange   string   `json:"time_range,omitempty"`
	Page        int      `json:"page,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	ResultIndex int      `json:"result_index,omitempty"`
}

// SearchAndReadResponse combines search results with the selected page read.
type SearchAndReadResponse struct {
	Search        SearchResponse   `json:"search"`
	SelectedIndex int              `json:"selected_index"`
	Selected      *SearchResult    `json:"selected,omitempty"`
	Read          *URLReadResponse `json:"read,omitempty"`
}
