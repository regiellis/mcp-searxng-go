package transcript

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Chapter segmentation defaults. These segment a transcript by caption timing
// alone — no LLM is involved, so transcript_chapters works without an API key.
const (
	defaultChapterMinSeconds = 60.0  // chapters shorter than this merge forward
	defaultChapterGapSeconds = 2.5   // a silence gap of at least this is a candidate boundary
	defaultChapterMaxSeconds = 300.0 // force a split once a chapter reaches this length
)

// ChapterOptions tunes how cues are grouped into chapters. Zero values fall back
// to the package defaults.
type ChapterOptions struct {
	MinSeconds float64
	GapSeconds float64
	MaxSeconds float64
}

func (o ChapterOptions) withDefaults() ChapterOptions {
	if o.MinSeconds <= 0 {
		o.MinSeconds = defaultChapterMinSeconds
	}
	if o.GapSeconds <= 0 {
		o.GapSeconds = defaultChapterGapSeconds
	}
	if o.MaxSeconds <= 0 {
		o.MaxSeconds = defaultChapterMaxSeconds
	}
	if o.MaxSeconds < o.MinSeconds {
		o.MaxSeconds = o.MinSeconds
	}
	return o
}

// cue is a single timestamped caption block.
type cue struct {
	start float64
	end   float64
	text  string
}

// Chapters parses a timestamped SRT/VTT transcript and segments it into
// time-bounded sections using caption timing only: a new chapter begins after a
// silence gap once the current chapter has met the minimum length, or whenever a
// chapter reaches the maximum length. This is deterministic structural
// segmentation, not semantic topic detection — it groups what was actually said
// and when, with no LLM. Preview lines are taken verbatim from the transcript.
func Chapters(raw string, opts ChapterOptions) (types.TranscriptChaptersResponse, error) {
	opts = opts.withDefaults()

	format := "srt"
	if strings.HasPrefix(strings.TrimPrefix(raw, "\uFEFF"), "WEBVTT") {
		format = "vtt"
	}

	cues := parseCues(raw)
	if len(cues) == 0 {
		return types.TranscriptChaptersResponse{}, fmt.Errorf("no timestamped cues found: transcript_chapters needs an SRT or VTT subtitle file")
	}

	chapters := make([]types.TranscriptChapter, 0)
	var curStart, curEnd float64
	var curParts []string
	open := false

	flush := func() {
		if !open {
			return
		}
		text := strings.TrimSpace(strings.Join(curParts, " "))
		chapters = append(chapters, types.TranscriptChapter{
			Index:           len(chapters),
			Start:           formatTimestamp(curStart),
			End:             formatTimestamp(curEnd),
			StartSeconds:    round1(curStart),
			DurationSeconds: round1(curEnd - curStart),
			Preview:         preview(text),
			Text:            text,
		})
	}

	for _, c := range cues {
		if open {
			dur := curEnd - curStart
			gap := c.start - curEnd
			if dur >= opts.MaxSeconds || (gap >= opts.GapSeconds && dur >= opts.MinSeconds) {
				flush()
				open = false
			}
		}
		if !open {
			curStart = c.start
			curParts = curParts[:0]
			open = true
		}
		curParts = append(curParts, c.text)
		curEnd = c.end
	}
	flush()

	return types.TranscriptChaptersResponse{
		Format:        format,
		CueCount:      len(cues),
		ChapterCount:  len(chapters),
		TotalDuration: formatTimestamp(cues[len(cues)-1].end - cues[0].start),
		Chapters:      chapters,
	}, nil
}

// parseCues extracts timestamped cues from SRT or VTT text. Cue indices, meta
// lines, and inline tags are dropped, and consecutive duplicate caption lines
// (the rolling-window artifact of auto-generated captions) are collapsed so a
// cue's text is not double-counted across overlapping blocks.
func parseCues(raw string) []cue {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	lines := strings.Split(raw, "\n")
	cues := make([]cue, 0)
	var prevLine string

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		if !isTimestampLine(line) {
			i++
			continue
		}
		start, end, ok := parseTiming(line)
		if !ok {
			i++
			continue
		}
		i++

		parts := make([]string, 0, 2)
		for i < len(lines) {
			t := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
			if t == "" {
				i++
				break
			}
			if isTimestampLine(t) {
				break // next cue with no blank separator
			}
			i++
			if isMetaLine(t) || isIndexLine(t) {
				continue
			}
			t = tagRE.ReplaceAllString(t, "")
			t = strings.TrimSpace(wsRE.ReplaceAllString(t, " "))
			if t == "" || t == prevLine {
				continue
			}
			parts = append(parts, t)
			prevLine = t
		}

		if text := strings.TrimSpace(strings.Join(parts, " ")); text != "" {
			cues = append(cues, cue{start: start, end: end, text: text})
		}
	}
	return cues
}

// parseTiming reads a cue timing line ("00:00:01,000 --> 00:00:04,000" or the
// VTT "." form, possibly with trailing cue settings) into start/end seconds.
func parseTiming(line string) (float64, float64, bool) {
	left, right, ok := strings.Cut(line, "-->")
	if !ok {
		return 0, 0, false
	}
	start, ok1 := parseTimestamp(left)
	// The right side may carry VTT cue settings after the timestamp.
	fields := strings.Fields(right)
	if len(fields) == 0 {
		return 0, 0, false
	}
	end, ok2 := parseTimestamp(fields[0])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return start, end, true
}

// parseTimestamp parses HH:MM:SS,mmm / HH:MM:SS.mmm / MM:SS.mmm into seconds.
func parseTimestamp(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var h, m float64
	var secField string
	if len(parts) == 3 {
		hh, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, false
		}
		mm, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, false
		}
		h, m, secField = hh, mm, parts[2]
	} else {
		mm, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, false
		}
		m, secField = mm, parts[1]
	}
	sec, err := strconv.ParseFloat(secField, 64)
	if err != nil {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

// preview returns a short verbatim opening of a chapter: the first sentence when
// one ends early, otherwise a word-boundary truncation.
func preview(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const limit = 160
	scan := text
	if len(scan) > limit+40 {
		scan = scan[:limit+40]
	}
	for i, r := range scan {
		if (r == '.' || r == '?' || r == '!') && i >= 20 {
			return strings.TrimSpace(scan[:i+1])
		}
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	if idx := strings.LastIndexByte(text[:limit], ' '); idx > limit/2 {
		cut = idx
	}
	return strings.TrimSpace(text[:cut]) + "…"
}

// formatTimestamp renders seconds as H:MM:SS (or M:SS under an hour).
func formatTimestamp(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	whole := int64(sec + 0.5)
	h := whole / 3600
	m := (whole % 3600) / 60
	s := whole % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
