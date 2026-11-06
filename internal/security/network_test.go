package security

import (
	"context"
	"net"
	"testing"
)

func TestNetworkGuardBlocksPrivateRanges(t *testing.T) {
	t.Parallel()

	guard := NetworkGuard{
		BlockPrivateNetworks: true,
		Policy:               NewDomainPolicy(nil, nil),
		LookupIP: func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		},
	}
	if err := guard.ResolveAndValidateHost(context.Background(), "example.com"); err == nil {
		t.Fatal("expected private IP to be rejected")
	}
}

func TestNetworkGuardAllowsPublicHost(t *testing.T) {
	t.Parallel()

	guard := NetworkGuard{
		BlockPrivateNetworks: true,
		Policy:               NewDomainPolicy([]string{"example.com"}, nil),
		LookupIP: func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	}
	if err := guard.ResolveAndValidateHost(context.Background(), "www.example.com"); err != nil {
		t.Fatalf("expected public host to pass, got %v", err)
	}
}

func TestNetworkGuardDeniesDomain(t *testing.T) {
	t.Parallel()

	guard := NetworkGuard{
		BlockPrivateNetworks: true,
		Policy:               NewDomainPolicy(nil, []string{"example.com"}),
	}
	if err := guard.ResolveAndValidateHost(context.Background(), "api.example.com"); err == nil {
		t.Fatal("expected deny-domain error")
	}
}
