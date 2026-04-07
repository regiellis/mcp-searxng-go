package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"mcp-searxng-go/internal/config"
	"mcp-searxng-go/pkg/client"
	"mcp-searxng-go/pkg/types"
)

// Reader fetches and extracts public text content.
type Reader struct {
	client    *http.Client
	cfg       config.FetchConfig
	validator URLValidator
	logger    *slog.Logger
}

// NewReader returns a URL reader with strict HTTP safety limits.
func NewReader(cfg config.FetchConfig, validator URLValidator, logger *slog.Logger) *Reader {
	return &Reader{
		cfg:       cfg,
		validator: validator,
		logger:    logger,
		client: client.New(client.Options{
			Timeout:               cfg.Timeout,
			DialTimeout:           5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxRedirects:          cfg.MaxRedirects,
			Guard:                 validator.Guard.DialGuard,
		}),
	}
}

// Read fetches a URL and extracts readable text.
func (r *Reader) Read(ctx context.Context, req types.URLReadRequest) (types.URLReadResponse, error) {
	parsed, err := r.validator.Validate(ctx, req.URL)
	if err != nil {
		return types.URLReadResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return types.URLReadResponse{}, err
	}
	httpReq.Header.Set("User-Agent", "mcp-searxng-go/1.0")
	httpReq.Header.Set("Accept", "text/html, text/plain, application/json, application/xml;q=0.9, text/*;q=0.8")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return types.URLReadResponse{}, err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if !isTextLike(contentType) {
		return types.URLReadResponse{}, fmt.Errorf("non-text content rejected: %s", contentType)
	}

	limited := io.LimitReader(resp.Body, int64(r.cfg.MaxBodySize)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return types.URLReadResponse{}, err
	}
	truncated := int64(len(body)) > int64(r.cfg.MaxBodySize)
	if truncated {
		body = body[:int(r.cfg.MaxBodySize)]
	}

	title, content, textTruncated, err := extractContent(contentType, body, r.cfg.MaxTextChars)
	if err != nil {
		return types.URLReadResponse{}, err
	}

	return types.URLReadResponse{
		FinalURL:    resp.Request.URL.String(),
		ContentType: contentType,
		StatusCode:  resp.StatusCode,
		Title:       title,
		Content:     content,
		Truncated:   truncated || textTruncated,
	}, nil
}

func extractContent(contentType string, body []byte, maxTextChars int) (string, string, bool, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return "", "", false, err
	}
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml" || strings.Contains(mediaType, "xml"):
		return ExtractHTMLText(bytes.NewReader(body), maxTextChars)
	case strings.HasPrefix(mediaType, "text/") || mediaType == "application/json":
		content := string(body)
		if len(content) > maxTextChars {
			return "", content[:maxTextChars], true, nil
		}
		return "", content, false, nil
	default:
		return "", "", false, errors.New("unsupported content type")
	}
}

func isTextLike(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.HasPrefix(strings.ToLower(contentType), "text/")
	}
	return strings.HasPrefix(mediaType, "text/") ||
		mediaType == "application/json" ||
		mediaType == "application/xml" ||
		mediaType == "application/xhtml+xml" ||
		mediaType == "application/rss+xml" ||
		mediaType == "application/atom+xml"
}
