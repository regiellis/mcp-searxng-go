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
	Brave    BraveConfig    `yaml:"brave"`
	Media    MediaConfig    `yaml:"media"`
	LLM      LLMConfig      `yaml:"llm"`
	Server   ServerConfig   `yaml:"server"`
	Fetch    FetchConfig    `yaml:"fetch"`
	Cache    CacheConfig    `yaml:"cache"`
	Security SecurityConfig `yaml:"security"`
	Storage  StorageConfig  `yaml:"storage"`
}

// StorageConfig configures on-disk persistence for research sessions and
// exported reports. All files are confined to Dir.
type StorageConfig struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

// LLMConfig configures the optional DeepSeek-backed transcript cleaning tool.
// The API key is sourced from the DEEPSEEK_API_KEY environment variable rather
// than this file so the secret is never committed. The endpoint is OpenAI/
// DeepSeek chat-completions compatible.
type LLMConfig struct {
	Enabled       bool          `yaml:"enabled"`
	APIKey        string        `yaml:"api_key"`
	BaseURL       string        `yaml:"base_url"`
	Model         string        `yaml:"model"`
	Timeout       time.Duration `yaml:"timeout"`
	MaxInputChars int           `yaml:"max_input_chars"` // per-request transcript chunk budget
}

// Active reports whether the transcript cleaning tool should be wired up.
func (l LLMConfig) Active() bool {
	return l.Enabled &&
		strings.TrimSpace(l.APIKey) != "" &&
		strings.TrimSpace(l.BaseURL) != ""
}

// MediaConfig configures the yt-dlp / ffmpeg backed media tools. All output is
// confined to OutputDir. Binaries are looked up on PATH unless an absolute path
// is given.
type MediaConfig struct {
	Enabled     bool          `yaml:"enabled"`
	OutputDir   string        `yaml:"output_dir"`
	YtDlpPath   string        `yaml:"yt_dlp_path"`
	FfmpegPath  string        `yaml:"ffmpeg_path"`
	FfprobePath string        `yaml:"ffprobe_path"`
	Timeout     time.Duration `yaml:"timeout"`
}

// SearXNGConfig contains SearXNG client configuration.
type SearXNGConfig struct {
	BaseURL          string        `yaml:"base_url"`
	Timeout          time.Duration `yaml:"timeout"`
	DefaultLanguage  string        `yaml:"default_language"`
	DefaultTimeRange string        `yaml:"default_time_range"`
	MaxLimit         int           `yaml:"max_limit"`
}

// BraveConfig contains optional Brave Search API client configuration.
// Results from Brave are merged into SearXNG results when an API key is present;
// if Brave is unreachable the SearXNG results are returned unchanged.
type BraveConfig struct {
	APIKey  string        `yaml:"api_key"`
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
	Enabled bool          `yaml:"enabled"`
}

// Active reports whether Brave merging should be attempted.
func (b BraveConfig) Active() bool {
	return b.Enabled &&
		strings.TrimSpace(b.APIKey) != "" &&
		strings.TrimSpace(b.BaseURL) != ""
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
	MaxPDFBytes    ByteSize      `yaml:"max_pdf_bytes"` // larger body cap for PDFs, which dwarf HTML pages
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
		Brave: BraveConfig{
			BaseURL: "https://api.search.brave.com/res/v1",
			Timeout: 8 * time.Second,
			Enabled: true,
		},
		Media: MediaConfig{
			Enabled:     true,
			OutputDir:   "media",
			YtDlpPath:   "yt-dlp",
			FfmpegPath:  "ffmpeg",
			FfprobePath: "ffprobe",
			Timeout:     10 * time.Minute,
		},
		LLM: LLMConfig{
			Enabled:       true,
			BaseURL:       "https://api.deepseek.com",
			Model:         "deepseek-v4-flash",
			Timeout:       5 * time.Minute,
			MaxInputChars: 48000,
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
			MaxPDFBytes:    ByteSize(16 << 20),
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
		Storage: StorageConfig{
			Enabled: true,
			Dir:     "data",
		},
	}
}

// LoadDotEnv reads a KEY=VALUE file (such as .env) and exports any keys that are
// not already present in the process environment. Existing environment variables
// always win so real env / systemd EnvironmentFile values are never overridden.
// A missing file is not an error: the function simply does nothing.
func LoadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, present := os.LookupEnv(key); present {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
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
	if c.Fetch.MaxPDFBytes <= 0 {
		return errors.New("fetch.max_pdf_bytes must be positive")
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
	if c.Media.Enabled {
		if strings.TrimSpace(c.Media.OutputDir) == "" {
			return errors.New("media.output_dir is required when media is enabled")
		}
		if c.Media.Timeout <= 0 {
			return errors.New("media.timeout must be positive")
		}
	}
	if c.Storage.Enabled && strings.TrimSpace(c.Storage.Dir) == "" {
		return errors.New("storage.dir is required when storage is enabled")
	}
	if c.LLM.Active() {
		if c.LLM.Timeout <= 0 {
			return errors.New("llm.timeout must be positive")
		}
		if c.LLM.MaxInputChars <= 0 {
			return errors.New("llm.max_input_chars must be positive")
		}
	}
	return nil
}

func applyEnv(cfg *Config) {
	setString("MCP_SEARXNG_BASE_URL", &cfg.SearXNG.BaseURL)
	setDuration("MCP_SEARXNG_TIMEOUT", &cfg.SearXNG.Timeout)
	setString("MCP_SEARXNG_DEFAULT_LANGUAGE", &cfg.SearXNG.DefaultLanguage)
	setString("MCP_SEARXNG_DEFAULT_TIME_RANGE", &cfg.SearXNG.DefaultTimeRange)
	setInt("MCP_SEARXNG_MAX_LIMIT", &cfg.SearXNG.MaxLimit)

	// BRAVE_SEARCH_API is the documented key name used in the deployed .env file.
	setString("BRAVE_SEARCH_API", &cfg.Brave.APIKey)
	setString("MCP_BRAVE_BASE_URL", &cfg.Brave.BaseURL)
	setDuration("MCP_BRAVE_TIMEOUT", &cfg.Brave.Timeout)
	setBool("MCP_BRAVE_ENABLED", &cfg.Brave.Enabled)

	setBool("MCP_MEDIA_ENABLED", &cfg.Media.Enabled)
	setString("MCP_MEDIA_OUTPUT_DIR", &cfg.Media.OutputDir)
	setString("MCP_MEDIA_YT_DLP_PATH", &cfg.Media.YtDlpPath)
	setString("MCP_MEDIA_FFMPEG_PATH", &cfg.Media.FfmpegPath)
	setString("MCP_MEDIA_FFPROBE_PATH", &cfg.Media.FfprobePath)
	setDuration("MCP_MEDIA_TIMEOUT", &cfg.Media.Timeout)

	// DEEPSEEK_API_KEY is the documented key name; the rest allow overriding the
	// endpoint/model without editing config.yaml.
	setBool("MCP_LLM_ENABLED", &cfg.LLM.Enabled)
	setString("DEEPSEEK_API_KEY", &cfg.LLM.APIKey)
	setString("MCP_LLM_BASE_URL", &cfg.LLM.BaseURL)
	setString("MCP_LLM_MODEL", &cfg.LLM.Model)
	setDuration("MCP_LLM_TIMEOUT", &cfg.LLM.Timeout)
	setInt("MCP_LLM_MAX_INPUT_CHARS", &cfg.LLM.MaxInputChars)

	setString("MCP_SERVER_MODE", &cfg.Server.Mode)
	setString("MCP_SERVER_ADDRESS", &cfg.Server.Address)
	setString("MCP_SERVER_PUBLIC_BASE_URL", &cfg.Server.PublicBaseURL)
	setDuration("MCP_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	setDuration("MCP_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	setString("MCP_LOG_LEVEL", &cfg.Server.LogLevel)

	setDuration("MCP_FETCH_TIMEOUT", &cfg.Fetch.Timeout)
	setByteSize("MCP_FETCH_MAX_BODY_SIZE", &cfg.Fetch.MaxBodySize)
	setByteSize("MCP_FETCH_MAX_PDF_BYTES", &cfg.Fetch.MaxPDFBytes)
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

	setBool("MCP_STORAGE_ENABLED", &cfg.Storage.Enabled)
	setString("MCP_STORAGE_DIR", &cfg.Storage.Dir)
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
