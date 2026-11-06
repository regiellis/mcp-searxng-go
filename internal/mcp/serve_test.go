package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func listenLoopback(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startServe runs ServeHTTP in the background and returns its result channel.
func startServe(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- ServeHTTP(ctx, srv, ln, timeout, discardLogger())
	}()
	return done
}

func waitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("ServeHTTP did not return")
		return nil
	}
}

func TestServeHTTPCleanShutdownReturnsNil(t *testing.T) {
	t.Parallel()

	ln := listenLoopback(t)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := startServe(ctx, srv, ln, time.Second)

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("server not serving: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := waitResult(t, done); err != nil {
		t.Fatalf("clean shutdown must return nil, got %v", err)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); err == nil {
		t.Fatal("listener still accepting after shutdown")
	}
}

func TestServeHTTPDrainsInFlightRequest(t *testing.T) {
	t.Parallel()

	ln := listenLoopback(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		if _, err := io.WriteString(w, "finished"); err != nil {
			t.Errorf("write: %v", err)
		}
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := startServe(ctx, srv, ln, 5*time.Second)

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		got <- result{body: string(body), err: err}
	}()

	<-entered
	cancel()
	// ServeHTTP must still be draining, not returned, while the request runs.
	select {
	case err := <-done:
		t.Fatalf("ServeHTTP returned before the in-flight request finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if r := <-got; r.err != nil || r.body != "finished" {
		t.Fatalf("in-flight request = %q, %v; want it to complete", r.body, r.err)
	}
	if err := waitResult(t, done); err != nil {
		t.Fatalf("drained shutdown must return nil, got %v", err)
	}
}

func TestServeHTTPReportsShutdownTimeout(t *testing.T) {
	t.Parallel()

	ln := listenLoopback(t)
	entered := make(chan struct{})
	hold := make(chan struct{})
	defer close(hold)
	srv := &http.Server{Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := startServe(ctx, srv, ln, 50*time.Millisecond)

	// The stuck request is force-closed, so the client normally sees an error;
	// either way it must finish, and any body it got must close cleanly.
	clientDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			clientDone <- nil
			return
		}
		clientDone <- resp.Body.Close()
	}()
	<-entered
	cancel()

	err := waitResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "graceful shutdown") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want graceful shutdown deadline error", err)
	}
	select {
	case closeErr := <-clientDone:
		if closeErr != nil {
			t.Fatalf("close body: %v", closeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stuck request was not cut off by the force close")
	}
}

func TestServeHTTPReturnsServeFailure(t *testing.T) {
	t.Parallel()

	ln := listenLoopback(t)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler()}
	err := waitResult(t, startServe(context.Background(), srv, ln, time.Second))
	if err == nil || !strings.Contains(err.Error(), "http serve") {
		t.Fatalf("err = %v, want an http serve failure", err)
	}
}

func TestShutdownTimeout(t *testing.T) {
	t.Parallel()

	cases := map[time.Duration]time.Duration{
		15 * time.Second: 20 * time.Second,
		0:                fallbackShutdownTimeout,
		-time.Second:     fallbackShutdownTimeout,
	}
	for write, want := range cases {
		if got := ShutdownTimeout(write); got != want {
			t.Errorf("ShutdownTimeout(%v) = %v, want %v", write, got, want)
		}
	}
}
