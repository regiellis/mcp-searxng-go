package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
searxng:
  base_url: "https://search.example"
server:
  mode: "stdio"
fetch:
  timeout: 5s
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MCP_SERVER_MODE", "http")
	t.Setenv("MCP_FETCH_MAX_BODY_SIZE", "3MB")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Mode != "http" {
		t.Fatalf("expected env override for mode, got %q", cfg.Server.Mode)
	}
	if cfg.Fetch.MaxBodySize != ByteSize(3<<20) {
		t.Fatalf("expected parsed byte size, got %d", cfg.Fetch.MaxBodySize)
	}
	if cfg.Cache.TTLSearch != 2*time.Minute {
		t.Fatalf("expected default ttl_search, got %s", cfg.Cache.TTLSearch)
	}
}

func TestLoadRejectsInvalidMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  mode: bad\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
