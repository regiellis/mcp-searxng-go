package fetch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractPDFText(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	content, truncated, err := ExtractPDFText(data, 0)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation with no cap")
	}
	for _, want := range []string{"caching strategies", "Fugu", "memory for speed"} {
		if !strings.Contains(content, want) {
			t.Fatalf("extracted text missing %q; got: %q", want, content)
		}
	}
}

func TestExtractPDFTextTruncates(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	content, truncated, err := ExtractPDFText(data, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(content) != 10 {
		t.Fatalf("expected 10-byte truncated content, got truncated=%v len=%d", truncated, len(content))
	}
}

func TestExtractPDFTextRejectsGarbage(t *testing.T) {
	t.Parallel()
	// Not a PDF: must return an error (recovered panic or parse error), never crash.
	if _, _, err := ExtractPDFText([]byte("this is plainly not a pdf"), 0); err == nil {
		t.Fatal("expected an error for non-PDF input")
	}
}
