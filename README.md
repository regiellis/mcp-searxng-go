# mcp-searxng-go

## Index

Sections by line range. Read with an offset to jump to one. Regenerate after edits.

  - Core Capabilities: lines 27-62
  - Design Philosophy: Why Go?: lines 64-73
  - Features Overview: lines 75-88
  - Project Layout: lines 90-109
  - Requirements: lines 111-118
  - Configuration: lines 120-175
  - Build & Run: lines 177-199
  - First Run: lines 201-234
  - MCP Tools Details: lines 236-566
  - Security Notes: lines 568-586
  - Development Workflow: lines 588-605
  - License: lines 607-609

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org/)
[![Release](https://img.shields.io/github/v/release/regiellis/mcp-searxng-go)](https://github.com/regiellis/mcp-searxng-go/releases)

`mcp-searxng-go` is a production-oriented **Model Context Protocol (MCP) server** written in Go 1.25. It acts as a dedicated gateway, exposing several focused search and reading tools backed by a configured SearXNG instance.

### Core Capabilities

This service exposes the following primary tools via the MCP interface:

*   **web\_search**: Executes general web searches with normalized results.
*   **image\_search**: Performs image-specific searches.
*   **video\_search**: Performs video-specific searches.
*   **news\_search**: Performs news-specific searches.
*   **search\_with\_engines**: Runs a search against specific SearXNG engines.
*   **search\_with\_site\_filter**: Adds a `site:` filter convenience wrapper.
*   **category\_search**: Searches one SearXNG category (packages, repos, q&a, science, map, music, files, social media, ...).
*   **package\_search**: Searches package registries (pkg.go.dev, crates.io, npm, pypi, docker hub, ...) with exact name matches first.
*   **multi\_search**: Runs one query across multiple categories.
*   **quick\_look**: Returns a compact snapshot across general, news, images, and videos.
*   **deep\_research**: Expands a query with broader search plus automatic source reads.
*   **scholar\_search**: Searches the `science` category (arxiv, pubmed, europepmc, semantic scholar, crossref) and returns structured paper records.
*   **local\_search**: Searches the `map` category for geocoded places plus `general` for listing pages with hours and reviews.
*   **shopping\_search**: Uses a shopping-oriented query preset for products, pricing, and reviews.
*   **recent\_search**: Applies recency defaults for latest updates.
*   **answer\_search**: Returns a compact answer-oriented summary backed by top sources.
*   **compare\_sources**: Surfaces agreement and disagreement notes across top sources.
*   **fact\_pack**: Bundles source reads with extracted dates, entities, and quotes.
*   **monitor\_query**: Returns stable fingerprints for polling result changes over time.
*   **search\_then\_extract**: Extracts requested fields from the top read results.
*   **search\_then\_rank**: Reranks results for intents like official docs, examples, or primary sources.
*   **image\_quick\_look**: Returns an image-first compact result set with media metadata when available.
*   **video\_quick\_look**: Returns a video-first compact result set with media metadata when available.
*   **find\_official\_docs**: Preset for official documentation discovery.
*   **find\_latest\_news**: Preset for fresh news discovery.
*   **find\_examples**: Preset for examples, demos, and tutorials.
*   **find\_primary\_sources**: Preset for source-of-record material.
*   **smart\_search**: Unified entrypoint with multiple search modes.
*   **search\_and\_read**: Searches first, then reads the selected result in one call.
*   **url\_read**: Fetches and parses the content of a public URL.

---

### Design Philosophy: Why Go?

This service was intentionally designed to minimize operational complexity and maximize reliability.

*   **Small Dependency Surface:** Keeps the codebase lean and easy to audit.
*   **Static Builds:** Produces static Linux binaries where practical, simplifying deployment.
*   **Explicit Control:** Offers granular control over timeouts and transport layers.
*   **Auditable:** Makes it easy to track and verify all outbound network behavior.

---

### Features Overview

*   **Transport:** Supports MCP over `stdio` and HTTP transport.
*   **Search:** Seamlessly integrates with SearXNG, providing normalized results.
*   **URL Reading:** Robust public URL reading with built-in Server-Side Request Forgery (SSRF) protections.
*   **Research Modes:** Supports both fast triage (`quick_look`) and heavier source collection (`deep_research`).
*   **Task Modes:** Supports answer synthesis, extraction, ranking, comparison, monitoring, and preset search intents.
*   **Caching:** Bounded in-memory TTL cache for fast lookups.
*   **Error Handling:** Structured JSON-RPC error responses.
*   **Configuration:** Flexible YAML configuration with full support for environment variable overrides.
*   **Logging:** Structured logging implemented using `log/slog`.
*   **QoL:** Startup logs expose preferred MCP URLs, `/tools`, and `/debug` endpoints.

---

### Project Layout

The project is organized logically to separate concerns:

```
mcp-searxng-go/
├── cmd/server/      # Main server entry points
├── internal/config/ # Configuration loading and structs
├── internal/mcp/    # MCP protocol handling logic
├── internal/search/ # SearXNG interaction layer
├── internal/fetch/  # HTTP request execution
├── internal/cache/  # In-memory caching logic
├── internal/security/ # Rate limiting, domain checks, etc.
├── pkg/client/      # Client-facing packages
├── pkg/types/       # Shared data structures
├── configs/         # Default configuration files
└── build/           # Compiled binaries
```

---

### Requirements

Before running, ensure you have the following:

*   **Runtime:** Go version `1.25` or newer.
*   **Dependency:** A reachable SearXNG instance.

---

### Configuration

The default configuration file location is `configs/config.yaml`.

#### Default `config.yaml` Structure

```yaml
searxng:
  base_url: "http://127.0.0.1:7777"
  timeout: 10s
  default_language: "all"
  default_time_range: ""
  max_limit: 10 # Max results per search

server:
  mode: "http"              # Options: "stdio" or "http"
  address: "0.0.0.0:8081"   # Listening address for HTTP mode
  public_base_url: ""       # Optional: external base URL used for logs/debug output and media download_url links (falls back to the request origin)
  read_timeout: 15s
  write_timeout: 15s
  log_level: "info" # Options: "debug", "info", "warn", "error"

fetch:
  timeout: 15s      # Max time for a single fetch
  max_body_size: 2MB
  max_text_chars: 12000
  max_redirects: 4
  allowed_schemes: [http, https]

cache:
  enabled: true
  ttl_search: 2m    # Time-To-Live for web_search results
  ttl_url_read: 5m  # Time-To-Live for url_read results
  max_entries: 256  # Total cache size limit

security:
  block_private_networks: false # Default: allow LAN traffic
  allow_domains: []
  deny_domains: []
```

#### Environment Variable Overrides

You can override any key configuration via environment variables, prefixed with `MCP_`.

| Config Key | Environment Variable | Example |
| :--- | :--- | :--- |
| `searxng.base_url` | `MCP_SEARXNG_BASE_URL` | `http://search.mycorp.com` |
| `server.mode` | `MCP_SERVER_MODE` | `http` |
| `server.address` | `MCP_SERVER_ADDRESS` | `:8082` |
| `server.public_base_url` | `MCP_SERVER_PUBLIC_BASE_URL` | `http://192.168.4.26:8081` |
| `log_level` | `MCP_LOG_LEVEL` | `debug` |
| `fetch.timeout` | `MCP_FETCH_TIMEOUT` | `20s` |
| `security.allow_domains` | `MCP_SECURITY_ALLOW_DOMAINS` | `"google.com,github.com"` |

---

### Build & Run

Use the provided tasks to manage the lifecycle of the service:

```bash
# Compile the server using the default config
task build

# Run the compiled binary with the default settings
./build/mcp-server --config configs/config.yaml

# Run directly from source for development
task run

# Start the server explicitly in HTTP mode
./build/mcp-server --mode http --listen :8081
```

If you know the URL clients should use, set:

```bash
MCP_SERVER_PUBLIC_BASE_URL=http://192.168.4.26:8081 ./build/mcp-server --config configs/config.yaml
```

### First Run

For a typical self-hosted setup:

1. Run SearXNG locally on `127.0.0.1:7777`
2. Start this MCP server
3. Point your MCP client at your machine's real IP, not `127.0.0.1`

Example:

```bash
task build
./build/mcp-server --config configs/config.yaml
```

Quick checks:

```bash
curl http://127.0.0.1:8081/healthz
curl http://127.0.0.1:8081/mcp
curl http://127.0.0.1:8081/tools
curl http://127.0.0.1:8081/debug
# Fetch a file produced by the media tools (path relative to media output_dir);
# media tool responses include this as download_url.
curl -O http://127.0.0.1:8081/files/subs-123456/video.en.srt
```

For browser-based MCP clients such as llama.cpp WebUI, use a LAN URL like:

```text
http://192.168.4.26:8081/mcp
```

Do not use `127.0.0.1` unless the MCP client is running on the same host and in the same network namespace.

### MCP Tools Details

#### web\_search

Executes a general search against SearXNG.

**Input Parameters:**
*   `query` (string): **Required**. The search term.
*   `language` (string): Optional. e.g., `"en"`, `"fr"`.
*   `time_range` (string): Optional. e.g., `"past_24h"`, `"all"`.
*   `page` (integer): Optional. The page number to retrieve.
*   `limit` (integer): Optional. Number of results per page (max 10 by default).

**Output Structure:**
*   `query` (string)
*   `category` (string)
*   `page` (integer)
*   `limit` (integer)
*   `result_count` (integer): results returned after `limit`.
*   `available_results` (integer): SearXNG results on this page before `limit`, so a capped page can be told apart from a thin one.
*   `results` (array of objects), each with:
    *   `title`, `url`, `snippet`, `domain`, `source`, `published_at`
    *   `kind`: `web`, `image`, `video`, `package`, `paper`, `torrent`, `file`, or `map` (from SearXNG's result template)
    *   `category`, `engine`, `engines` (every engine that returned this URL; Brave API duplicates are folded in), `score` (SearXNG relevance, comparable within one response)
    *   media fields when present: `thumbnail_url`, `content_url`, `embed_url`, `audio_url`, `width`, `height`, `resolution`, `format`, `file_size`, `duration`, `views`, `channel`, `author`, `tags`
    *   one detail block matching `kind`: `package` (name, version, maintainer, popularity, homepage, source_code_url, license), `paper` (doi, authors, journal, publisher, pdf_url, html_url, issn, isbn, ...), `torrent` (magnet_link, seeders, leechers), or `location` (latitude, longitude, bounding_box, osm_type, osm_id, address, links). Raw GeoJSON is not passed through.
*   `answers` (array): direct answers from SearXNG answerers/plugins (`answer`, `engine`, `url`), e.g. `sha256 hello`, `avg 1 2 3`, `random uuid`.
*   `infoboxes` (array, max 3): entity cards, usually Wikipedia/Wikidata: `title`, `content`, `image_url`, `engines`, `urls` (with `official` flag), `attributes` (`label`, `value`, Wikidata `entity` ID; max 30).
*   `suggestions` (max 10) and `corrections` (max 5): related queries and spelling fixes, ready to reissue.
*   `unresponsive_engines` (array of `engine`, `reason`): engines that failed or were suspended for this query.
*   `cached` (boolean)

Every field except `query`, `page`, `limit`, `result_count`, `available_results`, `results`, and `cached` is omitted when empty. Composite tools (`multi_search`, `quick_look`, `deep_research`, `answer_search`, ...) embed this same response per category, so they carry the same fields.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "web_search",
    "arguments": {
      "query": "golang",
      "limit": 5
    }
  }
}
```

#### image\_search
Identical input shape to `web_search`, but queries SearXNG with `categories=images`.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "image_search",
    "arguments": {
      "query": "golden retriever",
      "limit": 5
    }
  }
}
```

#### video\_search
Identical input shape to `web_search`, but queries SearXNG with `categories=videos`.

#### quick\_look
Returns a compact, multi-category snapshot intended for fast “what’s here?” checks. By default it searches `general`, `images`, `videos`, and `news` with a smaller per-category limit.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "quick_look",
    "arguments": {
      "query": "golang release",
      "limit": 3
    }
  }
}
```

#### deep\_research
Runs a broader general search, a parallel news search, then automatically reads the top general result pages so clients get source text back in one tool call.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "deep_research",
    "arguments": {
      "query": "golang 1.25 generics changes",
      "general_limit": 5,
      "news_limit": 3,
      "max_sources": 2
    }
  }
}
```

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "video_search",
    "arguments": {
      "query": "golang tutorial",
      "limit": 5
    }
  }
}
```

#### news\_search
Identical input shape to `web_search`, but queries SearXNG with `categories=news`.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "news_search",
    "arguments": {
      "query": "OpenAI",
      "time_range": "day",
      "limit": 5
    }
  }
}
```

#### search\_with\_engines
Searches using specific SearXNG engines.

**Additional Input Parameters:**
*   `engines` (array of strings): **Required**. Example: `["google", "duckduckgo"]`, `["arxiv"]`, `["openstreetmap"]`
*   `category` (string): Optional, any [supported category](#category_search); it labels the response only. It is not sent to SearXNG with `engines`, because SearXNG adds every engine of a sent category to the list (`engines=arxiv&categories=general` returns mostly general web results). The Brave API merge is also skipped, so only the named engines answer.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "search_with_engines",
    "arguments": {
      "query": "llama.cpp",
      "engines": ["google", "duckduckgo"],
      "category": "general",
      "limit": 5
    }
  }
}
```

#### search\_with\_site\_filter
Searches using a `site:` filter.

**Additional Input Parameters:**
*   `site` (string): **Required**. Example: `"github.com"`
*   `category` (string): Optional, any [supported category](#category_search). Defaults to `general`.

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "search_with_site_filter",
    "arguments": {
      "query": "mcp",
      "site": "github.com",
      "limit": 5
    }
  }
}
```

#### category\_search
Searches one SearXNG category. Input is the same as `web_search` plus a **required** `category`. An unsupported category returns an `invalid params` error whose `data.supported` lists the valid ones.

Supported categories, and the engines that answered for each on the live instance (SearXNG 2026.11.5, checked 2026-11-06):

| Category | Engines | Result `kind` |
| :--- | :--- | :--- |
| `general`, `images`, `videos`, `news` | web and media engines | `web`, `image`, `video` |
| `music` | youtube, genius, soundcloud, mixcloud | `web`, `video` |
| `it` | github, docker hub, stackoverflow, mdn | `package`, `web` |
| `packages` | pkg.go.dev, crates.io, lib.rs, npm, pypi, docker hub, packagist, pub.dev | `package` |
| `repos` | github, gitlab, codeberg, huggingface spaces | `package`, `web` |
| `q&a` | stackoverflow and Discourse forums | `web` |
| `science` | arxiv, pubmed, europepmc, semantic scholar, openaire | `paper`, `web` |
| `scientific publications` | the paper-only subset of `science`, plus crossref | `paper` |
| `files` | wikicommons files, torrent indexes | `file`, `torrent` |
| `map` | openstreetmap, photon | `map` |
| `social media` | mastodon, lemmy | `web` |

`shopping` is not supported: no shopping engine is enabled by default, so the category returns nothing. `shopping_search` stays a general-web preset. Which engines answer depends on the SearXNG instance's `settings.yml`.

#### package\_search
Searches the `packages` category. Input is the same as `web_search`. Before the limit is applied, package results whose name matches the query are moved to the front: exact name first, then last path segment (`golang.org/x/sync/errgroup` for `errgroup`), then substring. The rest keep SearXNG's order. Without this, SearXNG's per-registry interleaving can push the package an agent means below the cut. Each result carries a `package` block.

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "tools/call",
  "params": {
    "name": "package_search",
    "arguments": { "query": "errgroup", "limit": 3 }
  }
}
```

#### multi\_search
Runs one query across multiple categories and returns grouped results.

**Input Parameters:**
*   `query` (string): **Required**
*   `categories` (array of strings): Optional, any [supported category](#category_search). Defaults to `["general", "images", "videos", "news"]`
*   `language`, `time_range`, `page`, `limit`: Same meaning as `web_search`

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "tools/call",
  "params": {
    "name": "multi_search",
    "arguments": {
      "query": "golang",
      "categories": ["general", "videos", "news"],
      "limit": 3
    }
  }
}
```

#### search\_and\_read
Runs a search and then fetches the selected result URL with `url_read`.

**Input Parameters:**
*   `query` (string): **Required**
*   `category` (string): Optional. Defaults to `general`
*   `engines` (array of strings): Optional
*   `site` (string): Optional
*   `language`, `time_range`, `page`, `limit`: Same meaning as `web_search`
*   `result_index` (integer): Optional. Defaults to `0`

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "tools/call",
  "params": {
    "name": "search_and_read",
    "arguments": {
      "query": "Go release notes",
      "site": "go.dev",
      "limit": 3,
      "result_index": 0
    }
  }
}
```

Category results depend on which engines the SearXNG instance enables (see [category_search](#category_search)).

#### url\_read
Fetches and parses the content of a specified URL.

**Input Parameters:**
*   `url` (string): **Required**. The full URL to read.

**Output Structure:**
*   `final_url` (string): The URL after all redirects are followed.
*   `content_type` (string): MIME type of the content.
*   `status_code` (integer): HTTP status code received (e.g., 200).
*   `title` (string): The title tag content, if available.
*   `content` (string): The extracted plain text body.
*   `truncated` (boolean): `true` if the content was cut off due to size limits.
*   `cached` (boolean)

**Example MCP call:**

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "url_read",
    "arguments": {
      "url": "https://go.dev"
    }
  }
}
```

---

### Security Notes

The default configuration prioritizes ease-of-use for self-hosted deployments:

*   HTTP Mode is enabled by default.
*   CORS is enabled on the `/mcp` endpoint.
*   Local Testing works out-of-the-box with SearXNG running on `127.0.0.1:7777`.
*   Private Network Blocking is **off** by default.

#### `url_read` Specific Guardrails

Even when relaxed, `url_read` enforces strict security boundaries:
*   **Scheme Enforcement:** Only `http` and `https` are allowed.
*   **Redirection Cap:** Maximum of 4 redirects are followed.
*   **Content Type:** Non-text content is rejected by default.
*   **Sizing:** Body size and extracted text size are strictly bounded by config values.
*   **Limitation:** This tool does not execute JavaScript or perform full browser rendering/automation.

---

### Development Workflow

Use these tasks for common development tasks (assuming a task runner like `make` or `npm scripts`):

**Standard Tasks:**
*   `task build`
*   `task run`
*   `task test`
*   `task tidy`
*   `task lint`
*   `task clean`

**Hardening Tasks:**
*   `task vuln` (Runs dependency vulnerability scan)
*   `task sbom` (Generates Software Bill of Materials)
*   `task release` (Prepares versioning/tagging)

---

### License

This project is released under the **GPL-2.0-only** license.
