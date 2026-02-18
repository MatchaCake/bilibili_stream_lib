# CLAUDE.md — bilibili_stream_lib

## Overview
Go library for subscribing to Bilibili live room audio/video streams.
Three-layer design: Monitor (live detection) → API (stream URL) → Capture (ffmpeg PCM).
High-level StreamClient combines all three with pub/sub channel events.

## Module
`github.com/MatchaCake/bilibili_stream_lib`

## Architecture
- `monitor.go` — Room live/offline transition monitor (polling-based)
- `monitor_opts.go` — Monitor options (interval, cookie)
- `api.go` — HTTP API (room info, stream URL, room_init/resolve)
- `capture.go` — ffmpeg audio capture (raw PCM s16le output)
- `client.go` — High-level StreamClient (auto-capture on live)
- `client_opts.go` — Client options (interval, audio config, auto-capture toggle)
- `events.go` — Event types (RoomEvent, StreamEvent, AudioStream)

## Key Design Decisions
- Layered: each component usable independently (Monitor, API, Capture)
- StreamClient composes all three for convenience
- Channel-based pub/sub (same pattern as bilibili_dm_lib)
- Context-driven lifecycle, auto-reconnect on failure
- ffmpeg required on system PATH for audio capture
- Returns raw PCM so consumers pipe to STT, recording, etc.

## Bilibili APIs Used
- `room/v1/Room/room_init` — Resolve short room ID → real room ID
- `room/v1/Room/get_info` — Room info (live status, title, uid)
- `room/v1/Room/playUrl` — Stream URL (FLV)

## Dependencies
- `log/slog` — Logging
- ffmpeg (system binary) — Audio capture

## Build & Test
```bash
go build ./...
go vet ./...
```

## Git
- Author: MatchaCake <MatchaCake@users.noreply.github.com>
- No Co-Authored-By lines
