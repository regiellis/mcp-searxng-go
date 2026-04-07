# mcp-searxng-go

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
*   **multi\_search**: Runs one query across multiple categories.
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
  public_base_url: ""       # Optional: preferred external base URL for logs/debug output
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
*   `result_count` (integer)
*   `results` (array of objects): Each object contains `title`, `url`, `snippet`, `engine`, and `source`
*   `cached` (boolean)

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
*   `engines` (array of strings): **Required**. Example: `["google", "duckduckgo"]`
*   `category` (string): Optional. One of `general`, `images`, `videos`, `news`

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
*   `category` (string): Optional. One of `general`, `images`, `videos`, `news`

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

#### multi\_search
Runs one query across multiple categories and returns grouped results.

**Input Parameters:**
*   `query` (string): **Required**
*   `categories` (array of strings): Optional. Defaults to `["general", "images", "videos", "news"]`
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

Image and video availability depends on those categories being enabled in your SearXNG instance.

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
