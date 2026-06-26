package fetch

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

// pdfSpaceRE collapses the runs of whitespace that PDF text extraction tends to
// emit between glyphs and lines.
var pdfSpaceRE = regexp.MustCompile(`[ \t]{2,}`)

// ExtractPDFText extracts plain text from an in-memory PDF document, capped at
// maxChars (truncated reports whether the cap was hit). PDF parsing libraries
// can panic on malformed or unusual input, so a panic is recovered and returned
// as an error rather than taking down the caller. Text-layer PDFs extract
// cleanly; scanned/image-only PDFs yield little or no text and would need OCR.
func ExtractPDFText(data []byte, maxChars int) (content string, truncated bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			content, truncated, err = "", false, fmt.Errorf("pdf parse panicked: %v", r)
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false, fmt.Errorf("read pdf: %w", err)
	}
	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", false, fmt.Errorf("extract pdf text: %w", err)
	}
	var buf strings.Builder
	if _, err := io.Copy(&buf, textReader); err != nil {
		return "", false, fmt.Errorf("copy pdf text: %w", err)
	}

	text := normalizePDFText(buf.String())
	if text == "" {
		return "", false, fmt.Errorf("no extractable text in pdf (it may be scanned/image-only)")
	}
	if maxChars > 0 && len(text) > maxChars {
		return text[:maxChars], true, nil
	}
	return text, false, nil
}

func normalizePDFText(raw string) string {
	raw = strings.ReplaceAll(raw, "\x00", "")
	raw = pdfSpaceRE.ReplaceAllString(raw, " ")
	lines := strings.Split(raw, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
