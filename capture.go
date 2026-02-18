package stream

import (
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
		// Input: HTTP stream with required headers.
		"-user_agent", userAgent,
		"-headers", "Referer: " + referer + "\r\n",
		"-i", streamURL,
		// Output: raw PCM audio to stdout.
		"-vn",                                   // no video
		"-acodec", fmt.Sprintf("pcm_%s", cfg.Format), // e.g. pcm_s16le
		"-ar", strconv.Itoa(cfg.SampleRate),
		"-ac", strconv.Itoa(cfg.Channels),
		"-f", cfg.Format,
		"pipe:1", // output to stdout
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	slog.Info("capture: ffmpeg started", "stream_url_prefix", truncateURL(streamURL))

	// Wrap in a ReadCloser that also waits for the process to exit.
	return &ffmpegReader{
		ReadCloser: stdout,
		cmd:        cmd,
	}, nil
}

// ffmpegReader wraps the stdout pipe and ensures the ffmpeg process is
// cleaned up when Close is called.
type ffmpegReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (f *ffmpegReader) Close() error {
	// Close the stdout pipe first.
	pipeErr := f.ReadCloser.Close()

	// Wait for the process to exit (may already be dead from context cancel).
	waitErr := f.cmd.Wait()

	// Prefer the pipe error if both fail.
	if pipeErr != nil {
		return pipeErr
	}
	// Ignore exit errors caused by context cancellation (signal: killed).
	if waitErr != nil && f.cmd.ProcessState != nil && !f.cmd.ProcessState.Exited() {
		return nil // killed by context cancel
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
