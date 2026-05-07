package search

import (
	urlpkg "net/url"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type searxResult struct {
	Title        string `json:"title"`
	URL          string `json:"url"`
	Content      string `json:"content"`
	Engine       string `json:"engine"`
	Source       string `json:"source"`
	Thumbnail    string `json:"thumbnail"`
	ThumbnailSrc string `json:"thumbnail_src"`
	ImgSrc       string `json:"img_src"`
	ContentURL   string `json:"content_url"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     string `json:"duration"`
	PublishedAt  string `json:"publishedDate"`
	Channel      string `json:"channel"`
	Author       string `json:"author"`
}

func normalizeResults(results []searxResult, limit int) []types.SearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	out := make([]types.SearchResult, 0, limit)
	for _, item := range results {
		if len(out) >= limit {
			break
		}
		url := strings.TrimSpace(item.URL)
		title := strings.TrimSpace(item.Title)
		if url == "" || title == "" {
			continue
		}
		domain := ""
		if parsed, err := urlpkg.Parse(url); err == nil {
			domain = strings.TrimSpace(parsed.Hostname())
		}
		thumbnail := firstNonEmpty(item.Thumbnail, item.ThumbnailSrc)
		contentURL := firstNonEmpty(item.ContentURL, item.ImgSrc)
		out = append(out, types.SearchResult{
			Title:        title,
			URL:          url,
			Snippet:      strings.TrimSpace(item.Content),
			Engine:       strings.TrimSpace(item.Engine),
			Source:       strings.TrimSpace(item.Source),
			Domain:       domain,
			ThumbnailURL: strings.TrimSpace(thumbnail),
			ContentURL:   strings.TrimSpace(contentURL),
			Width:        item.Width,
			Height:       item.Height,
			Duration:     strings.TrimSpace(item.Duration),
			PublishedAt:  strings.TrimSpace(item.PublishedAt),
			Channel:      strings.TrimSpace(item.Channel),
			Author:       strings.TrimSpace(item.Author),
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
