package security

import "strings"

// DomainPolicy evaluates simple allow and deny lists.
type DomainPolicy struct {
	allow []string
	deny  []string
}

// NewDomainPolicy returns a normalized domain policy.
func NewDomainPolicy(allow, deny []string) DomainPolicy {
	return DomainPolicy{
		allow: normalizeDomains(allow),
		deny:  normalizeDomains(deny),
	}
}

// Allowed returns whether the host is allowed by the configured domain rules.
func (p DomainPolicy) Allowed(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return false
	}
	for _, denied := range p.deny {
		if matchesDomain(host, denied) {
			return false
		}
	}
	if len(p.allow) == 0 {
		return true
	}
	for _, allowed := range p.allow {
		if matchesDomain(host, allowed) {
			return true
		}
	}
	return false
}

func normalizeDomains(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), "."))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func matchesDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
