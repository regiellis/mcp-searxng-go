// Package transcript turns noisy subtitle files (SRT/VTT, including auto-
// generated captions) into clean, on-topic prose. It strips cue numbers,
// timestamps, and rolling-caption duplicates, then sends the text through an
// LLM that removes non-content material — channel intros and outros, like/
// subscribe asks, sponsor reads and ads, and caption filler — while preserving
// the substantive content verbatim rather than summarizing it.
package transcript

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Completer is the subset of an LLM client this package needs.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Model() string
}

// defaultMaxInputChars bounds a single chunk when none is configured.
const defaultMaxInputChars = 48000

var (
	tagRE = regexp.MustCompile(`<[^>]+>`)        // VTT inline tags: <c>, <00:00:01.000>, <i>
	wsRE  = regexp.MustCompile(`[ \t\x{00a0}]+`) // collapse runs of spaces/tabs/nbsp
)

const systemPrompt = `You are a transcript editor. You are given the raw text of an automatically generated transcript from a video or audio recording. Your job is to return a cleaned transcript.

Remove everything that is not part of the substantive subject matter, including:
- channel or show intros and outros
- greetings, sign-offs, and "thanks for watching"
- requests to like, subscribe, comment, share, or hit the bell
- sponsor reads, advertisements, and self-promotion
- repeated or duplicated caption lines and obvious filler

Preserve ALL substantive on-topic content: explanations, arguments, facts, instructions, examples, and details. Do NOT write a summary and do NOT shorten the substance — keep the full meaning. Repair caption artifacts by restoring sentence casing and punctuation and joining fragmented lines into readable paragraphs.

Output only the cleaned transcript text. Do not add headings, commentary, notes, or markdown.`

// Cleaner cleans transcripts using a Completer.
type Cleaner struct {
	llm           Completer
	maxInputChars int
}

// NewCleaner returns a Cleaner. maxInputChars bounds each LLM request so long
// transcripts are processed in order across multiple calls.
func NewCleaner(llm Completer, maxInputChars int) *Cleaner {
	if maxInputChars <= 0 {
		maxInputChars = defaultMaxInputChars
	}
	return &Cleaner{llm: llm, maxInputChars: maxInputChars}
}

// Clean normalizes raw subtitle text and runs it through the LLM. topic is an
// optional hint that tightens what counts as off-topic. The returned response
// has SourcePath and SavedPath left to the caller to populate.
func (c *Cleaner) Clean(ctx context.Context, raw, topic string) (types.CleanSubtitlesResponse, error) {
	normalized := Normalize(raw)
	if strings.TrimSpace(normalized) == "" {
		return types.CleanSubtitlesResponse{}, fmt.Errorf("no transcript text found in input")
	}

	system := systemPrompt
	if t := strings.TrimSpace(topic); t != "" {
		system += "\n\nThe topic of interest is: " + t + ". Treat material unrelated to this topic as removable."
	}

	chunks := Chunk(normalized, c.maxInputChars)
	parts := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		out, err := c.llm.Complete(ctx, system, chunk)
		if err != nil {
			return types.CleanSubtitlesResponse{}, fmt.Errorf("clean chunk %d/%d: %w", i+1, len(chunks), err)
		}
		if out = strings.TrimSpace(out); out != "" {
			parts = append(parts, out)
		}
	}

	content := strings.Join(parts, "\n\n")
	return types.CleanSubtitlesResponse{
		Topic:       strings.TrimSpace(topic),
		Model:       c.llm.Model(),
		Chunks:      len(chunks),
		InputChars:  len(normalized),
		OutputChars: len(content),
		Content:     content,
	}, nil
}

// Normalize converts SRT/VTT subtitle text into a single plaintext stream by
// dropping cue indices, timestamps, headers, and inline tags, and collapsing the
// consecutive duplicate lines that auto-generated captions emit.
func Normalize(raw string) string {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	lines := strings.Split(raw, "\n")
	kept := make([]string, 0, len(lines))
	var prev string
	for _, line := range lines {
		t := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if t == "" {
			continue
		}
		if isMetaLine(t) || isTimestampLine(t) || isIndexLine(t) {
			continue
		}
		t = tagRE.ReplaceAllString(t, "")
		t = strings.TrimSpace(wsRE.ReplaceAllString(t, " "))
		if t == "" || t == prev {
			continue
		}
		kept = append(kept, t)
		prev = t
	}
	return strings.Join(kept, " ")
}

func isMetaLine(s string) bool {
	if s == "WEBVTT" {
		return true
	}
	for _, prefix := range []string{"Kind:", "Language:", "NOTE", "STYLE", "REGION"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// isTimestampLine matches SRT/VTT cue timing lines, which always contain "-->".
func isTimestampLine(s string) bool {
	return strings.Contains(s, "-->")
}

// isIndexLine matches a bare SRT cue number.
func isIndexLine(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Chunk splits text into pieces no larger than maxChars, breaking on whitespace
// so words are never split mid-token.
func Chunk(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxChars <= 0 || len(text) <= maxChars {
		return []string{text}
	}
	var chunks []string
	for len(text) > maxChars {
		cut := maxChars
		if idx := strings.LastIndexByte(text[:maxChars], ' '); idx > maxChars/2 {
			cut = idx
		}
		if part := strings.TrimSpace(text[:cut]); part != "" {
			chunks = append(chunks, part)
		}
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
