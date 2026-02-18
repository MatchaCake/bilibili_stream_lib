package stream

import (
	"context"
	"io"
)

// RoomEvent represents a live/offline transition detected by Monitor.
type RoomEvent struct {
	RoomID int64
	Live   bool   // true = went live, false = went offline
	Title  string // room title (populated when going live)
}

// RoomInfo holds metadata about a Bilibili live room.
type RoomInfo struct {
	RoomID     int64
	ShortID    int64
	UID        int64
	LiveStatus int // 0=offline, 1=live, 2=rotation
	Title      string
	LiveTime   string
}

// CaptureConfig controls ffmpeg audio capture parameters.
type CaptureConfig struct {
	SampleRate int    // default 16000
	Channels   int    // default 1 (mono)
	Format     string // default "s16le"
}

// DefaultCaptureConfig returns a CaptureConfig with sensible defaults
// for speech processing: 16kHz mono signed 16-bit little-endian PCM.
func DefaultCaptureConfig() CaptureConfig {
	return CaptureConfig{
		SampleRate: 16000,
		Channels:   1,
		Format:     "s16le",
	}
}

// AudioStream represents an active audio capture from a live stream.
// Reader delivers raw PCM data according to the CaptureConfig used.
// Call Cancel to stop the ffmpeg process and release resources.
type AudioStream struct {
	RoomID int64
	Reader io.ReadCloser
	Cancel context.CancelFunc
}

// StreamEvent is emitted by StreamClient to report room state changes
// and audio capture lifecycle events.
type StreamEvent struct {
	RoomID int64
	Type   string       // "live", "offline", "audio_ready", "error"
	Audio  *AudioStream // non-nil when Type == "audio_ready"
	Error  error        // non-nil when Type == "error"
	Title  string
}

// Event type constants for StreamEvent.Type.
const (
	EventLive       = "live"
	EventOffline    = "offline"
	EventAudioReady = "audio_ready"
	EventError      = "error"
)
