package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// ServeHTTP serves srv on ln until ctx is cancelled, then shuts down
// gracefully: it stops accepting connections and waits up to shutdownTimeout
// for in-flight requests to finish before force-closing the rest.
//
// A requested shutdown that drains in time returns nil. http.ErrServerClosed
// is the normal result of Shutdown, not a failure, so it is never returned.
// The function does not return until shutdown has finished; returning when
// Serve returns would let the process exit mid-drain, because Serve returns
// as soon as Shutdown starts.
func ServeHTTP(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration, logger *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		// Serve stopped without a shutdown request: a real failure.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown requested; draining in-flight requests", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		closeErr := srv.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("force close: %w", closeErr)
		}
		return errors.Join(fmt.Errorf("graceful shutdown: %w", err), closeErr)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http serve: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

// fallbackShutdownTimeout bounds the drain when write_timeout is unset (0),
// which leaves responses themselves unbounded.
const fallbackShutdownTimeout = 30 * time.Second

// ShutdownTimeout derives the drain budget from the server's write timeout.
// write_timeout already caps how long any response may take, so a little
// past it covers every request that was in flight when shutdown began.
func ShutdownTimeout(writeTimeout time.Duration) time.Duration {
	if writeTimeout <= 0 {
		return fallbackShutdownTimeout
	}
	return writeTimeout + 5*time.Second
}
