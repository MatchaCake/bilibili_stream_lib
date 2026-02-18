package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	referer   = "https://live.bilibili.com/"

	roomInitURL  = "https://api.live.bilibili.com/room/v1/Room/room_init?id=%d"
	roomInfoURL  = "https://api.live.bilibili.com/room/v1/Room/get_info?room_id=%d"
	playURL      = "https://api.live.bilibili.com/room/v1/Room/playUrl?cid=%d&quality=4&platform=web"
)

// apiResponse is the common envelope for Bilibili API responses.
type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// doGet performs an authenticated GET request and decodes the API envelope.
func doGet(ctx context.Context, url string, cookie string) (*apiResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	if cookie != "" {
		req.Header.Set("Cookie", "SESSDATA="+cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("api error %d: %s", apiResp.Code, apiResp.Message)
	}
	return &apiResp, nil
}

// ResolveRoomID converts a short room ID to the real (long) room ID.
// If the ID is already a real room ID, Bilibili returns it unchanged.
func ResolveRoomID(ctx context.Context, shortID int64) (int64, error) {
	apiResp, err := doGet(ctx, fmt.Sprintf(roomInitURL, shortID), "")
	if err != nil {
		return 0, fmt.Errorf("resolve room id: %w", err)
	}

	var data struct {
		RoomID int64 `json:"room_id"`
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return 0, fmt.Errorf("parse room_init: %w", err)
	}
	return data.RoomID, nil
}

// GetRoomInfo fetches metadata for a live room.
func GetRoomInfo(ctx context.Context, roomID int64) (*RoomInfo, error) {
	apiResp, err := doGet(ctx, fmt.Sprintf(roomInfoURL, roomID), "")
	if err != nil {
		return nil, fmt.Errorf("get room info: %w", err)
	}

	var data struct {
		RoomID     int64  `json:"room_id"`
		ShortID    int64  `json:"short_id"`
		UID        int64  `json:"uid"`
		LiveStatus int    `json:"live_status"`
		Title      string `json:"title"`
		LiveTime   string `json:"live_time"`
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("parse room info: %w", err)
	}

	return &RoomInfo{
		RoomID:     data.RoomID,
		ShortID:    data.ShortID,
		UID:        data.UID,
		LiveStatus: data.LiveStatus,
		Title:      data.Title,
		LiveTime:   data.LiveTime,
	}, nil
}

// GetStreamURL fetches the FLV stream URL for a live room.
// Returns an error if the room is not currently live.
func GetStreamURL(ctx context.Context, roomID int64) (string, error) {
	apiResp, err := doGet(ctx, fmt.Sprintf(playURL, roomID), "")
	if err != nil {
		return "", fmt.Errorf("get stream url: %w", err)
	}

	var data struct {
		Durl []struct {
			URL string `json:"url"`
		} `json:"durl"`
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return "", fmt.Errorf("parse play url: %w", err)
	}
	if len(data.Durl) == 0 {
		return "", fmt.Errorf("no stream urls returned (room may be offline)")
	}
	return data.Durl[0].URL, nil
}
