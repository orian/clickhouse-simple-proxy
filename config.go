// --- config.go ---
package main

import (
	"time"
)

// Config holds the simplified proxy configuration
type Config struct {
	ListenAddr    string        `yaml:"listen_addr"`
	HeaderName    string        `yaml:"header_name"`    // Header for grouping (e.g., "X-User-Id")
	MaxConcurrent int           `yaml:"max_concurrent"` // Limit per header value
	MaxQueue      int           `yaml:"max_queue"`      // Queue size per header value
	QueueTimeout  time.Duration `yaml:"queue_timeout"`  // Max time to wait in queue
	Replicas      []string      `yaml:"replicas"`       // ClickHouse nodes ("host:port")
	ReplicaScheme string        `yaml:"replica_scheme"` // "http" or "https"

	SlowdownError string  `yaml:"slowdown_error"` // Substring in CH error to trigger slowdown (e.g., "Too many simultaneous queries")
	SlowdownCode  int     `yaml:"slowdown_code"`  // Optional: HTTP status code for slowdown error (e.g., 503, 500)
	SlowdownRate  float64 `yaml:"slowdown_rate"`  // Target req/sec when slowed down
	SlowdownBurst int     `yaml:"slowdown_burst"` // Burst allowance when slowed down

	ProxyTimeout time.Duration `yaml:"proxy_timeout"` // Timeout for requests to backend replicas
	UserAgent    string        `yaml:"user_agent"`    // Custom User-Agent for backend requests
}
