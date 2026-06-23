// Package media wraps yt-dlp and ffmpeg to download videos, transcode/compress
// media, and pull subtitles. All output is confined to a configured directory
// and every external process is invoked with an explicit argument vector (never
// a shell) so caller-supplied values cannot inject commands.
package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/internal/config"
	"github.com/regiellis/mcp-searxng-go/pkg/types"
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
	outputDir  string
	ytDlpPath  string
	ffmpegPath string
	timeout    time.Duration
	guard      URLGuard
	logger     *slog.Logger
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
		outputDir:  outputDir,
		ytDlpPath:  firstNonEmpty(cfg.YtDlpPath, "yt-dlp"),
		ffmpegPath: firstNonEmpty(cfg.FfmpegPath, "ffmpeg"),
		timeout:    cfg.Timeout,
		guard:      guard,
		logger:     logger,
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
	return errors.Join(errs...)
}

// OutputDir returns the sandbox root for produced files.
func (r *Runner) OutputDir() string { return r.outputDir }

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
