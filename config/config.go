package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ReplicaConfig struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type ShardConfig struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type NodeConfig struct {
	Shard   string            `yaml:"shard"`
	Replica string            `yaml:"replica"`
	Address string            `yaml:"address"`
	Labels  map[string]string `yaml:"labels"`
}

// Config holds the simplified proxy configuration
type Config struct {
	ListenAddr    string        `yaml:"listen_addr"`
	HeaderName    string        `yaml:"header_name"`    // Header for grouping (e.g., "X-User-Id")
	MaxConcurrent int           `yaml:"max_concurrent"` // Limit per header value
	MaxQueue      int           `yaml:"max_queue"`      // Queue size per header value
	QueueTimeout  time.Duration `yaml:"queue_timeout"`  // Max time to wait in queue
	ReplicaScheme string        `yaml:"replica_scheme"` // "http" or "https"

	Shards   []ShardConfig   `yaml:"shards"`
	Replicas []ReplicaConfig `yaml:"replicas"`
	Nodes    []NodeConfig    `yaml:"nodes"`

	SlowdownError string  `yaml:"slowdown_error"` // Substring in CH error to trigger slowdown (e.g., "Too many simultaneous queries")
	SlowdownCode  int     `yaml:"slowdown_code"`  // Optional: HTTP status code for slowdown error (e.g., 503, 500)
	SlowdownRate  float64 `yaml:"slowdown_rate"`  // Target req/sec when slowed down
	SlowdownBurst int     `yaml:"slowdown_burst"` // Burst allowance when slowed down

	ProxyTimeout time.Duration `yaml:"proxy_timeout"` // Timeout for requests to backend replicas
	UserAgent    string        `yaml:"user_agent"`    // Custom User-Agent for backend requests

	Version string `yaml:"version"` // Version of the config file
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
