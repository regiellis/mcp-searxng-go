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

	"github.com/regiellis/mcp-searxng-go/internal/brave"
	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/internal/fetch"
	"github.com/regiellis/mcp-searxng-go/internal/llm"
	"github.com/regiellis/mcp-searxng-go/internal/mcp"
	"github.com/regiellis/mcp-searxng-go/internal/media"
	"github.com/regiellis/mcp-searxng-go/internal/search"
	"github.com/regiellis/mcp-searxng-go/internal/security"
	"github.com/regiellis/mcp-searxng-go/internal/store"
	"github.com/regiellis/mcp-searxng-go/internal/transcript"
)

// Build metadata, injected at build time via -ldflags. See deploy.sh and the
// Taskfile build/release targets. Values are derived from git so builds remain
// reproducible (commit date is used rather than wall-clock build time).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
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

	// Load .env (if present) before config so keys like BRAVE_SEARCH_API are
	// picked up by environment overrides. Real env vars always take precedence.
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}

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

	logger := newLogger(cfg.Server.LogLevel, cfg.Server.Mode)
	logger.Info("startup", "version", version, "commit", commit, "date", date, "mode", cfg.Server.Mode, "address", cfg.Server.Address, "searxng", cfg.SearXNG.BaseURL)

	var searchOpts []search.Option
	if cfg.Brave.Active() {
		braveClient, err := brave.NewClient(cfg.Brave)
		if err != nil {
			return err
		}
		searchOpts = append(searchOpts, search.WithBrave(braveClient))
		logger.Info("brave search merge enabled", "base_url", cfg.Brave.BaseURL)
	}

	searchClient, err := search.NewClient(cfg.SearXNG, logger, searchOpts...)
	if err != nil {
		return err
	}
	logStartupStatus(logger, cfg, searchClient)
	guard := security.NetworkGuard{
		BlockPrivateNetworks: cfg.Security.BlockPrivateNetworks,
		Policy:               security.NewDomainPolicy(cfg.Security.AllowDomains, cfg.Security.DenyDomains),
	}
	validator := fetch.NewURLValidator(cfg.Fetch.AllowedSchemes, guard)
	reader := fetch.NewReader(cfg.Fetch, validator, logger)

	var mediaRunner *media.Runner
	if cfg.Media.Enabled {
		mediaRunner, err = media.NewRunner(cfg.Media, validator, logger)
		if err != nil {
			return err
		}
		if preErr := mediaRunner.Preflight(); preErr != nil {
			logger.Warn("media tools enabled but a backend binary is missing; calls will fail until installed", "error", preErr)
		} else {
			logger.Info("media tools enabled", "output_dir", mediaRunner.OutputDir())
		}
	}

	var (
		cleaner *transcript.Cleaner
		synth   mcp.Synthesizer
	)
	if cfg.LLM.Active() {
		llmClient, err := llm.NewClient(cfg.LLM)
		if err != nil {
			return err
		}
		cleaner = transcript.NewCleaner(llmClient, cfg.LLM.MaxInputChars)
		synth = llmClient
		logger.Info("llm features enabled", "model", llmClient.Model(), "base_url", cfg.LLM.BaseURL)
	} else if cfg.LLM.Enabled {
		logger.Info("llm features disabled: DEEPSEEK_API_KEY not set")
	}

	var researchStore *store.Store
	if cfg.Storage.Enabled {
		researchStore, err = store.NewStore(cfg.Storage.Dir, logger)
		if err != nil {
			return err
		}
		logger.Info("research storage enabled", "dir", researchStore.Dir())
	}

	server := mcp.NewServer(cfg, searchClient, reader, mediaRunner, cleaner, synth, researchStore, logger)

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
	preferredBaseURL := strings.TrimSpace(cfg.Server.PublicBaseURL)
	if preferredBaseURL == "" && len(baseURLs) > 0 {
		preferredBaseURL = baseURLs[0]
	}

	logger.Info("http endpoints",
		"bind", cfg.Server.Address,
		"local_ips", ips,
		"preferred_base_url", preferredBaseURL,
		"preferred_mcp_url", strings.TrimRight(preferredBaseURL, "/")+"/mcp",
		"preferred_tools_url", strings.TrimRight(preferredBaseURL, "/")+"/tools",
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

func newLogger(level, mode string) *slog.Logger {
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
	// In stdio transport, stdout carries the MCP JSON-RPC frames, so structured
	// logs must go to stderr to avoid corrupting the protocol. In http mode
	// stdout is free, and emitting structured logs there is the conventional
	// place a process/service collector expects them.
	out := os.Stderr
	if mode == "http" {
		out = os.Stdout
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slogLevel}))
}
