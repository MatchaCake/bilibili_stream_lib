package stream

import "time"

// clientConfig holds internal configuration for StreamClient.
type clientConfig struct {
	interval    time.Duration
	cookie      string
	audioCfg    CaptureConfig
	autoCapture bool
}

// ClientOption configures a StreamClient.
type ClientOption func(*clientConfig)

// WithInterval sets the polling interval for live status monitoring.
// Default is 30 seconds.
func WithInterval(d time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.interval = d
	}
}

// WithClientCookie sets the SESSDATA cookie for authenticated API requests.
func WithClientCookie(sessdata string) ClientOption {
	return func(c *clientConfig) {
		c.cookie = sessdata
	}
}

// WithAudioConfig sets the audio capture parameters (sample rate, channels, format).
func WithAudioConfig(cfg CaptureConfig) ClientOption {
	return func(c *clientConfig) {
		c.audioCfg = cfg
	}
}

// WithAutoCapture controls whether audio capture starts automatically when
// a room goes live. Default is true.
func WithAutoCapture(enabled bool) ClientOption {
	return func(c *clientConfig) {
		c.autoCapture = enabled
	}
}
