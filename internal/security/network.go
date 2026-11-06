package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/client"
)

// NetworkGuard enforces SSRF-related address policy.
type NetworkGuard struct {
	BlockPrivateNetworks bool
	Policy               DomainPolicy
	LookupIP             func(ctx context.Context, network, host string) ([]net.IP, error)
}

// ResolveAndValidateHost rejects denied domains and unsafe IP ranges.
func (g NetworkGuard) ResolveAndValidateHost(ctx context.Context, host string) error {
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return errors.New("host is required")
	}
	if !g.Policy.Allowed(host) {
		return fmt.Errorf("domain %q is not allowed", host)
	}
	if !g.BlockPrivateNetworks {
		return nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked private address: %s", ip.String())
		}
		return nil
	}

	lookup := g.LookupIP
	if lookup == nil {
		var resolver net.Resolver
		lookup = resolver.LookupIP
	}
	ips, err := lookup(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("resolve host %q: no addresses returned", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked private address: %s", ip.String())
		}
	}
	return nil
}

// DialGuard validates the concrete dial target.
func (g NetworkGuard) DialGuard(ctx context.Context, _, address string) error {
	host, _, err := client.SplitHostPort(address, "http")
	if err != nil {
		return err
	}
	return g.ResolveAndValidateHost(ctx, host)
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
