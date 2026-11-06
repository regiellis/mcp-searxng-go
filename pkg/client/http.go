package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

// GuardFunc validates the dial target before an outbound connection is made.
type GuardFunc func(ctx context.Context, network, address string) error

// Options configures outbound HTTP safety controls.
type Options struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxRedirects          int
	Guard                 GuardFunc
}

// New returns an HTTP client with strict timeout defaults.
func New(opts Options) *http.Client {
	dialer := &net.Dialer{
		Timeout:   opts.DialTimeout,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           guardedDialContext(dialer, opts.Guard),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxInt(opts.MaxIdleConns, 32),
		MaxIdleConnsPerHost:   maxInt(opts.MaxIdleConnsPerHost, 8),
		IdleConnTimeout:       maxDuration(opts.IdleConnTimeout, 90*time.Second),
		TLSHandshakeTimeout:   maxDuration(opts.TLSHandshakeTimeout, 5*time.Second),
		ResponseHeaderTimeout: maxDuration(opts.ResponseHeaderTimeout, 10*time.Second),
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Timeout:   maxDuration(opts.Timeout, 15*time.Second),
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxInt(opts.MaxRedirects, 4) {
				return errors.New("maximum redirects exceeded")
			}
			if opts.Guard == nil {
				return nil
			}
			host := req.URL.Host
			if _, _, err := net.SplitHostPort(host); err != nil {
				host = net.JoinHostPort(host, defaultPort(req.URL.Scheme))
			}
			return opts.Guard(req.Context(), "tcp", host)
		},
	}
	return client
}

func guardedDialContext(dialer *net.Dialer, guard GuardFunc) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if guard != nil {
			if err := guard(ctx, network, address); err != nil {
				return nil, err
			}
		}
		return dialer.DialContext(ctx, network, address)
	}
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func maxInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

// SplitHostPort returns the host and port, applying a default port when missing.
func SplitHostPort(address, scheme string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		if scheme == "https" {
			return host, 443, nil
		}
		return host, 80, nil
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	return host, value, nil
}
