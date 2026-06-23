package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/client"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type searxResponse struct {
	Query   string        `json:"query"`
	Results []searxResult `json:"results"`
}

// BraveSearcher fetches supplemental results to merge with SearXNG output.
// It is satisfied by *brave.Client and kept as an interface so the search
// package stays decoupled and easily testable.
type BraveSearcher interface {
	Search(ctx context.Context, category string, req types.SearchRequest, limit int) ([]types.SearchResult, error)
}

// Client calls SearXNG and normalizes results.
type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	defaultLanguage  string
	defaultTimeRange string
	maxLimit         int
	logger           *slog.Logger
	brave            BraveSearcher
}

// Option customizes a Client at construction time.
type Option func(*Client)

// WithBrave merges results from the given Brave searcher into SearXNG output.
// A nil searcher is ignored, leaving SearXNG as the sole source.
func WithBrave(searcher BraveSearcher) Option {
	return func(c *Client) {
		c.brave = searcher
	}
}

// NewClient returns a SearXNG client.
func NewClient(cfg config.SearXNGConfig, logger *slog.Logger, opts ...Option) (*Client, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	c := &Client{
		baseURL: baseURL,
		httpClient: client.New(client.Options{
			Timeout:               cfg.Timeout,
			DialTimeout:           5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxRedirects:          2,
		}),
		defaultLanguage:  cfg.DefaultLanguage,
		defaultTimeRange: cfg.DefaultTimeRange,
		maxLimit:         cfg.MaxLimit,
		logger:           logger,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Search calls SearXNG and returns normalized results.
func (c *Client) Search(ctx context.Context, req types.SearchRequest) (types.SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return types.SearchResponse{}, fmt.Errorf("query is required")
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = c.maxLimit
	}
	if limit > c.maxLimit {
		limit = c.maxLimit
	}

	relative := &url.URL{Path: "/search"}
	values := relative.Query()
	values.Set("q", buildQuery(req.Query, req.Site))
	values.Set("format", "json")
	values.Set("pageno", strconv.Itoa(page))
	if category := strings.TrimSpace(req.Category); category != "" {
		values.Set("categories", category)
	}
	if len(req.Engines) > 0 {
		values.Set("engines", strings.Join(cleanList(req.Engines), ","))
	}
	if language := choose(req.Language, c.defaultLanguage); language != "" {
		values.Set("language", language)
	}
	if timeRange := choose(req.TimeRange, c.defaultTimeRange); timeRange != "" {
		values.Set("time_range", timeRange)
	}
	relative.RawQuery = values.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL.ResolveReference(relative).String(), nil)
	if err != nil {
		return types.SearchResponse{}, err
	}
	httpReq.Header.Set("User-Agent", "github.com/regiellis/mcp-searxng-go/1.0")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return types.SearchResponse{}, err
	}
	defer resp.Body.Close()

	var decoded searxResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return types.SearchResponse{}, err
	}

	results := normalizeResults(decoded.Results, limit)
	results = c.mergeBrave(ctx, req, limit, results)
	return types.SearchResponse{
		Query:       strings.TrimSpace(req.Query),
		Category:    strings.TrimSpace(req.Category),
		Engines:     cleanList(req.Engines),
		Site:        strings.TrimSpace(req.Site),
		Page:        page,
		Limit:       limit,
		ResultCount: len(results),
		Results:     results,
	}, nil
}

// mergeBrave augments SearXNG results with Brave results when a Brave searcher
// is configured. It fails open: any Brave error is logged and the original
// SearXNG results are returned unchanged. SearXNG results keep priority; Brave
// results are interleaved, de-duplicated by URL, and the combined set is capped
// at limit so the configured max_limit is still honored.
func (c *Client) mergeBrave(ctx context.Context, req types.SearchRequest, limit int, results []types.SearchResult) []types.SearchResult {
	if c.brave == nil {
		return results
	}
	braveResults, err := c.brave.Search(ctx, strings.TrimSpace(req.Category), req, limit)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn("brave search unavailable; returning searxng results only",
				"category", strings.TrimSpace(req.Category), "error", err)
		}
		return results
	}
	if len(braveResults) == 0 {
		return results
	}
	return mergeResults(results, braveResults, limit)
}

// mergeResults interleaves two result sets, removing duplicate URLs and capping
// the output at limit (limit <= 0 means no cap). The primary set is preferred:
// when both sets contain the same URL the primary entry is kept.
func mergeResults(primary, secondary []types.SearchResult, limit int) []types.SearchResult {
	capacity := len(primary) + len(secondary)
	seen := make(map[string]struct{}, capacity)
	out := make([]types.SearchResult, 0, capacity)

	add := func(result types.SearchResult) {
		if limit > 0 && len(out) >= limit {
			return
		}
		key := dedupeKey(result.URL)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, result)
	}

	for i := 0; i < len(primary) || i < len(secondary); i++ {
		if limit > 0 && len(out) >= limit {
			break
		}
		if i < len(primary) {
			add(primary[i])
		}
		if i < len(secondary) {
			add(secondary[i])
		}
	}
	return out
}

// dedupeKey normalizes a URL for duplicate detection: lowercased host and path
// with the scheme, a leading "www.", and any trailing slash removed.
func dedupeKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.ToLower(raw)
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	key := host + path
	if parsed.RawQuery != "" {
		key += "?" + parsed.RawQuery
	}
	return key
}

// Ping checks whether the configured SearXNG instance is reachable.
func (c *Client) Ping(ctx context.Context) error {
	relative := &url.URL{Path: "/search"}
	values := relative.Query()
	values.Set("q", "ping")
	values.Set("format", "json")
	values.Set("pageno", "1")
	relative.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL.ResolveReference(relative).String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "github.com/regiellis/mcp-searxng-go/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func choose(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func buildQuery(query, site string) string {
	query = strings.TrimSpace(query)
	site = strings.TrimSpace(site)
	if site == "" {
		return query
	}
	return fmt.Sprintf("site:%s %s", site, query)
}

func cleanList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
