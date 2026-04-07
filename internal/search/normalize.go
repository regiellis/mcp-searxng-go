package search

import (
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type searxResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Engine  string `json:"engine"`
	Source  string `json:"source"`
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
		out = append(out, types.SearchResult{
			Title:   title,
			URL:     url,
			Snippet: strings.TrimSpace(item.Content),
			Engine:  strings.TrimSpace(item.Engine),
			Source:  strings.TrimSpace(item.Source),
		})
	}
	return out
}
