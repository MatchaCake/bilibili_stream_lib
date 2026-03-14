package stream

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFFmpegReaderCloseReturnsUnexpectedExit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := startTestReader(t, ctx, cancel, "printf boom >&2; exit 1")
	time.Sleep(50 * time.Millisecond)

	err := reader.Close()
	if err == nil {
		t.Fatal("expected process exit error")
	}
	if !strings.Contains(err.Error(), "ffmpeg wait") {
		t.Fatalf("expected wrapped wait error, got %v", err)
	}
}

func TestFFmpegReaderCloseIgnoresCallerCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := startTestReader(t, ctx, cancel, "sleep 5")
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func startTestReader(t *testing.T, ctx context.Context, cancel context.CancelFunc, script string) *ffmpegReader {
	t.Helper()

	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	return &ffmpegReader{
		ReadCloser: stdout,
		cancel:     cancel,
		stderr:     &stderr,
		waitCh:     waitCh,
	}
}
