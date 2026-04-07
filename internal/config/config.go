package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ByteSize is a human-friendly byte count such as 2MB or 512KB.
type ByteSize int64

// Config contains runtime configuration.
type Config struct {
	SearXNG  SearXNGConfig  `yaml:"searxng"`
	Server   ServerConfig   `yaml:"server"`
	Fetch    FetchConfig    `yaml:"fetch"`
	Cache    CacheConfig    `yaml:"cache"`
	Security SecurityConfig `yaml:"security"`
}

// SearXNGConfig contains SearXNG client configuration.
type SearXNGConfig struct {
	BaseURL          string        `yaml:"base_url"`
	Timeout          time.Duration `yaml:"timeout"`
	DefaultLanguage  string        `yaml:"default_language"`
	DefaultTimeRange string        `yaml:"default_time_range"`
	MaxLimit         int           `yaml:"max_limit"`
}

// ServerConfig contains transport configuration.
type ServerConfig struct {
	Mode          string        `yaml:"mode"`
	Address       string        `yaml:"address"`
	PublicBaseURL string        `yaml:"public_base_url"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
	LogLevel      string        `yaml:"log_level"`
}

// FetchConfig contains URL fetch limits.
type FetchConfig struct {
	Timeout        time.Duration `yaml:"timeout"`
	MaxBodySize    ByteSize      `yaml:"max_body_size"`
	MaxTextChars   int           `yaml:"max_text_chars"`
	MaxRedirects   int           `yaml:"max_redirects"`
	AllowedSchemes []string      `yaml:"allowed_schemes"`
}

// CacheConfig contains in-memory cache settings.
type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	TTLSearch  time.Duration `yaml:"ttl_search"`
	TTLURLRead time.Duration `yaml:"ttl_url_read"`
	MaxEntries int           `yaml:"max_entries"`
}

// SecurityConfig contains SSRF-related policy.
type SecurityConfig struct {
	BlockPrivateNetworks bool     `yaml:"block_private_networks"`
	AllowDomains         []string `yaml:"allow_domains"`
	DenyDomains          []string `yaml:"deny_domains"`
}

// Default returns a safe baseline configuration.
func Default() Config {
	return Config{
		SearXNG: SearXNGConfig{
			BaseURL:          "http://127.0.0.1:7777",
			Timeout:          10 * time.Second,
			DefaultLanguage:  "all",
			DefaultTimeRange: "",
			MaxLimit:         10,
		},
		Server: ServerConfig{
			Mode:          "http",
			Address:       "0.0.0.0:8081",
			PublicBaseURL: "",
			ReadTimeout:   15 * time.Second,
			WriteTimeout:  15 * time.Second,
			LogLevel:      "info",
		},
		Fetch: FetchConfig{
			Timeout:        15 * time.Second,
			MaxBodySize:    ByteSize(2 << 20),
			MaxTextChars:   12000,
			MaxRedirects:   4,
			AllowedSchemes: []string{"http", "https"},
		},
		Cache: CacheConfig{
			Enabled:    true,
			TTLSearch:  2 * time.Minute,
			TTLURLRead: 5 * time.Minute,
			MaxEntries: 256,
		},
		Security: SecurityConfig{
			BlockPrivateNetworks: false,
		},
	}
}

// Load reads YAML config and applies environment overrides.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks user-facing config invariants.
func (c Config) Validate() error {
	if c.SearXNG.BaseURL == "" {
		return errors.New("searxng.base_url is required")
	}
	if c.Server.Mode != "stdio" && c.Server.Mode != "http" {
		return fmt.Errorf("server.mode must be stdio or http, got %q", c.Server.Mode)
	}
	if c.SearXNG.MaxLimit <= 0 {
		return errors.New("searxng.max_limit must be positive")
	}
	if c.Fetch.Timeout <= 0 {
		return errors.New("fetch.timeout must be positive")
	}
	if c.Fetch.MaxBodySize <= 0 {
		return errors.New("fetch.max_body_size must be positive")
	}
	if c.Fetch.MaxTextChars <= 0 {
		return errors.New("fetch.max_text_chars must be positive")
	}
	if c.Fetch.MaxRedirects < 0 {
		return errors.New("fetch.max_redirects must be zero or positive")
	}
	if len(c.Fetch.AllowedSchemes) == 0 {
		return errors.New("fetch.allowed_schemes must not be empty")
	}
	return nil
}

func applyEnv(cfg *Config) {
	setString("MCP_SEARXNG_BASE_URL", &cfg.SearXNG.BaseURL)
	setDuration("MCP_SEARXNG_TIMEOUT", &cfg.SearXNG.Timeout)
	setString("MCP_SEARXNG_DEFAULT_LANGUAGE", &cfg.SearXNG.DefaultLanguage)
	setString("MCP_SEARXNG_DEFAULT_TIME_RANGE", &cfg.SearXNG.DefaultTimeRange)
	setInt("MCP_SEARXNG_MAX_LIMIT", &cfg.SearXNG.MaxLimit)

	setString("MCP_SERVER_MODE", &cfg.Server.Mode)
	setString("MCP_SERVER_ADDRESS", &cfg.Server.Address)
	setString("MCP_SERVER_PUBLIC_BASE_URL", &cfg.Server.PublicBaseURL)
	setDuration("MCP_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	setDuration("MCP_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	setString("MCP_LOG_LEVEL", &cfg.Server.LogLevel)

	setDuration("MCP_FETCH_TIMEOUT", &cfg.Fetch.Timeout)
	setByteSize("MCP_FETCH_MAX_BODY_SIZE", &cfg.Fetch.MaxBodySize)
	setInt("MCP_FETCH_MAX_TEXT_CHARS", &cfg.Fetch.MaxTextChars)
	setInt("MCP_FETCH_MAX_REDIRECTS", &cfg.Fetch.MaxRedirects)
	setCSV("MCP_FETCH_ALLOWED_SCHEMES", &cfg.Fetch.AllowedSchemes)

	setBool("MCP_CACHE_ENABLED", &cfg.Cache.Enabled)
	setDuration("MCP_CACHE_TTL_SEARCH", &cfg.Cache.TTLSearch)
	setDuration("MCP_CACHE_TTL_URL_READ", &cfg.Cache.TTLURLRead)
	setInt("MCP_CACHE_MAX_ENTRIES", &cfg.Cache.MaxEntries)

	setBool("MCP_SECURITY_BLOCK_PRIVATE_NETWORKS", &cfg.Security.BlockPrivateNetworks)
	setCSV("MCP_SECURITY_ALLOW_DOMAINS", &cfg.Security.AllowDomains)
	setCSV("MCP_SECURITY_DENY_DOMAINS", &cfg.Security.DenyDomains)
}

func setString(key string, target *string) {
	if value := os.Getenv(key); value != "" {
		*target = value
	}
}

func setInt(key string, target *int) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func setBool(key string, target *bool) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			*target = parsed
		}
	}
}

func setDuration(key string, target *time.Duration) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			*target = parsed
		}
	}
}

func setCSV(key string, target *[]string) {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		*target = out
	}
}

func setByteSize(key string, target *ByteSize) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := parseByteSize(value); err == nil {
			*target = parsed
		}
	}
}

// UnmarshalYAML decodes byte sizes from strings or integers.
func (b *ByteSize) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!int" {
			value, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return err
			}
			*b = ByteSize(value)
			return nil
		}
		value, err := parseByteSize(node.Value)
		if err != nil {
			return err
		}
		*b = value
		return nil
	default:
		return fmt.Errorf("invalid byte size node kind %d", node.Kind)
	}
}

func parseByteSize(value string) (ByteSize, error) {
	value = strings.TrimSpace(strings.ToUpper(value))
	multiplier := int64(1)
	for _, suffix := range []struct {
		name string
		unit int64
	}{
		{name: "GB", unit: 1 << 30},
		{name: "MB", unit: 1 << 20},
		{name: "KB", unit: 1 << 10},
		{name: "B", unit: 1},
	} {
		if strings.HasSuffix(value, suffix.name) {
			value = strings.TrimSuffix(value, suffix.name)
			multiplier = suffix.unit
			break
		}
	}
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	return ByteSize(number * multiplier), nil
}
