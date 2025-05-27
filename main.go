// --- main.go --- (Entry point)
package main

import (
	"clickhouse-test/config"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3" // Example using YAML, adjust as needed
)

func main() {
	configPath := flag.String("config", "config.yml", "Path to config file")
	flag.Parse()
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Error reading config file %s: %v", *configPath, err)
	}

	// Set defaults
	cfg := config.Config{
		ReplicaScheme: "http",
		QueueTimeout:  10 * time.Second,
		SlowdownRate:  1.0,
		SlowdownBurst: 1,
	}

	err = yaml.Unmarshal(configData, &cfg)
	if err != nil {
		log.Fatalf("Error parsing config file %s: %v", *configPath, err)
	}

	// --- Validate Config ---
	if cfg.ListenAddr == "" || cfg.HeaderName == "" || len(cfg.Replicas) == 0 {
		log.Fatalf("listen_addr, header_name, and replicas are required in config")
	}
	if cfg.SlowdownError == "" && cfg.SlowdownCode == 0 {
		log.Printf("Warning: Neither slowdown_error nor slowdown_code is set. Replica slowdown will not be triggered.")
	}
	log.Printf("Config loaded: Listen=%s Header=%s Concurrent=%d Queue=%d Replicas=%d",
		cfg.ListenAddr, cfg.HeaderName, cfg.MaxConcurrent, cfg.MaxQueue, len(cfg.Replicas))

	// --- Setup Proxy ---
	proxy, err := NewSimpleProxy(&cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// --- Start Server ---
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      proxy,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 180 * time.Second, // Allow time for long queries
		IdleTimeout:  200 * time.Second,
	}

	log.Printf("Starting simple ClickHouse proxy on %s...", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server failed: %v", err)
	}
}
