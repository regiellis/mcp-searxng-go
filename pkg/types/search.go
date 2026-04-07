package types

// SearchRequest is the input for the web_search tool.
type SearchRequest struct {
	Query     string `json:"query"`
	Category  string `json:"category,omitempty"`
	Language  string `json:"language,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Page      int    `json:"page,omitempty"`
	Limit     int    `json:"limit,omitempty"`
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
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
	ResultCount int            `json:"result_count"`
	Results     []SearchResult `json:"results"`
	Cached      bool           `json:"cached"`
}
