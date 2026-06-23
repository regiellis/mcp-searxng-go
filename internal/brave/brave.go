// Package brave provides a thin client over the Brave Search API. Results are
// normalized into the shared search result type so they can be merged with
// SearXNG output. The client is intentionally fail-open: callers treat any error
// as "Brave unavailable" and fall back to SearXNG-only results.
package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/client"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Engine is the provenance label applied to every Brave-sourced result.
const Engine = "brave"

// endpoint describes how a SearXNG category maps onto a Brave API endpoint.
type endpoint struct {
	path         string
	maxCount     int
	hasFreshness bool
	// nested reports whether results live under "web.results" (true) or the
	// top-level "results" array (false).
	nested bool
}

// endpoints maps each supported category to its Brave endpoint. Categories that
// are absent here are simply not queried against Brave.
var endpoints = map[string]endpoint{
	"general": {path: "/web/search", maxCount: 20, hasFreshness: true, nested: true},
	"images":  {path: "/images/search", maxCount: 200, hasFreshness: false, nested: false},
	"videos":  {path: "/videos/search", maxCount: 50, hasFreshness: true, nested: false},
	"news":    {path: "/news/search", maxCount: 50, hasFreshness: true, nested: false},
}

// Client calls the Brave Search API and normalizes results.
type Client struct {
	baseURL    *urlpkg.URL
	apiKey     string
	httpClient *http.Client
}

type braveThumbnail struct {
	Src string `json:"src"`
}

type braveMetaURL struct {
	Hostname string `json:"hostname"`
}

type braveProfile struct {
	Name string `json:"name"`
}

type braveImageProps struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type braveVideoMeta struct {
	Duration  string       `json:"duration"`
	Creator   string       `json:"creator"`
	Publisher string       `json:"publisher"`
	Author    braveProfile `json:"author"`
}

// braveResult is the union of the fields we consume across the web, news, image
// and video endpoints. Unused fields per endpoint simply stay at their zero value.
type braveResult struct {
	Title       string          `json:"title"`
	URL         string          `json:"url"`
	Description string          `json:"description"`
	Source      string          `json:"source"`
	Age         string          `json:"age"`
	PageAge     string          `json:"page_age"`
	Thumbnail   braveThumbnail  `json:"thumbnail"`
	Properties  braveImageProps `json:"properties"`
	MetaURL     braveMetaURL    `json:"meta_url"`
	Profile     braveProfile    `json:"profile"`
	Video       braveVideoMeta  `json:"video"`
}

type braveResponse struct {
	Web struct {
		Results []braveResult `json:"results"`
	} `json:"web"`
	Results []braveResult `json:"results"`
}

// NewClient builds a Brave client from configuration. It returns an error only
// when the configured base URL cannot be parsed; an empty API key is allowed so
// callers can decide whether Brave is active via config.BraveConfig.Active.
func NewClient(cfg config.BraveConfig) (*Client, error) {
	baseURL, err := urlpkg.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse brave base_url: %w", err)
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		httpClient: client.New(client.Options{
			Timeout:               cfg.Timeout,
			DialTimeout:           5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxRedirects:          2,
		}),
	}, nil
}

// Search queries the Brave endpoint matching the SearXNG category and returns
// normalized results. Categories without a Brave mapping return no results and
// no error, leaving SearXNG as the sole source for them.
func (c *Client) Search(ctx context.Context, category string, req types.SearchRequest, limit int) ([]types.SearchResult, error) {
	if c == nil || c.apiKey == "" {
		return nil, nil
	}
	ep, ok := endpoints[strings.TrimSpace(category)]
	if !ok {
		return nil, nil
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil
	}

	count := limit
	if count <= 0 || count > ep.maxCount {
		count = ep.maxCount
	}

	relative := &urlpkg.URL{Path: ep.path}
	values := relative.Query()
	values.Set("q", query)
	values.Set("count", strconv.Itoa(count))
	if lang := braveLang(req.Language); lang != "" {
		values.Set("search_lang", lang)
	}
	if ep.hasFreshness {
		if freshness := braveFreshness(req.TimeRange); freshness != "" {
			values.Set("freshness", freshness)
		}
	}
	relative.RawQuery = values.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL.ResolveReference(relative).String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Subscription-Token", c.apiKey)
	httpReq.Header.Set("User-Agent", "github.com/regiellis/mcp-searxng-go/1.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("brave status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	results := decoded.Results
	if ep.nested {
		results = decoded.Web.Results
	}
	return normalize(results, count), nil
}

func normalize(results []braveResult, limit int) []types.SearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	out := make([]types.SearchResult, 0, limit)
	for _, item := range results {
		if len(out) >= limit {
			break
		}
		link := strings.TrimSpace(item.URL)
		title := strings.TrimSpace(item.Title)
		if link == "" || title == "" {
			continue
		}
		domain := strings.TrimSpace(item.MetaURL.Hostname)
		if domain == "" {
			if parsed, err := urlpkg.Parse(link); err == nil {
				domain = strings.TrimSpace(parsed.Hostname())
			}
		}
		published := strings.TrimSpace(item.PageAge)
		if published == "" {
			published = strings.TrimSpace(item.Age)
		}
		out = append(out, types.SearchResult{
			Title:        title,
			URL:          link,
			Snippet:      strings.TrimSpace(item.Description),
			Engine:       Engine,
			Source:       firstNonEmpty(item.Profile.Name, item.Source),
			Domain:       domain,
			ThumbnailURL: strings.TrimSpace(item.Thumbnail.Src),
			ContentURL:   strings.TrimSpace(item.Properties.URL),
			Width:        item.Properties.Width,
			Height:       item.Properties.Height,
			Duration:     strings.TrimSpace(item.Video.Duration),
			PublishedAt:  published,
			Channel:      strings.TrimSpace(item.Video.Publisher),
			Author:       firstNonEmpty(item.Video.Author.Name, item.Video.Creator),
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// braveLang maps an internal language value to a Brave search_lang code. The
// catch-all "all" sentinel and empty values disable the filter.
func braveLang(language string) string {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" || language == "all" {
		return ""
	}
	// SearXNG languages may be "en" or "en-US"; Brave expects the base code.
	if base, _, found := strings.Cut(language, "-"); found {
		return base
	}
	return language
}

// braveFreshness maps an internal time range to a Brave freshness token.
func braveFreshness(timeRange string) string {
	switch strings.TrimSpace(strings.ToLower(timeRange)) {
	case "day":
		return "pd"
	case "week":
		return "pw"
	case "month":
		return "pm"
	case "year":
		return "py"
	default:
		return ""
	}
}
