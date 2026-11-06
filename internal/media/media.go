// Package media wraps yt-dlp and ffmpeg to download videos, transcode/compress
// media, and pull subtitles. All output is confined to a configured directory
// and every external process is invoked with an explicit argument vector (never
// a shell) so caller-supplied values cannot inject commands.
package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Read size limits for ReadFile. The default keeps subtitle/text retrieval cheap
// while the ceiling caps how much a single response can return regardless of the
// caller-supplied max_bytes.
const (
	defaultReadLimit int64 = 1 << 20  // 1 MiB
	maxReadLimit     int64 = 16 << 20 // 16 MiB
)

// URLGuard validates a URL's scheme and resolves its host against SSRF policy
// before it is handed to yt-dlp. It is satisfied by fetch.URLValidator.
type URLGuard interface {
	Validate(ctx context.Context, rawURL string) (*url.URL, error)
}

// audioFormats are container extensions treated as audio-only on transcode.
var audioFormats = map[string]struct{}{
	"mp3": {}, "m4a": {}, "aac": {}, "opus": {}, "ogg": {}, "oga": {}, "wav": {}, "flac": {},
}

// Runner executes media operations within a sandboxed output directory.
type Runner struct {
	outputDir   string
	ytDlpPath   string
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
	guard       URLGuard
	logger      *slog.Logger
}

// NewRunner creates the output directory and returns a configured Runner.
func NewRunner(cfg config.MediaConfig, guard URLGuard, logger *slog.Logger) (*Runner, error) {
	outputDir, err := filepath.Abs(strings.TrimSpace(cfg.OutputDir))
	if err != nil {
		return nil, fmt.Errorf("resolve media output_dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, fmt.Errorf("create media output_dir: %w", err)
	}
	return &Runner{
		outputDir:   outputDir,
		ytDlpPath:   firstNonEmpty(cfg.YtDlpPath, "yt-dlp"),
		ffmpegPath:  firstNonEmpty(cfg.FfmpegPath, "ffmpeg"),
		ffprobePath: firstNonEmpty(cfg.FfprobePath, "ffprobe"),
		timeout:     cfg.Timeout,
		guard:       guard,
		logger:      logger,
	}, nil
}

// Preflight reports whether the configured binaries are resolvable on PATH.
// Missing binaries are returned as a joined error so callers can warn without
// failing startup (the media tools simply error when invoked).
func (r *Runner) Preflight() error {
	var errs []error
	if _, err := exec.LookPath(r.ytDlpPath); err != nil {
		errs = append(errs, fmt.Errorf("yt-dlp not found (%s): %w", r.ytDlpPath, err))
	}
	if _, err := exec.LookPath(r.ffmpegPath); err != nil {
		errs = append(errs, fmt.Errorf("ffmpeg not found (%s): %w", r.ffmpegPath, err))
	}
	if _, err := exec.LookPath(r.ffprobePath); err != nil {
		errs = append(errs, fmt.Errorf("ffprobe not found (%s): %w", r.ffprobePath, err))
	}
	return errors.Join(errs...)
}

// OutputDir returns the sandbox root for produced files.
func (r *Runner) OutputDir() string { return r.outputDir }

// ResolvePath validates a caller-supplied path (absolute or relative) against
// the sandbox and returns its absolute location inside the output directory.
// It exists so transports (for example the HTTP /files endpoint) can serve
// produced files with the same traversal defenses as the media tools.
func (r *Runner) ResolvePath(raw string) (string, error) { return r.resolveInOutput(raw) }

// Download fetches the best-quality stream (or audio) for a URL via yt-dlp.
func (r *Runner) Download(ctx context.Context, req types.DownloadVideoRequest) (types.DownloadVideoResponse, error) {
	sourceURL := strings.TrimSpace(req.URL)
	if _, err := r.guard.Validate(ctx, sourceURL); err != nil {
		return types.DownloadVideoResponse{}, err
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	args := []string{
		"--no-playlist", "--no-progress", "--restrict-filenames",
		"-P", r.outputDir,
		"-o", "%(id)s.%(ext)s",
	}
	switch {
	case req.AudioOnly:
		args = append(args, "-x", "--audio-format", "mp3")
	case strings.TrimSpace(req.Format) != "":
		args = append(args, "-f", strings.TrimSpace(req.Format))
	default:
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4")
	}
	// Print the final post-move filepath plus metadata on a single tab-delimited line.
	args = append(args,
		"--no-simulate",
		"--print", "after_move:%(filepath)s\t%(title)s\t%(duration_string)s",
		"--", sourceURL,
	)

	out, err := r.run(ctx, r.ytDlpPath, args)
	if err != nil {
		return types.DownloadVideoResponse{}, err
	}
	path, title, duration := parsePrintLine(out)
	if path == "" {
		return types.DownloadVideoResponse{}, errors.New("yt-dlp did not report an output file")
	}
	file, err := r.statFile(path)
	if err != nil {
		return types.DownloadVideoResponse{}, err
	}
	file.Title = title
	file.Duration = duration
	return types.DownloadVideoResponse{SourceURL: sourceURL, File: file}, nil
}

// Transcode converts/compresses a file already inside the output directory.
func (r *Runner) Transcode(ctx context.Context, req types.TranscodeRequest) (types.TranscodeResponse, error) {
	inputPath, err := r.resolveInOutput(req.Path)
	if err != nil {
		return types.TranscodeResponse{}, err
	}
	inputInfo, err := r.statFile(inputPath)
	if err != nil {
		return types.TranscodeResponse{}, err
	}

	format := sanitizeExt(req.Format)
	if format == "" {
		format = "mp4"
	}
	outputPath, err := r.outputPathFor(inputPath, req.OutputName, format)
	if err != nil {
		return types.TranscodeResponse{}, err
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	args := []string{"-y", "-i", inputPath}
	if _, audioOnly := audioFormats[format]; audioOnly {
		args = append(args, "-vn")
		args = append(args, "-c:a", firstNonEmpty(req.AudioCodec, defaultAudioCodec(format)))
	} else {
		if req.MaxWidth > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale='min(%d,iw)':-2", req.MaxWidth))
		}
		args = append(args, "-c:v", firstNonEmpty(req.VideoCodec, "libx264"))
		if req.CRF > 0 {
			args = append(args, "-crf", strconv.Itoa(req.CRF))
		} else if req.VideoCodec == "" {
			args = append(args, "-crf", "23")
		}
		args = append(args, "-c:a", firstNonEmpty(req.AudioCodec, "aac"))
	}
	args = append(args, outputPath)

	if _, err := r.run(ctx, r.ffmpegPath, args); err != nil {
		return types.TranscodeResponse{}, err
	}
	outputInfo, err := r.statFile(outputPath)
	if err != nil {
		return types.TranscodeResponse{}, err
	}
	return types.TranscodeResponse{Input: inputInfo, Output: outputInfo}, nil
}

// Subtitles downloads subtitle tracks for a URL into a unique sub-directory.
func (r *Runner) Subtitles(ctx context.Context, req types.SubtitlesRequest) (types.SubtitlesResponse, error) {
	sourceURL := strings.TrimSpace(req.URL)
	if _, err := r.guard.Validate(ctx, sourceURL); err != nil {
		return types.SubtitlesResponse{}, err
	}

	lang := firstNonEmpty(strings.TrimSpace(req.Language), "en")
	format := sanitizeExt(req.Format)
	if format == "" {
		format = "srt"
	}

	subDir, err := os.MkdirTemp(r.outputDir, "subs-")
	if err != nil {
		return types.SubtitlesResponse{}, fmt.Errorf("create subtitle dir: %w", err)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	args := []string{
		"--skip-download", "--no-progress", "--restrict-filenames",
		"--write-subs",
		"--sub-langs", lang,
		"--convert-subs", format,
		"-P", subDir,
		"-o", "%(id)s.%(ext)s",
	}
	if req.AutoGenerated {
		args = append(args, "--write-auto-subs")
	}
	args = append(args, "--", sourceURL)

	if _, err := r.run(ctx, r.ytDlpPath, args); err != nil {
		return types.SubtitlesResponse{}, err
	}

	entries, err := os.ReadDir(subDir)
	if err != nil {
		return types.SubtitlesResponse{}, err
	}
	files := make([]types.MediaFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		file, statErr := r.statFile(filepath.Join(subDir, entry.Name()))
		if statErr != nil {
			continue
		}
		file.Language = subtitleLanguage(entry.Name(), lang)
		files = append(files, file)
	}
	if len(files) == 0 {
		_ = os.RemoveAll(subDir)
		return types.SubtitlesResponse{}, fmt.Errorf("no subtitles found for language %q", lang)
	}
	return types.SubtitlesResponse{SourceURL: sourceURL, Files: files}, nil
}

// ffprobeOutput mirrors the subset of `ffprobe -show_format -show_streams -of
// json` that probe_media exposes. Numeric container fields (duration, size,
// bit_rate) are reported as strings by ffprobe.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	Index        int               `json:"index"`
	CodecName    string            `json:"codec_name"`
	CodecType    string            `json:"codec_type"`
	Profile      string            `json:"profile"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	RFrameRate   string            `json:"r_frame_rate"`
	AvgFrameRate string            `json:"avg_frame_rate"`
	Channels     int               `json:"channels"`
	SampleRate   string            `json:"sample_rate"`
	Tags         map[string]string `json:"tags"`
}

type ffprobeFormat struct {
	Filename       string `json:"filename"`
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
}

// Probe returns container and per-stream metadata for a file inside the output
// directory using ffprobe, without downloading or re-encoding it. This lets a
// caller inspect duration, codecs, and resolution before deciding how (or
// whether) to transcode.
func (r *Runner) Probe(ctx context.Context, req types.ProbeMediaRequest) (types.ProbeMediaResponse, error) {
	path, err := r.resolveInOutput(req.Path)
	if err != nil {
		return types.ProbeMediaResponse{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return types.ProbeMediaResponse{}, err
	}
	if info.IsDir() {
		return types.ProbeMediaResponse{}, fmt.Errorf("path %q is a directory", req.Path)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	out, err := r.run(ctx, r.ffprobePath, []string{
		"-v", "error",
		"-show_format",
		"-show_streams",
		"-of", "json",
		path,
	})
	if err != nil {
		return types.ProbeMediaResponse{}, err
	}

	var probe ffprobeOutput
	if err := json.Unmarshal([]byte(out), &probe); err != nil {
		return types.ProbeMediaResponse{}, fmt.Errorf("parse ffprobe output: %w", err)
	}

	resp := types.ProbeMediaResponse{
		Path:          path,
		Filename:      info.Name(),
		FormatName:    probe.Format.FormatName,
		FormatLong:    probe.Format.FormatLongName,
		Duration:      probe.Format.Duration,
		DurationHuman: humanDuration(probe.Format.Duration),
		SizeBytes:     info.Size(),
		BitRate:       probe.Format.BitRate,
		StreamCount:   len(probe.Streams),
		Streams:       make([]types.ProbeStream, 0, len(probe.Streams)),
	}
	for _, s := range probe.Streams {
		stream := types.ProbeStream{
			Index:   s.Index,
			Type:    s.CodecType,
			Codec:   s.CodecName,
			Profile: s.Profile,
		}
		switch s.CodecType {
		case "video":
			stream.Width = s.Width
			stream.Height = s.Height
			stream.FrameRate = simplifyFrameRate(firstNonEmpty(s.AvgFrameRate, s.RFrameRate))
		case "audio":
			stream.Channels = s.Channels
			stream.SampleRate = s.SampleRate
		}
		if s.Tags != nil {
			stream.Language = firstNonEmpty(s.Tags["language"], s.Tags["LANGUAGE"])
			stream.Title = firstNonEmpty(s.Tags["title"], s.Tags["TITLE"])
		}
		resp.Streams = append(resp.Streams, stream)
	}
	return resp, nil
}

// simplifyFrameRate reduces an ffprobe rational frame rate ("30/1",
// "30000/1001") to a compact decimal string ("30", "29.97"). Unparseable or
// zero rates yield an empty string.
func simplifyFrameRate(rational string) string {
	rational = strings.TrimSpace(rational)
	if rational == "" || rational == "0/0" {
		return ""
	}
	num, den, ok := strings.Cut(rational, "/")
	if !ok {
		return rational
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 || n == 0 {
		return ""
	}
	fps := n / d
	// Whole numbers print without a fractional part; others to two decimals.
	if fps == float64(int64(fps)) {
		return strconv.FormatInt(int64(fps), 10)
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(fps, 'f', 2, 64), "0"), ".")
}

// humanDuration converts ffprobe's fractional-seconds string into H:MM:SS (or
// M:SS for sub-hour durations). An unparseable value yields an empty string.
func humanDuration(seconds string) string {
	seconds = strings.TrimSpace(seconds)
	if seconds == "" {
		return ""
	}
	total, err := strconv.ParseFloat(seconds, 64)
	if err != nil || total < 0 {
		return ""
	}
	whole := int64(total)
	h := whole / 3600
	m := (whole % 3600) / 60
	s := whole % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ReadFile returns the contents of a file inside the output directory so a
// caller (for example an agent that only has the MCP channel and no shared
// filesystem) can retrieve a downloaded subtitle or other produced file. Output
// is UTF-8 text when possible, otherwise base64; reads are capped to defend the
// caller's context window.
func (r *Runner) ReadFile(_ context.Context, req types.ReadMediaFileRequest) (types.ReadMediaFileResponse, error) {
	path, err := r.resolveInOutput(req.Path)
	if err != nil {
		return types.ReadMediaFileResponse{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return types.ReadMediaFileResponse{}, err
	}
	if info.IsDir() {
		return types.ReadMediaFileResponse{}, fmt.Errorf("path %q is a directory", req.Path)
	}

	limit := req.MaxBytes
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}

	file, err := os.Open(path)
	if err != nil {
		return types.ReadMediaFileResponse{}, err
	}
	defer func() { _ = file.Close() }()

	// Read one byte past the limit so we can flag truncation without loading the
	// whole file into memory.
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return types.ReadMediaFileResponse{}, err
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}

	resp := types.ReadMediaFileResponse{
		Path:      path,
		Filename:  info.Name(),
		Ext:       strings.TrimPrefix(filepath.Ext(info.Name()), "."),
		SizeBytes: info.Size(),
		Truncated: truncated,
	}
	if utf8.Valid(data) {
		resp.Encoding = "text"
		resp.Content = string(data)
	} else {
		resp.Encoding = "base64"
		resp.Content = base64.StdEncoding.EncodeToString(data)
	}
	return resp, nil
}

// WriteDerived writes derived data (such as a cleaned transcript) next to a
// source file inside the sandbox, naming it from the source's stem plus suffix
// (for example "video.en" + ".clean.txt"). The source path is validated so the
// target always lands inside the output directory.
func (r *Runner) WriteDerived(sourcePath, suffix string, data []byte) (types.MediaFile, error) {
	src, err := r.resolveInOutput(sourcePath)
	if err != nil {
		return types.MediaFile{}, err
	}
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	target := filepath.Join(filepath.Dir(src), stem+suffix)
	if err := os.WriteFile(target, data, 0o640); err != nil {
		return types.MediaFile{}, err
	}
	return r.statFile(target)
}

func (r *Runner) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, r.timeout)
}

// run executes a binary with the given args and returns trimmed stdout. On a
// non-zero exit the trimmed stderr (capped) is included in the error.
func (r *Runner) run(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if r.logger != nil {
		r.logger.Info("media exec", "cmd", filepath.Base(name), "args", len(args))
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s timed out or was cancelled: %w", filepath.Base(name), ctx.Err())
		}
		return "", fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, trimErr(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// resolveInOutput validates that a caller-supplied path lives inside the output
// directory, defending against absolute paths and ".." traversal.
func (r *Runner) resolveInOutput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("path is required")
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(r.outputDir, candidate)
	}
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(r.outputDir, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q is outside the media output directory", raw)
	}
	return candidate, nil
}

// outputPathFor derives a non-clobbering output path for a transcode result.
func (r *Runner) outputPathFor(inputPath, requestedName, ext string) (string, error) {
	if name := strings.TrimSpace(requestedName); name != "" {
		if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			return "", fmt.Errorf("invalid output_name %q: must be a bare filename", requestedName)
		}
		base := name
		if filepath.Ext(base) == "" {
			base += "." + ext
		}
		return filepath.Join(r.outputDir, base), nil
	}
	stem := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	return filepath.Join(r.outputDir, stem+".transcoded."+ext), nil
}

func (r *Runner) statFile(path string) (types.MediaFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return types.MediaFile{}, err
	}
	name := info.Name()
	return types.MediaFile{
		Path:      path,
		Filename:  name,
		Ext:       strings.TrimPrefix(filepath.Ext(name), "."),
		SizeBytes: info.Size(),
	}, nil
}

func defaultAudioCodec(format string) string {
	switch format {
	case "mp3":
		return "libmp3lame"
	case "opus", "ogg", "oga":
		return "libopus"
	case "flac":
		return "flac"
	case "wav":
		return "pcm_s16le"
	default:
		return "aac"
	}
}

// sanitizeExt reduces a requested format to a bare lowercase extension token.
func sanitizeExt(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	format = strings.TrimPrefix(format, ".")
	for _, r := range format {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return ""
		}
	}
	return format
}

// subtitleLanguage extracts the language tag from a yt-dlp subtitle filename
// such as "id.en.srt", falling back to the requested language.
func subtitleLanguage(filename, fallback string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if idx := strings.LastIndex(base, "."); idx >= 0 && idx < len(base)-1 {
		return base[idx+1:]
	}
	return fallback
}

func parsePrintLine(line string) (path, title, duration string) {
	parts := strings.Split(strings.TrimSpace(line), "\t")
	if len(parts) > 0 {
		path = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		title = cleanNA(parts[1])
	}
	if len(parts) > 2 {
		duration = cleanNA(parts[2])
	}
	return path, title, duration
}

func cleanNA(value string) string {
	value = strings.TrimSpace(value)
	if value == "NA" {
		return ""
	}
	return value
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
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
