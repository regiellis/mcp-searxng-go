// Package llm provides a thin client over the DeepSeek chat-completions API
// (OpenAI-compatible). It is used to clean noisy auto-generated transcripts:
// callers send a system instruction plus a transcript chunk and receive the
// edited text. The client carries no ret/streaming logic; one request, one
// response, bounded by the caller's context.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/client"
)

// maxResponseBytes caps how much of a chat response we will read into memory.
const maxResponseBytes = 8 << 20 // 8 MiB

// Client calls a DeepSeek-compatible chat-completions endpoint.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient builds a DeepSeek client from configuration. The base URL must be
// set; an empty API key is allowed so callers gate activation via
// config.LLMConfig.Active.
func NewClient(cfg config.LLMConfig) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("llm base_url is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "deepseek-v4-flash"
	}
	// The response header timeout must accommodate long generations, so it is
	// derived from the overall request timeout rather than a small fixed value.
	headerTimeout := cfg.Timeout
	if headerTimeout <= 0 {
		headerTimeout = 5 * time.Minute
	}
	return &Client{
		baseURL: base,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   model,
		httpClient: client.New(client.Options{
			Timeout:               cfg.Timeout,
			DialTimeout:           5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: headerTimeout,
			IdleConnTimeout:       30 * time.Second,
			MaxRedirects:          2,
		}),
	}, nil
}

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete sends a system+user message pair and returns the assistant's text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	if c == nil || c.apiKey == "" {
		return "", fmt.Errorf("llm api key not configured")
	}

	payload, err := json.Marshal(chatRequest{
		Model:       c.model,
		Stream:      false,
		Temperature: 0.2,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("deepseek status %d: %s", resp.StatusCode, trim(string(data)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode deepseek response: %w", err)
	}
	if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
		return "", fmt.Errorf("deepseek error: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("deepseek returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	const max = 500
	if len(s) > max {
		return s[:max]
	}
	return s
}
