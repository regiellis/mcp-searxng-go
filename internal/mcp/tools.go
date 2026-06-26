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
			Description: "Search, read the top sources, and return a compact answer-oriented summary packet. Deterministic by default (no LLM). Pass synthesize=true to additionally compose a written, cited answer from the read sources via the LLM (requires DEEPSEEK_API_KEY).",
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
			Description: "Fetch a public URL with strict safety checks and readable text extraction. Handles HTML, plain text, JSON/XML, and PDF documents (text-layer PDFs; scanned/image-only PDFs yield little text).",
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
			Description: "Download a video (or its audio) at best quality via yt-dlp into the server media directory. Returns the saved file path and metadata. This can take a while for large videos; pass async=true to run it in the background and get a job_id to poll with media_job_status instead of blocking.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "format": "uri"},
					"format":     map[string]any{"type": "string", "description": "Optional yt-dlp -f format selector; defaults to best video+audio."},
					"audio_only": map[string]any{"type": "boolean", "description": "Extract best audio as mp3 instead of video."},
					"async":      map[string]any{"type": "boolean", "description": "Run in the background and return a job_id immediately; poll media_job_status for the result."},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "transcode_media",
			Description: "Convert or compress a media file already in the server media directory using ffmpeg. Path must reference a file inside that directory. Transcoding can take a while; pass async=true to run it in the background and get a job_id to poll with media_job_status instead of blocking.",
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
					"async":       map[string]any{"type": "boolean", "description": "Run in the background and return a job_id immediately; poll media_job_status for the result."},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "download_subtitles",
			Description: "Download subtitle/caption tracks for a video URL via yt-dlp and save them to the server media directory for further work. This can take a while; pass async=true to run it in the background and get a job_id to poll with media_job_status instead of blocking.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":            map[string]any{"type": "string", "format": "uri"},
					"language":       map[string]any{"type": "string", "description": "Subtitle language selector (yt-dlp --sub-langs). Default en."},
					"format":         map[string]any{"type": "string", "description": "Convert subtitles to this format, e.g. srt, vtt. Default srt."},
					"auto_generated": map[string]any{"type": "boolean", "description": "Include auto-generated captions when no human subtitles exist."},
					"async":          map[string]any{"type": "boolean", "description": "Run in the background and return a job_id immediately; poll media_job_status for the result."},
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
			Name:        "translate_subtitles",
			Description: "Translate a downloaded subtitle/transcript file into another language using an LLM, preserving the full meaning and substance (not a summary). Output is translated prose, not a re-timed subtitle file. Path must reference a file inside the media directory (e.g. from download_subtitles). The call may take a while for long transcripts. Requires DEEPSEEK_API_KEY.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":            map[string]any{"type": "string", "description": "Path to a subtitle/transcript file inside the media output directory."},
					"target_language": map[string]any{"type": "string", "description": "Language to translate into, e.g. \"Spanish\", \"French\", \"Japanese\"."},
					"save":            map[string]any{"type": "boolean", "description": "Also write the translation to a .<language>.txt file in the media directory."},
				},
				"required":             []string{"path", "target_language"},
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
		{
			Name:        "fetch_feed",
			Description: "Fetch and parse an RSS 2.0 or Atom feed into structured items (title, link, published, summary, id), through the same SSRF-safe fetcher as url_read. Useful for news, blog, and release feeds. Deterministic, no LLM.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":   map[string]any{"type": "string", "format": "uri", "description": "Feed URL (RSS or Atom)."},
					"limit": map[string]any{"type": "integer", "minimum": 1, "description": "Max items to return. Default 20, capped at 100."},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "save_research",
			Description: "Persist or append to a research session: a titled, timestamped record of an investigation that survives across calls. Omit id to start a new session (returns its id); pass an existing id to append a note and update title/query/tags. Deterministic, no LLM.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Existing session id to append to. Omit to create a new session."},
					"title":   map[string]any{"type": "string", "description": "Session title (set on create; updates an existing session when provided)."},
					"query":   map[string]any{"type": "string", "description": "The research question or query this session is about."},
					"note":    map[string]any{"type": "string", "description": "A finding or note to append as a timestamped entry."},
					"sources": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional supporting URLs for this note."},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags merged into the session."},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "get_research",
			Description: "Retrieve a stored research session by id, including all its timestamped notes and sources.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "The research session id (from save_research or list_research)."},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "list_research",
			Description: "List stored research sessions (id, title, query, tags, note count, last updated), newest first.",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			Name:        "export_report",
			Description: "Render a research report as markdown and save it to the storage directory, returning the file path and the rendered content. Pass an existing research session id to render that session (its notes become sections), or provide title/query/summary/sections/sources directly. Deterministic formatting, no LLM.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Research session id to render. Omit to render the provided fields instead."},
					"title":   map[string]any{"type": "string", "description": "Report title (defaults to the session title or query)."},
					"query":   map[string]any{"type": "string", "description": "The research question, shown near the top."},
					"summary": map[string]any{"type": "string", "description": "Optional summary/answer section."},
					"sections": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"heading": map[string]any{"type": "string"},
								"body":    map[string]any{"type": "string"},
							},
							"required":             []string{"heading", "body"},
							"additionalProperties": false,
						},
						"description": "Optional headed content blocks.",
					},
					"sources": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional source URLs listed at the end."},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "media_job_status",
			Description: "Check the status of a background media job started with async=true (download_video, transcode_media, or download_subtitles). Returns status (running, completed, or failed); when completed, the result field holds the same payload the tool would have returned synchronously.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"job_id": map[string]any{"type": "string", "description": "The job_id returned when a media tool was called with async=true."},
				},
				"required":             []string{"job_id"},
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
			"synthesize":  map[string]any{"type": "boolean", "description": "Also compose a written, cited answer from the read sources via the LLM. Requires DEEPSEEK_API_KEY; off by default."},
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
