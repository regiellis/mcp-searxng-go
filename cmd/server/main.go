package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mcp-searxng-go/internal/config"
	"mcp-searxng-go/internal/fetch"
	"mcp-searxng-go/internal/mcp"
	"mcp-searxng-go/internal/search"
	"mcp-searxng-go/internal/security"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", "configs/config.yaml", "Path to YAML config")
		mode       = flag.String("mode", "", "Transport mode override: stdio or http")
		listen     = flag.String("listen", "", "HTTP listen address override")
		logLevel   = flag.String("log-level", "", "Log level override")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *mode != "" {
		cfg.Server.Mode = *mode
	}
	if *listen != "" {
		cfg.Server.Address = *listen
	}
	if *logLevel != "" {
		cfg.Server.LogLevel = *logLevel
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger := newLogger(cfg.Server.LogLevel)
	logger.Info("startup", "mode", cfg.Server.Mode, "address", cfg.Server.Address, "searxng", cfg.SearXNG.BaseURL)

	searchClient, err := search.NewClient(cfg.SearXNG, logger)
	if err != nil {
		return err
	}
	logStartupStatus(logger, cfg, searchClient)
	guard := security.NetworkGuard{
		BlockPrivateNetworks: cfg.Security.BlockPrivateNetworks,
		Policy:               security.NewDomainPolicy(cfg.Security.AllowDomains, cfg.Security.DenyDomains),
	}
	reader := fetch.NewReader(cfg.Fetch, fetch.NewURLValidator(cfg.Fetch.AllowedSchemes, guard), logger)
	server := mcp.NewServer(cfg, searchClient, reader, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.Server.Mode == "http" {
		httpServer := &http.Server{
			Addr:         cfg.Server.Address,
			Handler:      server.HTTPHandler(),
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		}
		go func() {
			<-ctx.Done()
			_ = httpServer.Shutdown(context.Background())
		}()
		return httpServer.ListenAndServe()
	}

	return server.ServeStdio(ctx, os.Stdin, os.Stdout)
}

func logStartupStatus(logger *slog.Logger, cfg config.Config, searchClient *search.Client) {
	if cfg.Server.Mode != "http" {
		logger.Info("stdio ready", "searxng", cfg.SearXNG.BaseURL)
		return
	}

	host, port, err := net.SplitHostPort(cfg.Server.Address)
	if err != nil {
		host = cfg.Server.Address
		port = "8081"
	}
	if host == "" {
		host = "0.0.0.0"
	}

	ips := localIPs()
	baseURLs := make([]string, 0, len(ips)+1)
	for _, ip := range ips {
		baseURLs = append(baseURLs, fmt.Sprintf("http://%s:%s", ip, port))
	}
	if len(baseURLs) == 0 {
		baseURLs = append(baseURLs, fmt.Sprintf("http://%s:%s", host, port))
	}

	logger.Info("http endpoints",
		"bind", cfg.Server.Address,
		"local_ips", ips,
		"mcp_urls", addPath(baseURLs, "/mcp"),
		"healthz_urls", addPath(baseURLs, "/healthz"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := searchClient.Ping(ctx); err != nil {
		logger.Warn("searxng connectivity check failed", "url", cfg.SearXNG.BaseURL, "error", err)
		return
	}
	logger.Info("searxng connectivity ok", "url", cfg.SearXNG.BaseURL)
}

func localIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func addPath(urls []string, path string) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		out = append(out, strings.TrimRight(raw, "/")+path)
	}
	return out
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
}
