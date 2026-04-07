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

	"mcp-searxng-go/internal/config"
	"mcp-searxng-go/pkg/client"
	"mcp-searxng-go/pkg/types"
)

type searxResponse struct {
	Query   string        `json:"query"`
	Results []searxResult `json:"results"`
}

// Client calls SearXNG and normalizes results.
type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	defaultLanguage  string
	defaultTimeRange string
	maxLimit         int
	logger           *slog.Logger
}

// NewClient returns a SearXNG client.
func NewClient(cfg config.SearXNGConfig, logger *slog.Logger) (*Client, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
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
	}, nil
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
	values.Set("q", req.Query)
	values.Set("format", "json")
	values.Set("pageno", strconv.Itoa(page))
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
	httpReq.Header.Set("User-Agent", "mcp-searxng-go/1.0")
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
	return types.SearchResponse{
		Query:       strings.TrimSpace(req.Query),
		Page:        page,
		Limit:       limit,
		ResultCount: len(results),
		Results:     results,
	}, nil
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
	req.Header.Set("User-Agent", "mcp-searxng-go/1.0")
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
