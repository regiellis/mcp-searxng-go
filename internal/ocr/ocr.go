// Package ocr reads scanned/image-only PDFs by rasterizing each page with
// pdftoppm and running Tesseract on the page images. Both binaries are invoked
// with explicit argument vectors (never a shell) and all work happens in a
// temporary directory that is removed afterward. The binaries must be installed
// on the host; when they are absent the engine reports itself unavailable so the
// tool can degrade gracefully rather than failing the server.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
)

// Engine runs OCR over PDF documents.
type Engine struct {
	tesseract string
	pdftoppm  string
	languages string
	dpi       int
	maxPages  int
	timeout   time.Duration
	logger    *slog.Logger
}

// NewEngine returns an OCR engine from config, applying defaults for any unset
// fields.
func NewEngine(cfg config.OCRConfig, logger *slog.Logger) *Engine {
	e := &Engine{
		tesseract: firstNonEmpty(cfg.TesseractPath, "tesseract"),
		pdftoppm:  firstNonEmpty(cfg.PdftoppmPath, "pdftoppm"),
		languages: firstNonEmpty(cfg.Languages, "eng"),
		dpi:       cfg.DPI,
		maxPages:  cfg.MaxPages,
		timeout:   cfg.Timeout,
		logger:    logger,
	}
	if e.dpi <= 0 {
		e.dpi = 200
	}
	if e.maxPages <= 0 {
		e.maxPages = 10
	}
	if e.timeout <= 0 {
		e.timeout = 2 * time.Minute
	}
	return e
}

// Preflight reports whether the required binaries are resolvable on PATH.
func (e *Engine) Preflight() error {
	var errs []error
	if _, err := exec.LookPath(e.tesseract); err != nil {
		errs = append(errs, fmt.Errorf("tesseract not found (%s): %w", e.tesseract, err))
	}
	if _, err := exec.LookPath(e.pdftoppm); err != nil {
		errs = append(errs, fmt.Errorf("pdftoppm not found (%s): %w", e.pdftoppm, err))
	}
	return errors.Join(errs...)
}

// Available reports whether OCR can actually run on this host.
func (e *Engine) Available() bool { return e.Preflight() == nil }

// Languages returns the configured Tesseract language string.
func (e *Engine) Languages() string { return e.languages }

// Result is the outcome of OCRing a document.
type Result struct {
	Text  string
	Pages int
}

// PDFText rasterizes the PDF and OCRs up to maxPages pages (0 uses the engine
// default). It returns the concatenated page text. The work directory is removed
// before returning.
func (e *Engine) PDFText(ctx context.Context, pdf []byte, maxPages int) (Result, error) {
	if err := e.Preflight(); err != nil {
		return Result{}, fmt.Errorf("ocr unavailable: %w", err)
	}
	if len(pdf) == 0 {
		return Result{}, errors.New("empty pdf input")
	}
	pages := e.maxPages
	if maxPages > 0 && maxPages < pages {
		pages = maxPages
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	workDir, err := os.MkdirTemp("", "ocr-")
	if err != nil {
		return Result{}, fmt.Errorf("create ocr workdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	inputPath := filepath.Join(workDir, "in.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o600); err != nil {
		return Result{}, fmt.Errorf("write pdf: %w", err)
	}

	// Rasterize pages 1..pages to <workDir>/page-N.png.
	prefix := filepath.Join(workDir, "page")
	if _, err := e.run(ctx, e.pdftoppm, []string{
		"-png",
		"-r", strconv.Itoa(e.dpi),
		"-f", "1",
		"-l", strconv.Itoa(pages),
		inputPath, prefix,
	}); err != nil {
		return Result{}, fmt.Errorf("rasterize pdf: %w", err)
	}

	images, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return Result{}, err
	}
	if len(images) == 0 {
		return Result{}, errors.New("pdf produced no rasterized pages")
	}
	sort.Strings(images)

	parts := make([]string, 0, len(images))
	for _, img := range images {
		out, err := e.run(ctx, e.tesseract, []string{img, "stdout", "-l", e.languages})
		if err != nil {
			return Result{}, fmt.Errorf("ocr page %s: %w", filepath.Base(img), err)
		}
		if text := strings.TrimSpace(out); text != "" {
			parts = append(parts, text)
		}
	}

	return Result{
		Text:  strings.TrimSpace(strings.Join(parts, "\n\n")),
		Pages: len(images),
	}, nil
}

// run executes a binary with an explicit argument vector and returns trimmed
// stdout. On a non-zero exit the trimmed stderr (capped) is included.
func (e *Engine) run(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s timed out or was cancelled: %w", filepath.Base(name), ctx.Err())
		}
		return "", fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, trimErr(stderr.String()))
	}
	return stdout.String(), nil
}

func trimErr(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	const max = 500
	if len(stderr) > max {
		return stderr[len(stderr)-max:]
	}
	return stderr
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
