package stream

import "time"

// monitorConfig holds internal configuration for Monitor.
type monitorConfig struct {
	interval time.Duration
	cookie   string
}

// MonitorOption configures a Monitor.
type MonitorOption func(*monitorConfig)

// WithMonitorInterval sets the polling interval for live status checks.
// Default is 30 seconds.
func WithMonitorInterval(d time.Duration) MonitorOption {
	return func(c *monitorConfig) {
		c.interval = d
	}
}

// WithCookie sets the SESSDATA cookie for authenticated API requests.
// This is optional; most API endpoints work without authentication.
func WithCookie(sessdata string) MonitorOption {
	return func(c *monitorConfig) {
		c.cookie = sessdata
	}
}
