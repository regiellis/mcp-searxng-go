package media

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type okGuard struct{}

func (okGuard) Validate(_ context.Context, raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func newRunner(t *testing.T, cfg config.MediaConfig) *Runner {
	t.Helper()
	if strings.TrimSpace(cfg.OutputDir) == "" {
		cfg.OutputDir = t.TempDir()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	r, err := NewRunner(cfg, okGuard{}, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveInOutputRejectsTraversal(t *testing.T) {
	t.Parallel()
	r := newRunner(t, config.MediaConfig{})

	if _, err := r.resolveInOutput("../escape.mp4"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if _, err := r.resolveInOutput("/etc/passwd"); err == nil {
		t.Fatal("expected absolute path outside sandbox to be rejected")
	}
	got, err := r.resolveInOutput("clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(r.OutputDir(), "clip.mp4"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSanitizeExt(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		".MP4":      "mp4",
		"webm":      "webm",
		"mp3":       "mp3",
		"../etc":    "",
		"mp4;rm":    "",
		"sub title": "",
	}
	for in, want := range cases {
		if got := sanitizeExt(in); got != want {
			t.Fatalf("sanitizeExt(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePrintLine(t *testing.T) {
	t.Parallel()
	path, title, duration := parsePrintLine("/media/abc.mp4\tMy Title\t3:21")
	if path != "/media/abc.mp4" || title != "My Title" || duration != "3:21" {
		t.Fatalf("unexpected parse: %q %q %q", path, title, duration)
	}
	// NA fields are cleaned to empty.
	_, title, duration = parsePrintLine("/media/abc.mp4\tNA\tNA")
	if title != "" || duration != "" {
		t.Fatalf("expected NA cleaned, got %q %q", title, duration)
	}
}

func TestSubtitleLanguage(t *testing.T) {
	t.Parallel()
	if got := subtitleLanguage("video123.en.srt", "xx"); got != "en" {
		t.Fatalf("expected en, got %q", got)
	}
	if got := subtitleLanguage("plain.srt", "es"); got != "es" {
		t.Fatalf("expected fallback es, got %q", got)
	}
}

func TestOutputPathForRejectsBadName(t *testing.T) {
	t.Parallel()
	r := newRunner(t, config.MediaConfig{})
	if _, err := r.outputPathFor("/x/in.mkv", "../evil", "mp4"); err == nil {
		t.Fatal("expected invalid output_name rejection")
	}
	got, err := r.outputPathFor(filepath.Join(r.OutputDir(), "in.mkv"), "", "mp4")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(r.OutputDir(), "in.transcoded.mp4"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// writeStub writes an executable shell script and returns its path.
func writeStub(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub not supported on windows")
	}
	path := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTranscodeRunsFfmpegAndSandboxesInput(t *testing.T) {
	dir := t.TempDir()
	// Fake ffmpeg: the output file is the last argument; create it.
	ffmpeg := writeStub(t, `for last; do :; done; printf 'transcoded' > "$last"`)
	r := newRunner(t, config.MediaConfig{OutputDir: dir, FfmpegPath: ffmpeg})

	input := filepath.Join(dir, "clip.mkv")
	if err := os.WriteFile(input, []byte("source-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := r.Transcode(context.Background(), types.TranscodeRequest{Path: "clip.mkv", Format: "mp4", CRF: 28})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Output.Filename != "clip.transcoded.mp4" {
		t.Fatalf("unexpected output name: %q", resp.Output.Filename)
	}
	if resp.Output.SizeBytes == 0 {
		t.Fatal("expected non-empty output")
	}

	// Input outside the sandbox must be rejected before ffmpeg runs.
	if _, err := r.Transcode(context.Background(), types.TranscodeRequest{Path: "/etc/hosts"}); err == nil {
		t.Fatal("expected sandbox rejection for outside path")
	}
}

func TestDownloadParsesYtDlpOutput(t *testing.T) {
	dir := t.TempDir()
	// Fake yt-dlp: locate -P dir, create a file there, print the print-template line.
	ytdlp := writeStub(t, `
outdir="."
prev=""
for a in "$@"; do
  if [ "$prev" = "-P" ]; then outdir="$a"; fi
  prev="$a"
done
f="$outdir/video123.mp4"
printf 'data' > "$f"
printf '%s\tExample Video\t1:02\n' "$f"
`)
	r := newRunner(t, config.MediaConfig{OutputDir: dir, YtDlpPath: ytdlp})

	resp, err := r.Download(context.Background(), types.DownloadVideoRequest{URL: "https://example.com/watch?v=123"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.File.Filename != "video123.mp4" || resp.File.Title != "Example Video" || resp.File.Duration != "1:02" {
		t.Fatalf("unexpected download response: %#v", resp.File)
	}
	if resp.File.SizeBytes != 4 {
		t.Fatalf("expected size 4, got %d", resp.File.SizeBytes)
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := newRunner(t, config.MediaConfig{OutputDir: dir})

	// Text file (e.g. a subtitle) is returned verbatim as UTF-8.
	srt := "1\n00:00:01,000 --> 00:00:02,000\nHello\n"
	if err := os.WriteFile(filepath.Join(dir, "video.en.srt"), []byte(srt), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, err := r.ReadFile(context.Background(), types.ReadMediaFileRequest{Path: "video.en.srt"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Encoding != "text" || resp.Content != srt {
		t.Fatalf("unexpected text read: encoding=%q content=%q", resp.Encoding, resp.Content)
	}
	if resp.Truncated {
		t.Fatal("did not expect truncation")
	}

	// Binary content is base64-encoded.
	bin := []byte{0x00, 0xff, 0xfe, 0x01}
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), bin, 0o644); err != nil {
		t.Fatal(err)
	}
	resp, err = r.ReadFile(context.Background(), types.ReadMediaFileRequest{Path: "blob.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Encoding != "base64" {
		t.Fatalf("expected base64 encoding, got %q", resp.Encoding)
	}

	// max_bytes truncates and flags it.
	resp, err = r.ReadFile(context.Background(), types.ReadMediaFileRequest{Path: "video.en.srt", MaxBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated || len(resp.Content) != 5 {
		t.Fatalf("expected truncated 5-byte content, got truncated=%v len=%d", resp.Truncated, len(resp.Content))
	}

	// Paths outside the sandbox are rejected.
	if _, err := r.ReadFile(context.Background(), types.ReadMediaFileRequest{Path: "/etc/passwd"}); err == nil {
		t.Fatal("expected sandbox rejection for outside path")
	}
}

func TestProbeParsesFfprobeJSON(t *testing.T) {
	dir := t.TempDir()
	const probeJSON = `{
  "streams": [
    {"index": 0, "codec_name": "h264", "codec_type": "video", "profile": "High", "width": 1920, "height": 1080, "avg_frame_rate": "30000/1001", "tags": {"language": "eng"}},
    {"index": 1, "codec_name": "aac", "codec_type": "audio", "channels": 2, "sample_rate": "48000", "tags": {"language": "eng", "title": "Stereo"}}
  ],
  "format": {"filename": "clip.mp4", "format_name": "mov,mp4,m4a,3gp,3g2,mj2", "format_long_name": "QuickTime / MOV", "duration": "3725.500000", "size": "12345678", "bit_rate": "1500000"}
}`
	// Fake ffprobe ignores its args and prints the canned JSON document.
	ffprobe := writeStub(t, `cat <<'EOF'
`+probeJSON+`
EOF`)
	r := newRunner(t, config.MediaConfig{OutputDir: dir, FfprobePath: ffprobe})

	if err := os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := r.Probe(context.Background(), types.ProbeMediaRequest{Path: "clip.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" || resp.DurationHuman != "1:02:05" {
		t.Fatalf("unexpected format/duration: %q %q", resp.FormatName, resp.DurationHuman)
	}
	if resp.StreamCount != 2 || len(resp.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", resp.StreamCount)
	}
	video := resp.Streams[0]
	if video.Type != "video" || video.Codec != "h264" || video.Width != 1920 || video.Height != 1080 {
		t.Fatalf("unexpected video stream: %#v", video)
	}
	if video.FrameRate != "29.97" {
		t.Fatalf("expected frame rate 29.97, got %q", video.FrameRate)
	}
	audio := resp.Streams[1]
	if audio.Type != "audio" || audio.Channels != 2 || audio.SampleRate != "48000" || audio.Title != "Stereo" {
		t.Fatalf("unexpected audio stream: %#v", audio)
	}

	// Paths outside the sandbox are rejected before ffprobe runs.
	if _, err := r.Probe(context.Background(), types.ProbeMediaRequest{Path: "/etc/passwd"}); err == nil {
		t.Fatal("expected sandbox rejection for outside path")
	}
}

func TestSimplifyFrameRate(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"30/1":       "30",
		"30000/1001": "29.97",
		"24/1":       "24",
		"0/0":        "",
		"":           "",
		"60":         "60",
	}
	for in, want := range cases {
		if got := simplifyFrameRate(in); got != want {
			t.Fatalf("simplifyFrameRate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"3725.5": "1:02:05",
		"125.0":  "2:05",
		"5":      "0:05",
		"":       "",
		"bad":    "",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Fatalf("humanDuration(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPreflightReportsMissingBinaries(t *testing.T) {
	t.Parallel()
	r := newRunner(t, config.MediaConfig{YtDlpPath: "definitely-not-a-real-binary-xyz", FfmpegPath: "also-not-real-xyz"})
	if err := r.Preflight(); err == nil {
		t.Fatal("expected preflight to report missing binaries")
	}
}
