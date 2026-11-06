package fetch

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/regiellis/mcp-searxng-go/internal/security"
)

// URLValidator validates schemes and SSRF-sensitive targets.
type URLValidator struct {
	AllowedSchemes map[string]struct{}
	Guard          security.NetworkGuard
}

// NewURLValidator creates a URL validator from configured schemes and guard.
func NewURLValidator(schemes []string, guard security.NetworkGuard) URLValidator {
	allowed := make(map[string]struct{}, len(schemes))
	for _, scheme := range schemes {
		allowed[strings.ToLower(strings.TrimSpace(scheme))] = struct{}{}
	}
	return URLValidator{AllowedSchemes: allowed, Guard: guard}
}

// Validate parses the URL, enforces the scheme, and checks the target host.
func (v URLValidator) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid URL: missing scheme or host")
	}
	if _, ok := v.AllowedSchemes[strings.ToLower(parsed.Scheme)]; !ok {
		return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if err := v.Guard.ResolveAndValidateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}
