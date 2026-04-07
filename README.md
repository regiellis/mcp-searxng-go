# mcp-searxng-go

`mcp-searxng-go` is a production-oriented MCP server in Go 1.25 that exposes two focused tools through a configured SearXNG instance:

* `web_search`
* `url_read`

The project is intentionally small. It prefers standard library components, blocks unsafe network targets by default, and keeps deployment to a single binary plus a YAML config file.

## Why Go

This service is designed around low operational complexity:

* small dependency surface
* static Linux builds where practical
* explicit timeout and transport control
* easy auditing of outbound network behavior

## Features

* stdio MCP transport by default
* optional HTTP MCP transport
* SearXNG-backed web search with normalized results
* public URL reading with SSRF protections
* bounded in-memory TTL cache
* structured JSON-RPC errors
* YAML config with environment variable overrides
* structured logs via `log/slog`

## Project Layout

```text
cmd/server/
internal/config/
internal/mcp/
internal/search/
internal/fetch/
internal/cache/
internal/security/
pkg/client/
pkg/types/
configs/
build/
```

## Requirements

* Go 1.25
* a reachable SearXNG instance

## Configuration

The default config path is [configs/config.yaml](/mnt/GARAGE/GO/PROJECTS/mcp-searxng-go/configs/config.yaml).

```yaml
searxng:
  base_url: "http://127.0.0.1:8080"
  timeout: 10s
  default_language: "all"
  default_time_range: ""
  max_limit: 10

server:
  mode: "stdio"
  address: ":8081"
  read_timeout: 15s
  write_timeout: 15s
  log_level: "info"

fetch:
  timeout: 15s
  max_body_size: 2MB
  max_text_chars: 12000
  max_redirects: 4
  allowed_schemes: [http, https]

cache:
  enabled: true
  ttl_search: 2m
  ttl_url_read: 5m
  max_entries: 256

security:
  block_private_networks: false
  allow_domains: []
  deny_domains: []
```

Useful environment overrides include:

* `MCP_SEARXNG_BASE_URL`
* `MCP_SERVER_MODE`
* `MCP_SERVER_ADDRESS`
* `MCP_LOG_LEVEL`
* `MCP_FETCH_TIMEOUT`
* `MCP_FETCH_MAX_BODY_SIZE`
* `MCP_SECURITY_ALLOW_DOMAINS`
* `MCP_SECURITY_DENY_DOMAINS`

## Build And Run

```bash
task build
./build/mcp-server --config configs/config.yaml
```

For a quick development loop:

```bash
task run
```

To run in HTTP mode:

```bash
./build/mcp-server --mode http --listen :8081
```

## MCP Tools

### `web_search`

Input:

* `query` required string
* `language` optional string
* `time_range` optional string
* `page` optional integer
* `limit` optional integer

Output:

* normalized query
* page and enforced limit
* result count
* normalized results with title, URL, snippet, and engine or source

### `url_read`

Input:

* `url` required string

Output:

* final URL
* content type
* status code
* extracted title when available
* readable text content
* truncation flag

## Security Notes

The default config is intentionally easy to use for self-hosted setups:

* HTTP mode is enabled by default
* CORS is enabled on `/mcp`
* local SearXNG on `127.0.0.1:7777` works out of the box
* private-network blocking is off by default

If you want stricter behavior, enable it in config.

`url_read` still keeps some basic guardrails:

* only `http` and `https` are allowed
* redirects are capped
* non-text content is rejected
* body size and extracted text size are bounded

This does not execute JavaScript, render pages, or attempt browser automation.

## Development

Common tasks:

```bash
task build
task run
task test
task tidy
task lint
task clean
```

Optional hardening tasks:

```bash
task vuln
task sbom
task release
```

## License

GPL-2.0-only
