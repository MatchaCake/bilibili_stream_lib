package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"
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
		"-probesize", "500000", // 500KB (default 5MB)
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

	captureCtx, captureCancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(captureCtx, "ffmpeg", args...)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		captureCancel()
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		captureCancel()
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	slog.Info("capture: ffmpeg started", "stream_url_prefix", truncateURL(streamURL))

	return &ffmpegReader{
		ReadCloser: stdout,
		cancel:     captureCancel,
		stderr:     &stderrBuf,
		waitCh:     waitCh,
	}, nil
}

// ffmpegReader wraps the stdout pipe and ensures the ffmpeg process is
// cleaned up when Close is called.
type ffmpegReader struct {
	io.ReadCloser
	cancel    context.CancelFunc
	stderr    *bytes.Buffer
	waitCh    <-chan error
	closeOnce sync.Once
	closeErr  error
}

func (f *ffmpegReader) Close() error {
	f.closeOnce.Do(func() {
		pipeErr := f.ReadCloser.Close()
		waitErr, closeRequested := f.waitForProcess()

		if waitErr != nil && !closeRequested && f.stderr.Len() > 0 {
			slog.Error("capture: ffmpeg exited with error", "stderr", f.stderr.String())
		}
		if waitErr != nil && !closeRequested {
			f.closeErr = fmt.Errorf("ffmpeg wait: %w", waitErr)
			return
		}
		if pipeErr != nil && !errors.Is(pipeErr, os.ErrClosed) && !errors.Is(pipeErr, io.ErrClosedPipe) {
			f.closeErr = pipeErr
		}
	})
	return f.closeErr
}

func (f *ffmpegReader) waitForProcess() (error, bool) {
	select {
	case err := <-f.waitCh:
		return err, false
	default:
	}

	f.cancel()
	return <-f.waitCh, true
}

// truncateURL returns the first 80 characters of a URL for logging.
func truncateURL(u string) string {
	if len(u) <= 80 {
		return u
	}
	return u[:80] + "..."
}
