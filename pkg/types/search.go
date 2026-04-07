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
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	Engine  string `json:"engine,omitempty"`
	Source  string `json:"source,omitempty"`
}

// SearchResponse is returned by the web_search tool.
type SearchResponse struct {
	Query       string         `json:"query"`
	Category    string         `json:"category,omitempty"`
	Engines     []string       `json:"engines,omitempty"`
	Site        string         `json:"site,omitempty"`
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
	ResultCount int            `json:"result_count"`
	Results     []SearchResult `json:"results"`
	Cached      bool           `json:"cached"`
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
