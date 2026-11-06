package ocr

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regiellis/mcp-searxng-go/internal/config"
)

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func newEngine() *Engine {
	return NewEngine(config.OCRConfig{Enabled: true, Languages: "eng", DPI: 200, MaxPages: 5},
		slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
}

func TestPDFTextOCR(t *testing.T) {
	e := newEngine()
	if !e.Available() {
		t.Skip("tesseract/pdftoppm not installed; skipping OCR pipeline test")
	}
	data, err := os.ReadFile(filepath.Join("testdata", "scanned.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.PDFText(context.Background(), data, 0)
	if err != nil {
		t.Fatalf("ocr: %v", err)
	}
	if res.Pages != 1 {
		t.Fatalf("expected 1 page, got %d", res.Pages)
	}
	// OCR is fuzzy; assert on robust tokens rather than exact strings.
	lower := strings.ToLower(res.Text)
	for _, want := range []string{"invoice", "4200", "payment"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("OCR text missing %q; got: %q", want, res.Text)
		}
	}
}

func TestPDFTextRejectsEmpty(t *testing.T) {
	t.Parallel()
	e := newEngine()
	if !e.Available() {
		t.Skip("ocr binaries not installed")
	}
	if _, err := e.PDFText(context.Background(), nil, 0); err == nil {
		t.Fatal("expected error for empty pdf input")
	}
}

func TestUnavailableEngineReportsClearly(t *testing.T) {
	t.Parallel()
	e := NewEngine(config.OCRConfig{
		TesseractPath: "definitely-not-real-xyz",
		PdftoppmPath:  "also-not-real-xyz",
	}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if e.Available() {
		t.Fatal("expected engine to be unavailable with bogus binaries")
	}
	if _, err := e.PDFText(context.Background(), []byte("x"), 0); err == nil {
		t.Fatal("expected PDFText to error when binaries are missing")
	}
}
