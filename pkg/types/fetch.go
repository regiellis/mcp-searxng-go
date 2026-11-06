package types

// URLReadRequest is the input for the url_read tool.
type URLReadRequest struct {
	URL string `json:"url"`
}

// URLReadResponse is returned by the url_read tool.
type URLReadResponse struct {
	FinalURL    string `json:"final_url"`
	ContentType string `json:"content_type"`
	StatusCode  int    `json:"status_code"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	Truncated   bool   `json:"truncated"`
	Cached      bool   `json:"cached"`
}
