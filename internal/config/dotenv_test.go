package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsAndRespectsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\nexport BRAVE_SEARCH_API=\"secret-key\"\nMCP_BRAVE_ENABLED=true\nPRESET=fromfile\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PRESET", "fromenv")

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("BRAVE_SEARCH_API"); got != "secret-key" {
		t.Fatalf("expected quotes stripped key, got %q", got)
	}
	if got := os.Getenv("MCP_BRAVE_ENABLED"); got != "true" {
		t.Fatalf("expected MCP_BRAVE_ENABLED=true, got %q", got)
	}
	if got := os.Getenv("PRESET"); got != "fromenv" {
		t.Fatalf("existing env must win, got %q", got)
	}
}

func TestLoadDotEnvMissingFileIsNoError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
}

func TestBraveConfigActive(t *testing.T) {
	if (BraveConfig{Enabled: true, APIKey: "k", BaseURL: "https://x"}).Active() != true {
		t.Fatal("expected active with key+url+enabled")
	}
	if (BraveConfig{Enabled: true, BaseURL: "https://x"}).Active() != false {
		t.Fatal("expected inactive without key")
	}
	if (BraveConfig{Enabled: false, APIKey: "k", BaseURL: "https://x"}).Active() != false {
		t.Fatal("expected inactive when disabled")
	}
}
