package mcp

import "mcp-searxng-go/pkg/types"

func toolDefinitions() []types.ToolDefinition {
	return []types.ToolDefinition{
		{
			Name:        "web_search",
			Description: "Search general web results through the configured SearXNG instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]any{"type": "string"},
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
			Name:        "image_search",
			Description: "Search image results through the configured SearXNG instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]any{"type": "string"},
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
			Name:        "video_search",
			Description: "Search video results through the configured SearXNG instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]any{"type": "string"},
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
