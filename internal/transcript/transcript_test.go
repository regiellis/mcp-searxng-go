package transcript

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeStripsSRTCues(t *testing.T) {
	t.Parallel()
	raw := "1\n00:00:01,000 --> 00:00:02,000\nHello world\n\n2\n00:00:02,000 --> 00:00:04,000\nThis is a test\n"
	got := Normalize(raw)
	if got != "Hello world This is a test" {
		t.Fatalf("unexpected normalize: %q", got)
	}
}

func TestNormalizeVTTAndDedup(t *testing.T) {
	t.Parallel()
	// Auto-generated VTT: header, inline tags, and rolling duplicate lines.
	raw := "\uFEFFWEBVTT\nKind: captions\nLanguage: en\n\n" +
		"00:00:00.000 --> 00:00:02.000\n<c>welcome</c> back\n" +
		"00:00:02.000 --> 00:00:04.000\nwelcome back\n" +
		"00:00:04.000 --> 00:00:06.000\nto the show\n"
	got := Normalize(raw)
	if got != "welcome back to the show" {
		t.Fatalf("unexpected vtt normalize: %q", got)
	}
}

func TestChunkBreaksOnWhitespace(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("word ", 100) // 500 chars
	chunks := Chunk(strings.TrimSpace(text), 120)
	if len(chunks) < 4 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 120 {
			t.Fatalf("chunk %d exceeds limit: %d", i, len(c))
		}
		if strings.HasPrefix(c, " ") || strings.HasSuffix(c, " ") {
			t.Fatalf("chunk %d not trimmed: %q", i, c)
		}
	}
	// Reassembled words must be preserved (no split tokens).
	if joined := strings.Join(chunks, " "); strings.Contains(joined, "wor d") {
		t.Fatal("a word was split across chunks")
	}
}

type fakeCompleter struct {
	calls   int
	lastSys string
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string) (string, error) {
	f.calls++
	f.lastSys = system
	// Echo the user content prefixed so we can assert it was sent.
	return "CLEANED:" + user, nil
}

func (f *fakeCompleter) Model() string { return "fake-model" }

func TestTranslateProcessesAllChunksAndSetsLanguage(t *testing.T) {
	t.Parallel()
	fake := &fakeCompleter{}

	raw := "1\n00:00:01,000 --> 00:00:02,000\n" + strings.Repeat("hola ", 30) + "\n"
	resp, err := Translate(context.Background(), fake, raw, "  Spanish  ", 30)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TargetLanguage != "Spanish" {
		t.Fatalf("target language not trimmed/set: %q", resp.TargetLanguage)
	}
	if resp.Chunks < 2 || fake.calls != resp.Chunks {
		t.Fatalf("expected calls==chunks>=2, got calls=%d chunks=%d", fake.calls, resp.Chunks)
	}
	if resp.Model != "fake-model" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if !strings.Contains(fake.lastSys, "into Spanish") {
		t.Fatalf("target language not injected into system prompt: %q", fake.lastSys)
	}
}

func TestTranslateRejectsEmptyInputs(t *testing.T) {
	t.Parallel()
	if _, err := Translate(context.Background(), &fakeCompleter{}, "1\n00:00:01,000 --> 00:00:02,000\n\n", "French", 0); err == nil {
		t.Fatal("expected error for transcript with no text")
	}
	if _, err := Translate(context.Background(), &fakeCompleter{}, "some text", "   ", 0); err == nil {
		t.Fatal("expected error for missing target language")
	}
}

func TestCleanProcessesAllChunks(t *testing.T) {
	t.Parallel()
	fake := &fakeCompleter{}
	c := NewCleaner(fake, 30) // small budget forces multiple chunks

	raw := "1\n00:00:01,000 --> 00:00:02,000\n" + strings.Repeat("alpha ", 30) + "\n"
	resp, err := c.Clean(context.Background(), raw, "alphabets")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Chunks < 2 || fake.calls != resp.Chunks {
		t.Fatalf("expected calls==chunks>=2, got calls=%d chunks=%d", fake.calls, resp.Chunks)
	}
	if resp.Model != "fake-model" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if !strings.Contains(resp.Content, "CLEANED:") {
		t.Fatalf("expected cleaned content, got %q", resp.Content)
	}
	if !strings.Contains(fake.lastSys, "topic of interest is: alphabets") {
		t.Fatalf("topic hint not injected into system prompt: %q", fake.lastSys)
	}
}

func TestCleanRejectsEmptyTranscript(t *testing.T) {
	t.Parallel()
	c := NewCleaner(&fakeCompleter{}, 0)
	// Only cues and timestamps, no text.
	raw := "1\n00:00:01,000 --> 00:00:02,000\n\n2\n00:00:02,000 --> 00:00:03,000\n"
	if _, err := c.Clean(context.Background(), raw, ""); err == nil {
		t.Fatal("expected error for transcript with no text")
	}
}
