package transcript

import (
	"strings"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	t.Parallel()
	cases := map[string]float64{
		"00:00:01,500": 1.5,    // SRT comma
		"00:01:02.250": 62.25,  // VTT dot, with hours
		"01:02.000":    62.0,   // VTT, hours omitted
		"1:00:00.000":  3600.0, // one hour
	}
	for in, want := range cases {
		got, ok := parseTimestamp(in)
		if !ok || got != want {
			t.Fatalf("parseTimestamp(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parseTimestamp("not-a-time"); ok {
		t.Fatal("expected failure on garbage timestamp")
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()
	cases := map[float64]string{
		5:    "0:05",
		65:   "1:05",
		3725: "1:02:05",
	}
	for in, want := range cases {
		if got := formatTimestamp(in); got != want {
			t.Fatalf("formatTimestamp(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestChaptersSplitsOnGapAndLength(t *testing.T) {
	t.Parallel()
	// Two cues close together, then a long silence, then a third cue. With a low
	// min length the silence is a chapter boundary.
	srt := strings.Join([]string{
		"1",
		"00:00:00,000 --> 00:00:03,000",
		"Welcome to the talk about caching.",
		"",
		"2",
		"00:00:03,000 --> 00:00:06,000",
		"Caching trades memory for speed.",
		"",
		"3",
		"00:00:30,000 --> 00:00:34,000",
		"Now a completely separate topic begins.",
		"",
	}, "\n")

	resp, err := Chapters(srt, ChapterOptions{MinSeconds: 5, GapSeconds: 2.5, MaxSeconds: 300})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Format != "srt" {
		t.Fatalf("expected srt format, got %q", resp.Format)
	}
	if resp.CueCount != 3 {
		t.Fatalf("expected 3 cues, got %d", resp.CueCount)
	}
	if resp.ChapterCount != 2 {
		t.Fatalf("expected 2 chapters from the silence split, got %d", resp.ChapterCount)
	}
	if resp.Chapters[0].Start != "0:00" || resp.Chapters[1].Start != "0:30" {
		t.Fatalf("unexpected chapter starts: %q / %q", resp.Chapters[0].Start, resp.Chapters[1].Start)
	}
	if !strings.Contains(resp.Chapters[0].Text, "Caching trades memory") {
		t.Fatalf("first chapter should accumulate both early cues: %q", resp.Chapters[0].Text)
	}
	if resp.Chapters[0].Preview != "Welcome to the talk about caching." {
		t.Fatalf("unexpected preview: %q", resp.Chapters[0].Preview)
	}
}

func TestChaptersParsesVTTAndDedupes(t *testing.T) {
	t.Parallel()
	// Auto-caption style VTT with a rolling duplicate line and inline tags.
	vtt := strings.Join([]string{
		"WEBVTT",
		"Kind: captions",
		"Language: en",
		"",
		"00:00:00.000 --> 00:00:02.000",
		"<c>hello there</c>",
		"",
		"00:00:02.000 --> 00:00:04.000",
		"hello there",
		"",
		"00:00:04.000 --> 00:00:06.000",
		"this is the actual content",
		"",
	}, "\n")

	resp, err := Chapters(vtt, ChapterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Format != "vtt" {
		t.Fatalf("expected vtt format, got %q", resp.Format)
	}
	// One chapter (all cues within default min length, no large gap).
	if resp.ChapterCount != 1 {
		t.Fatalf("expected 1 chapter, got %d", resp.ChapterCount)
	}
	text := resp.Chapters[0].Text
	if strings.Count(text, "hello there") != 1 {
		t.Fatalf("rolling duplicate not collapsed: %q", text)
	}
	if !strings.Contains(text, "this is the actual content") {
		t.Fatalf("missing content cue: %q", text)
	}
}

func TestChaptersRejectsUntimedText(t *testing.T) {
	t.Parallel()
	if _, err := Chapters("just a plain paragraph with no timestamps", ChapterOptions{}); err == nil {
		t.Fatal("expected error when no cues are present")
	}
}
