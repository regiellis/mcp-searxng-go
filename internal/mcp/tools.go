package mcp

import "mcp-searxng-go/pkg/types"

func toolDefinitions() []types.ToolDefinition {
	return []types.ToolDefinition{
		{
			Name:        "web_search",
			Description: "Search general web results through the configured SearXNG instance.",
			InputSchema: searchSchema(false, false),
		},
		{
			Name:        "image_search",
			Description: "Search image results through the configured SearXNG instance.",
			InputSchema: searchSchema(false, false),
		},
		{
			Name:        "video_search",
			Description: "Search video results through the configured SearXNG instance.",
			InputSchema: searchSchema(false, false),
		},
		{
			Name:        "news_search",
			Description: "Search news results through the configured SearXNG instance.",
			InputSchema: searchSchema(false, false),
		},
		{
			Name:        "search_with_engines",
			Description: "Search using specific SearXNG engines.",
			InputSchema: searchSchema(true, false),
		},
		{
			Name:        "search_with_site_filter",
			Description: "Search with a site: filter convenience wrapper.",
			InputSchema: searchSchema(false, true),
		},
		{
			Name:        "multi_search",
			Description: "Run one query across multiple categories such as general, images, videos, and news.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"categories": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"language":   map[string]any{"type": "string"},
					"time_range": map[string]any{"type": "string"},
					"page":       map[string]any{"type": "integer", "minimum": 1},
					"limit":      map[string]any{"type": "integer", "minimum": 1},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "search_and_read",
			Description: "Search first, then read the selected result URL in one tool call.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"category": map[string]any{
						"type": "string",
						"enum": []string{"general", "images", "videos", "news"},
					},
					"engines": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"site":       map[string]any{"type": "string"},
					"language":   map[string]any{"type": "string"},
					"time_range": map[string]any{"type": "string"},
					"page":       map[string]any{"type": "integer", "minimum": 1},
					"limit":      map[string]any{"type": "integer", "minimum": 1},
					"result_index": map[string]any{
						"type":    "integer",
						"minimum": 0,
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "url_read",
			Description: "Fetch a public URL with strict safety checks and readable text extraction.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "format": "uri"},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
		},
	}
}

func searchSchema(includeEngines, includeSite bool) map[string]any {
	properties := map[string]any{
		"query":      map[string]any{"type": "string"},
		"language":   map[string]any{"type": "string"},
		"time_range": map[string]any{"type": "string"},
		"page":       map[string]any{"type": "integer", "minimum": 1},
		"limit":      map[string]any{"type": "integer", "minimum": 1},
	}
	if includeEngines {
		properties["engines"] = map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		}
		properties["category"] = map[string]any{
			"type": "string",
			"enum": []string{"general", "images", "videos", "news"},
		}
	}
	if includeSite {
		properties["site"] = map[string]any{"type": "string"}
		properties["category"] = map[string]any{
			"type": "string",
			"enum": []string{"general", "images", "videos", "news"},
		}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}
