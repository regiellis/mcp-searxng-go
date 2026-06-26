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
		{
			Name:        "download_video",
			Description: "Download a video (or its audio) at best quality via yt-dlp into the server media directory. Returns the saved file path and metadata.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "format": "uri"},
					"format":     map[string]any{"type": "string", "description": "Optional yt-dlp -f format selector; defaults to best video+audio."},
					"audio_only": map[string]any{"type": "boolean", "description": "Extract best audio as mp3 instead of video."},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "transcode_media",
			Description: "Convert or compress a media file already in the server media directory using ffmpeg. Path must reference a file inside that directory.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Path to an existing file inside the media output directory."},
					"format":      map[string]any{"type": "string", "description": "Target container/extension, e.g. mp4, webm, mp3. Default mp4."},
					"video_codec": map[string]any{"type": "string", "description": "ffmpeg video codec, e.g. libx264, libx265, vp9."},
					"audio_codec": map[string]any{"type": "string", "description": "ffmpeg audio codec, e.g. aac, libmp3lame, libopus."},
					"crf":         map[string]any{"type": "integer", "minimum": 0, "maximum": 51, "description": "Constant rate factor (lower = higher quality)."},
					"max_width":   map[string]any{"type": "integer", "minimum": 1, "description": "Downscale so width does not exceed this value."},
					"output_name": map[string]any{"type": "string", "description": "Optional output base filename (no directories)."},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "download_subtitles",
			Description: "Download subtitle/caption tracks for a video URL via yt-dlp and save them to the server media directory for further work.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":            map[string]any{"type": "string", "format": "uri"},
					"language":       map[string]any{"type": "string", "description": "Subtitle language selector (yt-dlp --sub-langs). Default en."},
					"format":         map[string]any{"type": "string", "description": "Convert subtitles to this format, e.g. srt, vtt. Default srt."},
					"auto_generated": map[string]any{"type": "boolean", "description": "Include auto-generated captions when no human subtitles exist."},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "clean_subtitles",
			Description: "Clean a downloaded subtitle/transcript file using an LLM: removes intros, outros, like/subscribe asks, sponsor reads, ads, and caption filler while preserving the substantive on-topic content (not a brief summary). Path must reference a file inside the media directory (e.g. from download_subtitles). The call may take a while for long transcripts. Requires DEEPSEEK_API_KEY.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "Path to a subtitle/transcript file inside the media output directory."},
					"topic": map[string]any{"type": "string", "description": "Optional topic hint; material unrelated to it is treated as removable."},
					"save":  map[string]any{"type": "boolean", "description": "Also write the cleaned text to a .clean.txt file in the media directory."},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "transcript_chapters",
			Description: "Segment a timestamped subtitle/transcript file (SRT or VTT) into time-bounded chapters with start/end times and a text preview, using caption timing only. This is deterministic structural segmentation (silence gaps and length), not an LLM summary, and requires no API key. Path must reference a file inside the media directory (e.g. from download_subtitles).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Path to a timestamped SRT/VTT file inside the media output directory."},
					"min_seconds": map[string]any{"type": "number", "minimum": 0, "description": "Merge chapters shorter than this many seconds. Default 60."},
					"gap_seconds": map[string]any{"type": "number", "minimum": 0, "description": "Silence gap (seconds) treated as a candidate section boundary. Default 2.5."},
					"max_seconds": map[string]any{"type": "number", "minimum": 0, "description": "Force a chapter split once it reaches this many seconds. Default 300."},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "probe_media",
			Description: "Inspect a media file already in the server media directory with ffprobe: returns container format, duration, size, bitrate, and per-stream codec/resolution/language metadata. Use this to decide how (or whether) to transcode before re-encoding. Path must reference a file inside that directory.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to an existing file inside the media output directory (e.g. from download_video)."},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "read_media_file",
			Description: "Read the contents of a file inside the server media directory, such as a subtitle returned by download_subtitles. Returns UTF-8 text when possible, otherwise base64. Use this to retrieve files the other media tools saved server-side.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "Path to a file inside the media output directory (e.g. a path from download_subtitles)."},
					"max_bytes": map[string]any{"type": "integer", "minimum": 1, "description": "Optional cap on bytes returned; clamped to a server ceiling. Default 1 MiB."},
				},
				"required":             []string{"path"},
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
