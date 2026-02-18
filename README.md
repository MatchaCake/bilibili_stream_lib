# bilibili_stream_lib

Go library for subscribing to Bilibili live room audio/video streams. Provides a channel-based pub/sub interface following the same design pattern as [bilibili_dm_lib](https://github.com/MatchaCake/bilibili_dm_lib).

## Features

- **Monitor** live rooms for live/offline transitions via polling
- **Fetch** stream URLs (FLV/HLS) from Bilibili's API
- **Capture** audio from live streams as raw PCM via ffmpeg
- **StreamClient** combines all three with auto-capture on live events

## Requirements

- Go 1.22+
- [ffmpeg](https://ffmpeg.org/) installed and available in `PATH` (for audio capture)

## Install

```bash
go get github.com/MatchaCake/bilibili_stream_lib
```

## Architecture

The library has three independent layers that can be used separately or combined:

```
StreamClient (high-level, auto-capture)
    ├── Monitor (live/offline detection)
    ├── API (stream URL, room info)
    └── Capture (ffmpeg PCM extraction)
```

## Usage

### Layer 1: API (standalone functions)

```go
import stream "github.com/MatchaCake/bilibili_stream_lib"

// Resolve short room ID to real room ID
realID, err := stream.ResolveRoomID(ctx, 123)

// Get room metadata
info, err := stream.GetRoomInfo(ctx, realID)
fmt.Println(info.Title, info.LiveStatus)

// Get stream URL (only works when live)
url, err := stream.GetStreamURL(ctx, realID)
```

### Layer 2: Monitor (live/offline events)

```go
m := stream.NewMonitor(
    stream.WithMonitorInterval(15 * time.Second),
)

events, err := m.Watch(ctx, []int64{12345, 67890})
if err != nil {
    log.Fatal(err)
}

for ev := range events {
    if ev.Live {
        fmt.Printf("Room %d went live: %s\n", ev.RoomID, ev.Title)
    } else {
        fmt.Printf("Room %d went offline\n", ev.RoomID)
    }
}
```

Dynamic room management:

```go
m.AddRoom(99999)    // start watching
m.RemoveRoom(12345) // stop watching
```

### Layer 3: Capture (ffmpeg audio)

```go
url, _ := stream.GetStreamURL(ctx, roomID)

reader, err := stream.CaptureAudio(ctx, url, &stream.CaptureConfig{
    SampleRate: 16000,
    Channels:   1,
    Format:     "s16le",
})
if err != nil {
    log.Fatal(err)
}
defer reader.Close()

// Read raw PCM data
buf := make([]byte, 4096)
for {
    n, err := reader.Read(buf)
    if err != nil {
        break
    }
    // Process buf[:n] - pipe to STT, save to file, etc.
}
```

### Full: StreamClient (auto-capture)

```go
client := stream.NewStreamClient(
    stream.WithInterval(15 * time.Second),
    stream.WithAudioConfig(stream.CaptureConfig{
        SampleRate: 16000,
        Channels:   1,
        Format:     "s16le",
    }),
    stream.WithAutoCapture(true),
)

events, err := client.Subscribe(ctx, []int64{12345, 67890})
if err != nil {
    log.Fatal(err)
}

for ev := range events {
    switch ev.Type {
    case stream.EventLive:
        fmt.Printf("Room %d is live: %s\n", ev.RoomID, ev.Title)
    case stream.EventOffline:
        fmt.Printf("Room %d went offline\n", ev.RoomID)
    case stream.EventAudioReady:
        // ev.Audio.Reader is an io.ReadCloser with raw PCM data
        go processAudio(ev.Audio)
    case stream.EventError:
        fmt.Printf("Error for room %d: %v\n", ev.RoomID, ev.Error)
    }
}
```

Dynamic room management works the same way:

```go
client.AddRoom(99999)
client.RemoveRoom(12345)
```

## Event Types

### RoomEvent (from Monitor)

| Field  | Type   | Description                    |
|--------|--------|--------------------------------|
| RoomID | int64  | Bilibili room ID               |
| Live   | bool   | true=went live, false=offline   |
| Title  | string | Room title (when going live)    |

### StreamEvent (from StreamClient)

| Field  | Type          | Description                          |
|--------|---------------|--------------------------------------|
| RoomID | int64         | Bilibili room ID                     |
| Type   | string        | "live", "offline", "audio_ready", "error" |
| Audio  | *AudioStream  | Non-nil for "audio_ready"            |
| Error  | error         | Non-nil for "error"                  |
| Title  | string        | Room title                           |

## Audio Format

By default, audio is captured as:
- **Format**: signed 16-bit little-endian PCM (`s16le`)
- **Sample rate**: 16000 Hz
- **Channels**: 1 (mono)

This is ideal for speech-to-text pipelines. Customize via `CaptureConfig`.

## License

MIT License - see [LICENSE](LICENSE)
