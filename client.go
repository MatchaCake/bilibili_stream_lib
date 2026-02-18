package stream

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

const (
	streamEventBufSize = 64
	baseRetryDelay     = 2 * time.Second
	maxRetryDelay      = 2 * time.Minute
	maxCaptureRetries  = 5
)

// StreamClient is a high-level client that combines Monitor, stream URL
// fetching, and ffmpeg audio capture into a single pub/sub interface.
//
// When a room goes live, StreamClient automatically fetches the stream URL
// and starts audio capture (if autoCapture is enabled), emitting StreamEvent
// on the subscribed channel.
type StreamClient struct {
	cfg     clientConfig
	monitor *Monitor

	subs   []chan StreamEvent
	subsMu sync.RWMutex

	// Track active captures so we can cancel them on room offline.
	captures   map[int64]context.CancelFunc
	capturesMu sync.Mutex
}

// NewStreamClient creates a StreamClient with the given options.
func NewStreamClient(opts ...ClientOption) *StreamClient {
	cfg := clientConfig{
		interval:    defaultMonitorInterval,
		audioCfg:    DefaultCaptureConfig(),
		autoCapture: true,
	}
	for _, o := range opts {
		o(&cfg)
	}

	monitorOpts := []MonitorOption{
		WithMonitorInterval(cfg.interval),
	}
	if cfg.cookie != "" {
		monitorOpts = append(monitorOpts, WithCookie(cfg.cookie))
	}

	return &StreamClient{
		cfg:      cfg,
		monitor:  NewMonitor(monitorOpts...),
		captures: make(map[int64]context.CancelFunc),
	}
}

// Subscribe begins monitoring the given rooms and returns a channel that
// receives StreamEvent for live/offline transitions, audio readiness, and errors.
// The channel is closed when ctx is cancelled.
func (c *StreamClient) Subscribe(ctx context.Context, roomIDs []int64) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, streamEventBufSize)

	c.subsMu.Lock()
	c.subs = append(c.subs, ch)
	c.subsMu.Unlock()

	roomEvents, err := c.monitor.Watch(ctx, roomIDs)
	if err != nil {
		return nil, err
	}

	// Dispatch goroutine: converts RoomEvents into StreamEvents.
	go c.dispatch(ctx, roomEvents)

	// Cleanup goroutine: close subscriber channels when done.
	go func() {
		<-ctx.Done()
		// Cancel all active captures.
		c.capturesMu.Lock()
		for roomID, cancel := range c.captures {
			cancel()
			delete(c.captures, roomID)
		}
		c.capturesMu.Unlock()

		time.Sleep(100 * time.Millisecond)
		c.subsMu.Lock()
		for _, sub := range c.subs {
			close(sub)
		}
		c.subs = nil
		c.subsMu.Unlock()
	}()

	return ch, nil
}

// AddRoom adds a room to the client. Safe to call after Subscribe().
func (c *StreamClient) AddRoom(roomID int64) {
	c.monitor.AddRoom(roomID)
}

// RemoveRoom stops monitoring a room and cancels any active capture.
func (c *StreamClient) RemoveRoom(roomID int64) {
	c.monitor.RemoveRoom(roomID)

	c.capturesMu.Lock()
	if cancel, ok := c.captures[roomID]; ok {
		cancel()
		delete(c.captures, roomID)
	}
	c.capturesMu.Unlock()
}

// dispatch reads RoomEvents from the monitor and handles them.
func (c *StreamClient) dispatch(ctx context.Context, roomEvents <-chan RoomEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-roomEvents:
			if !ok {
				return
			}
			c.handleRoomEvent(ctx, ev)
		}
	}
}

// handleRoomEvent processes a single RoomEvent.
func (c *StreamClient) handleRoomEvent(ctx context.Context, ev RoomEvent) {
	if ev.Live {
		c.publishStreamEvent(StreamEvent{
			RoomID: ev.RoomID,
			Type:   EventLive,
			Title:  ev.Title,
		})

		if c.cfg.autoCapture {
			go c.startCapture(ctx, ev.RoomID, ev.Title)
		}
	} else {
		// Cancel any active capture for this room.
		c.capturesMu.Lock()
		if cancel, ok := c.captures[ev.RoomID]; ok {
			cancel()
			delete(c.captures, ev.RoomID)
		}
		c.capturesMu.Unlock()

		c.publishStreamEvent(StreamEvent{
			RoomID: ev.RoomID,
			Type:   EventOffline,
			Title:  ev.Title,
		})
	}
}

// startCapture fetches the stream URL and starts ffmpeg audio capture,
// retrying on failure with exponential backoff.
func (c *StreamClient) startCapture(ctx context.Context, roomID int64, title string) {
	captureCtx, cancel := context.WithCancel(ctx)

	c.capturesMu.Lock()
	// Cancel previous capture if any.
	if prevCancel, ok := c.captures[roomID]; ok {
		prevCancel()
	}
	c.captures[roomID] = cancel
	c.capturesMu.Unlock()

	for attempt := 0; attempt < maxCaptureRetries; attempt++ {
		if captureCtx.Err() != nil {
			return
		}

		streamURL, err := GetStreamURL(captureCtx, roomID)
		if err != nil {
			slog.Warn("client: failed to get stream URL",
				"room_id", roomID, "attempt", attempt+1, "error", err)
			c.publishStreamEvent(StreamEvent{
				RoomID: roomID,
				Type:   EventError,
				Error:  err,
				Title:  title,
			})
			if !c.retryWait(captureCtx, attempt) {
				return
			}
			continue
		}

		reader, err := CaptureAudio(captureCtx, streamURL, &c.cfg.audioCfg)
		if err != nil {
			slog.Warn("client: failed to start capture",
				"room_id", roomID, "attempt", attempt+1, "error", err)
			c.publishStreamEvent(StreamEvent{
				RoomID: roomID,
				Type:   EventError,
				Error:  err,
				Title:  title,
			})
			if !c.retryWait(captureCtx, attempt) {
				return
			}
			continue
		}

		slog.Info("client: audio capture started", "room_id", roomID)
		c.publishStreamEvent(StreamEvent{
			RoomID: roomID,
			Type:   EventAudioReady,
			Audio: &AudioStream{
				RoomID: roomID,
				Reader: reader,
				Cancel: cancel,
			},
			Title: title,
		})
		return
	}

	slog.Error("client: exhausted capture retries", "room_id", roomID)
}

// retryWait waits with exponential backoff. Returns false if the context
// was cancelled during the wait.
func (c *StreamClient) retryWait(ctx context.Context, attempt int) bool {
	delay := time.Duration(float64(baseRetryDelay) * math.Pow(2, float64(attempt)))
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

// publishStreamEvent fans out a StreamEvent to all subscriber channels.
func (c *StreamClient) publishStreamEvent(ev StreamEvent) {
	c.subsMu.RLock()
	defer c.subsMu.RUnlock()
	for _, ch := range c.subs {
		select {
		case ch <- ev:
		default:
			slog.Warn("client: subscriber channel full, dropping event",
				"room_id", ev.RoomID, "type", ev.Type)
		}
	}
}
