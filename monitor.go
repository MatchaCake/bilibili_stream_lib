package stream

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultMonitorInterval = 30 * time.Second
	eventBufSize           = 64
)

// Monitor watches Bilibili live rooms for live/offline transitions
// and emits RoomEvent on a channel when a room's status changes.
type Monitor struct {
	cfg monitorConfig

	mu     sync.Mutex
	rooms  map[int64]context.CancelFunc // roomID -> cancel
	status map[int64]bool               // roomID -> last known live status

	subs   []chan RoomEvent
	subsMu sync.RWMutex

	parentCtx context.Context
	started   bool
}

// NewMonitor creates a Monitor with the given options.
func NewMonitor(opts ...MonitorOption) *Monitor {
	cfg := monitorConfig{
		interval: defaultMonitorInterval,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &Monitor{
		cfg:    cfg,
		rooms:  make(map[int64]context.CancelFunc),
		status: make(map[int64]bool),
	}
}

// Watch begins monitoring the given rooms and returns a channel that
// receives RoomEvent whenever a room transitions between live and offline.
// The channel is closed when ctx is cancelled.
func (m *Monitor) Watch(ctx context.Context, roomIDs []int64) (<-chan RoomEvent, error) {
	ch := make(chan RoomEvent, eventBufSize)

	m.subsMu.Lock()
	m.subs = append(m.subs, ch)
	m.subsMu.Unlock()

	m.parentCtx = ctx
	m.started = true

	for _, id := range roomIDs {
		m.startRoom(ctx, id)
	}

	// Close subscriber channels when context is done.
	go func() {
		<-ctx.Done()
		// Wait a moment for final events to flush.
		time.Sleep(100 * time.Millisecond)
		m.subsMu.Lock()
		for _, sub := range m.subs {
			close(sub)
		}
		m.subs = nil
		m.subsMu.Unlock()
	}()

	return ch, nil
}

// AddRoom adds a room to the monitor. Safe to call after Watch().
func (m *Monitor) AddRoom(roomID int64) {
	m.mu.Lock()
	if _, exists := m.rooms[roomID]; exists {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	if m.started && m.parentCtx != nil {
		m.startRoom(m.parentCtx, roomID)
	}
}

// RemoveRoom stops monitoring a room.
func (m *Monitor) RemoveRoom(roomID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.rooms[roomID]; ok {
		cancel()
		delete(m.rooms, roomID)
		delete(m.status, roomID)
	}
}

// startRoom launches a polling goroutine for a single room.
func (m *Monitor) startRoom(ctx context.Context, roomID int64) {
	roomCtx, cancel := context.WithCancel(ctx)

	m.mu.Lock()
	m.rooms[roomID] = cancel
	m.mu.Unlock()

	go m.pollRoom(roomCtx, roomID)
}

// pollRoom periodically checks a room's live status and emits events on transitions.
func (m *Monitor) pollRoom(ctx context.Context, roomID int64) {
	slog.Info("monitor: watching room", "room_id", roomID)

	// Do an initial check immediately.
	m.checkRoom(ctx, roomID)

	ticker := time.NewTicker(m.cfg.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitor: stopped watching room", "room_id", roomID)
			return
		case <-ticker.C:
			m.checkRoom(ctx, roomID)
		}
	}
}

// checkRoom queries room info and emits an event if the live status changed.
func (m *Monitor) checkRoom(ctx context.Context, roomID int64) {
	info, err := GetRoomInfo(ctx, roomID)
	if err != nil {
		if ctx.Err() != nil {
			return // context cancelled, not a real error
		}
		slog.Warn("monitor: failed to get room info", "room_id", roomID, "error", err)
		return
	}

	live := info.LiveStatus == 1

	m.mu.Lock()
	prevLive, known := m.status[roomID]
	m.status[roomID] = live
	m.mu.Unlock()

	// Only emit on transitions, not on initial check (unless room is already live).
	if known && live == prevLive {
		return
	}

	if !known && !live {
		// First check shows offline — don't emit an event.
		return
	}

	ev := RoomEvent{
		RoomID: roomID,
		Live:   live,
		Title:  info.Title,
	}

	if live {
		slog.Info("monitor: room went live", "room_id", roomID, "title", info.Title)
	} else {
		slog.Info("monitor: room went offline", "room_id", roomID)
	}

	m.publishEvent(ev)
}

// publishEvent fans out an event to all subscriber channels.
// Uses non-blocking send to prevent slow consumers from stalling the monitor.
func (m *Monitor) publishEvent(ev RoomEvent) {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	for _, ch := range m.subs {
		select {
		case ch <- ev:
		default:
			slog.Warn("monitor: subscriber channel full, dropping event",
				"room_id", ev.RoomID)
		}
	}
}
