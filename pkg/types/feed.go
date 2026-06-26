package types

// FetchFeedRequest is the input for the fetch_feed tool.
type FetchFeedRequest struct {
	URL   string `json:"url"`
	Limit int    `json:"limit,omitempty"` // max items to return; 0 means a server default
}

// FeedItem is a single entry from an RSS or Atom feed.
type FeedItem struct {
	Title     string `json:"title"`
	Link      string `json:"link,omitempty"`
	Published string `json:"published,omitempty"`
	Summary   string `json:"summary,omitempty"`
	ID        string `json:"id,omitempty"`
}

// FetchFeedResponse is returned by the fetch_feed tool.
type FetchFeedResponse struct {
	SourceURL string     `json:"source_url"`
	FeedTitle string     `json:"feed_title,omitempty"`
	FeedLink  string     `json:"feed_link,omitempty"`
	Format    string     `json:"format"` // rss or atom
	ItemCount int        `json:"item_count"`
	Items     []FeedItem `json:"items"`
}
