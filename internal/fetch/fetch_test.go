package fetch

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/internal/security"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

func TestExtractHTMLText(t *testing.T) {
	t.Parallel()

	title, text, truncated, err := ExtractHTMLText(strings.NewReader(`<html><head><title>Hello</title><script>x</script></head><body><p>One</p><p>Two</p></body></html>`), 100)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Hello" {
		t.Fatalf("unexpected title %q", title)
	}
	if !strings.Contains(text, "One") || !strings.Contains(text, "Two") {
		t.Fatalf("unexpected text %q", text)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
}

func TestURLValidatorRejectsMalformedAndBlockedTargets(t *testing.T) {
	t.Parallel()

	validator := NewURLValidator([]string{"http", "https"}, security.NetworkGuard{
		BlockPrivateNetworks: true,
		Policy:               security.NewDomainPolicy(nil, nil),
		LookupIP: func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
	})
	if _, err := validator.Validate(context.Background(), "://bad"); err == nil {
		t.Fatal("expected malformed URL error")
	}
	if _, err := validator.Validate(context.Background(), "http://localhost"); err == nil {
		t.Fatal("expected blocked localhost error")
	}
}

func TestReaderReadAndRedirect(t *testing.T) {
	t.Parallel()

	final := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Example</title></head><body>hello world</body></html>`))
	}))
	defer final.Close()

	redirect := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	reader := newTestReader()
	resp, err := reader.Read(context.Background(), types.URLReadRequest{URL: redirect.URL})
	if err != nil {
		t.Fatalf("read url: %v", err)
	}
	if resp.FinalURL != final.URL {
		t.Fatalf("expected final URL %q, got %q", final.URL, resp.FinalURL)
	}
	if resp.Title != "Example" {
		t.Fatalf("unexpected title %q", resp.Title)
	}
	if !strings.Contains(resp.Content, "hello world") {
		t.Fatalf("unexpected content %q", resp.Content)
	}
}

func TestReaderRejectsBinaryContent(t *testing.T) {
	t.Parallel()

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x01, 0x02})
	}))
	defer server.Close()

	reader := newTestReader()
	if _, err := reader.Read(context.Background(), types.URLReadRequest{URL: server.URL}); err == nil {
		t.Fatal("expected non-text content rejection")
	}
}

func TestReaderTimeout(t *testing.T) {
	t.Parallel()

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("slow"))
	}))
	defer server.Close()

	reader := newTestReader()
	reader.cfg.Timeout = 50 * time.Millisecond
	reader.client.Timeout = reader.cfg.Timeout

	if _, err := reader.Read(context.Background(), types.URLReadRequest{URL: server.URL}); err == nil {
		t.Fatal("expected timeout")
	}
}

func newTestReader() *Reader {
	return NewReader(config.FetchConfig{
		Timeout:        time.Second,
		MaxBodySize:    config.ByteSize(4096),
		MaxTextChars:   2048,
		MaxRedirects:   3,
		AllowedSchemes: []string{"http", "https"},
	}, NewURLValidator([]string{"http", "https"}, security.NetworkGuard{
		BlockPrivateNetworks: false,
		Policy:               security.NewDomainPolicy(nil, nil),
	}), slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type localHTTPServer struct {
	URL   string
	Close func()
}

func newHTTPTestServer(t *testing.T, handler http.Handler) *localHTTPServer {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ipv4 loopback: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(listener)
	}()
	return &localHTTPServer{
		URL: "http://" + listener.Addr().String(),
		Close: func() {
			_ = srv.Shutdown(context.Background())
			_ = listener.Close()
		},
	}
}
