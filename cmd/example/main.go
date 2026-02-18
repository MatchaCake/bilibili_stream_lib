// Example demonstrates the three layers of bilibili_stream_lib:
// 1. API layer (standalone functions)
// 2. Monitor (live/offline event channel)
// 3. StreamClient (auto-capture with pub/sub events)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"

	stream "github.com/MatchaCake/bilibili_stream_lib"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <room_id> [room_id...]\n", os.Args[0])
		os.Exit(1)
	}

	var roomIDs []int64
	for _, arg := range os.Args[1:] {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid room ID %q: %v\n", arg, err)
			os.Exit(1)
		}
		roomIDs = append(roomIDs, id)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// --- Layer 1: API (standalone functions) ---
	slog.Info("=== API Layer Demo ===")
	for _, id := range roomIDs {
		realID, err := stream.ResolveRoomID(ctx, id)
		if err != nil {
			slog.Error("resolve room id", "id", id, "error", err)
			continue
		}
		slog.Info("resolved room", "input", id, "real_id", realID)

		info, err := stream.GetRoomInfo(ctx, realID)
		if err != nil {
			slog.Error("get room info", "room_id", realID, "error", err)
			continue
		}
		slog.Info("room info",
			"room_id", info.RoomID,
			"title", info.Title,
			"live_status", info.LiveStatus,
			"uid", info.UID,
		)
	}

	// --- Layer 3: StreamClient (full auto-capture) ---
	slog.Info("=== StreamClient Demo ===")

	client := stream.NewStreamClient(
		stream.WithInterval(15*time.Second),
		stream.WithAudioConfig(stream.CaptureConfig{
			SampleRate: 16000,
			Channels:   1,
			Format:     "s16le",
		}),
		stream.WithAutoCapture(true),
	)

	events, err := client.Subscribe(ctx, roomIDs)
	if err != nil {
		slog.Error("subscribe failed", "error", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for events... (Ctrl+C to stop)")

	for ev := range events {
		switch ev.Type {
		case stream.EventLive:
			slog.Info("LIVE", "room_id", ev.RoomID, "title", ev.Title)

		case stream.EventOffline:
			slog.Info("OFFLINE", "room_id", ev.RoomID)

		case stream.EventAudioReady:
			slog.Info("AUDIO READY", "room_id", ev.RoomID)
			// Read a small sample of audio data.
			go func(audio *stream.AudioStream) {
				defer audio.Reader.Close()
				buf := make([]byte, 4096)
				n, err := audio.Reader.Read(buf)
				if err != nil {
					slog.Error("read audio", "room_id", audio.RoomID, "error", err)
					return
				}
				slog.Info("received audio data",
					"room_id", audio.RoomID,
					"bytes", n,
				)
				// In real usage, you'd continuously read and pipe to STT, etc.
			}(ev.Audio)

		case stream.EventError:
			slog.Error("ERROR", "room_id", ev.RoomID, "error", ev.Error)
		}
	}
}
