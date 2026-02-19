package stream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
)

// CaptureAudio starts an ffmpeg process that reads from streamURL and outputs
// raw PCM audio to the returned ReadCloser. The caller must close the reader
// or cancel the context to stop ffmpeg and release resources.
//
// ffmpeg must be installed and available in the system PATH.
func CaptureAudio(ctx context.Context, streamURL string, cfg *CaptureConfig) (io.ReadCloser, error) {
	if cfg == nil {
		d := DefaultCaptureConfig()
		cfg = &d
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		// Low-latency input: minimize buffering for live streams.
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-analyzeduration", "500000", // 0.5s (default 5s)
		"-probesize", "500000",       // 500KB (default 5MB)
		// Input: HTTP stream with required headers.
		"-user_agent", userAgent,
		"-headers", "Referer: " + referer + "\r\n",
		"-i", streamURL,
		// Output: raw PCM audio to stdout.
		"-vn",
		"-acodec", fmt.Sprintf("pcm_%s", cfg.Format),
		"-ar", strconv.Itoa(cfg.SampleRate),
		"-ac", strconv.Itoa(cfg.Channels),
		"-f", cfg.Format,
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	slog.Info("capture: ffmpeg started", "stream_url_prefix", truncateURL(streamURL))

	return &ffmpegReader{
		ReadCloser: stdout,
		cmd:        cmd,
		ctx:        ctx,
		stderr:     &stderrBuf,
	}, nil
}

// ffmpegReader wraps the stdout pipe and ensures the ffmpeg process is
// cleaned up when Close is called.
type ffmpegReader struct {
	io.ReadCloser
	cmd    *exec.Cmd
	ctx    context.Context
	stderr *bytes.Buffer
}

func (f *ffmpegReader) Close() error {
	// Close the stdout pipe first.
	pipeErr := f.ReadCloser.Close()

	// Wait for the process to exit (may already be dead from context cancel).
	waitErr := f.cmd.Wait()

	// Log stderr if ffmpeg exited with error (not from context cancel).
	if waitErr != nil && f.ctx.Err() == nil && f.stderr.Len() > 0 {
		slog.Error("capture: ffmpeg exited with error", "stderr", f.stderr.String())
	}

	if pipeErr != nil {
		return pipeErr
	}
	if waitErr != nil && f.ctx.Err() != nil {
		return nil
	}
	return waitErr
}

// truncateURL returns the first 80 characters of a URL for logging.
func truncateURL(u string) string {
	if len(u) <= 80 {
		return u
	}
	return u[:80] + "..."
}
