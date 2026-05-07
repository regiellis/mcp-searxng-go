package mcp

import "github.com/regiellis/mcp-searxng-go/pkg/types"

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
			Name:        "quick_look",
			Description: "Return a compact snapshot across general, news, images, and videos for rapid triage.",
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
					"limit":      map[string]any{"type": "integer", "minimum": 1},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "deep_research",
			Description: "Run broader web and news searches, then read the top result pages for deeper context.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":         map[string]any{"type": "string"},
					"language":      map[string]any{"type": "string"},
					"time_range":    map[string]any{"type": "string"},
					"general_limit": map[string]any{"type": "integer", "minimum": 1},
					"news_limit":    map[string]any{"type": "integer", "minimum": 1},
					"max_sources":   map[string]any{"type": "integer", "minimum": 1},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "scholar_search",
			Description: "Search for academic and research-oriented sources using scholarly query presets.",
			InputSchema: categorySearchSchema(true),
		},
		{
			Name:        "local_search",
			Description: "Search for local places and nearby results using a location-aware query preset.",
			InputSchema: categorySearchSchema(true),
		},
		{
			Name:        "shopping_search",
			Description: "Search for product, pricing, and buying information using a shopping-oriented query preset.",
			InputSchema: categorySearchSchema(false),
		},
		{
			Name:        "recent_search",
			Description: "Search with recency defaults for latest updates and fresh results.",
			InputSchema: searchSchema(false, false),
		},
		{
			Name:        "answer_search",
			Description: "Search, read the top sources, and return a compact answer-oriented summary packet.",
			InputSchema: answerSearchSchema(),
		},
		{
			Name:        "compare_sources",
			Description: "Search, read several top sources, and surface agreement and disagreement notes.",
			InputSchema: compareSourcesSchema(),
		},
		{
			Name:        "fact_pack",
			Description: "Build a quick fact pack with source reads, dates, entities, and quotes.",
			InputSchema: factPackSchema(),
		},
		{
			Name:        "monitor_query",
			Description: "Return stable fingerprints and top results for polling a query over time.",
			InputSchema: monitorQuerySchema(),
		},
		{
			Name:        "search_then_extract",
			Description: "Search, read top results, and extract requested structured fields.",
			InputSchema: searchThenExtractSchema(),
		},
		{
			Name:        "search_then_rank",
			Description: "Search and rerank results for a caller-specified intent such as official docs or examples.",
			InputSchema: searchThenRankSchema(),
		},
		{
			Name:        "image_quick_look",
			Description: "Return a compact image-first result set with media metadata when available.",
			InputSchema: visualQuickLookSchema(),
		},
		{
			Name:        "video_quick_look",
			Description: "Return a compact video-first result set with media metadata when available.",
			InputSchema: visualQuickLookSchema(),
		},
		{
			Name:        "find_official_docs",
			Description: "Search and rerank for official documentation and primary vendor references.",
			InputSchema: presetSchema(),
		},
		{
			Name:        "find_latest_news",
			Description: "Search for the latest news with freshness defaults and news-oriented ranking.",
			InputSchema: presetSchema(),
		},
		{
			Name:        "find_examples",
			Description: "Search and rerank for tutorials, examples, code samples, and demos.",
			InputSchema: presetSchema(),
		},
		{
			Name:        "find_primary_sources",
			Description: "Search and rerank for canonical, first-party, and source-of-record material.",
			InputSchema: presetSchema(),
		},
		{
			Name:        "smart_search",
			Description: "Unified search entrypoint with modes for category search, quick look, research, extraction, and ranking.",
			InputSchema: smartSearchSchema(),
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

func categorySearchSchema(includeLocation bool) map[string]any {
	properties := map[string]any{
		"query": map[string]any{"type": "string"},
		"categories": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"language":   map[string]any{"type": "string"},
		"time_range": map[string]any{"type": "string"},
		"page":       map[string]any{"type": "integer", "minimum": 1},
		"limit":      map[string]any{"type": "integer", "minimum": 1},
	}
	if includeLocation {
		properties["location"] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func answerSearchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string"},
			"language":    map[string]any{"type": "string"},
			"time_range":  map[string]any{"type": "string"},
			"limit":       map[string]any{"type": "integer", "minimum": 1},
			"read_top_n":  map[string]any{"type": "integer", "minimum": 1},
			"max_summary": map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func compareSourcesSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string"},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
			"read_top_n": map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func factPackSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string"},
			"language":    map[string]any{"type": "string"},
			"time_range":  map[string]any{"type": "string"},
			"read_top_n":  map[string]any{"type": "integer", "minimum": 1},
			"quote_limit": map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func monitorQuerySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
			"categories": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func searchThenExtractSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
			"fields": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
			"read_top_n": map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query", "fields"},
		"additionalProperties": false,
	}
}

func searchThenRankSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string"},
			"intent":     map[string]any{"type": "string"},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query", "intent"},
		"additionalProperties": false,
	}
}

func visualQuickLookSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string"},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func presetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":      map[string]any{"type": "string"},
			"language":   map[string]any{"type": "string"},
			"time_range": map[string]any{"type": "string"},
			"limit":      map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func smartSearchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string"},
			"mode":        map[string]any{"type": "string"},
			"categories":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"intent":      map[string]any{"type": "string"},
			"fields":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"language":    map[string]any{"type": "string"},
			"time_range":  map[string]any{"type": "string"},
			"limit":       map[string]any{"type": "integer", "minimum": 1},
			"read_top_n":  map[string]any{"type": "integer", "minimum": 1},
			"max_sources": map[string]any{"type": "integer", "minimum": 1},
			"location":    map[string]any{"type": "string"},
		},
		"required":             []string{"query", "mode"},
		"additionalProperties": false,
	}
}
