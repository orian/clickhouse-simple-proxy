package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	cfg, err := LoadConfig("config.yml")
	require.NoError(t, err)

	want := &Config{
		ListenAddr:    ":18123",
		HeaderName:    "X-User-Id",
		MaxConcurrent: 3,
		MaxQueue:      10,
		QueueTimeout:  60 * time.Second,
		ReplicaScheme: "http",
		Shards: []ShardConfig{
			{
				Name: "1",
				Labels: map[string]string{
					"single": "true",
				},
			},
		},
		Replicas: []ReplicaConfig{
			{
				Name: "primary",
				Labels: map[string]string{
					"workload": "online",
				},
			},
			{
				Name: "secondary",
				Labels: map[string]string{
					"workload": "offline",
				},
			},
		},
		Nodes: []NodeConfig{
			{
				Shard:   "1",
				Replica: "primary",
				Address: "10.5.0.2:8123",
			},
			{
				Shard:   "1",
				Replica: "secondary",
				Address: "10.5.0.3:8123",
			},
		},
		SlowdownError: "Too many simultaneous queries",
		SlowdownCode:  503,
		SlowdownRate:  1,
		SlowdownBurst: 1,
		ProxyTimeout:  120 * time.Second,
		UserAgent:     "SimpleClickHouseProxy/1.0",
		Version:       "1.0",
	}
	require.Equal(t, want, cfg)
}
