// --- replica.go ---
package main

import (
	"context"
	"log"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Replica represents a backend ClickHouse node with its own rate limiter
type Replica struct {
	URL          *url.URL
	limiter      *rate.Limiter
	mu           sync.Mutex // Protects limiter state changes
	isSlowedDown bool
	slowRate     rate.Limit
	slowBurst    int
	// Add health status if needed later
}

func NewReplica(addr string, scheme string, slowRate float64, slowBurst int) (*Replica, error) {
	if scheme == "" {
		scheme = "http"
	}
	fullAddr := scheme + "://" + addr
	parsedURL, err := url.Parse(fullAddr)
	if err != nil {
		return nil, err
	}
	// Start with no rate limit (Infinite rate, burst of 1 is effectively no limit)
	limiter := rate.NewLimiter(rate.Inf, 1)

	return &Replica{
		URL:       parsedURL,
		limiter:   limiter,
		slowRate:  rate.Limit(slowRate),
		slowBurst: slowBurst,
	}, nil
}

// Wait respects the replica's current rate limit.
func (r *Replica) Wait(ctx context.Context) error {
	return r.limiter.Wait(ctx)
}

// SlowDown reduces the rate limit for this replica.
func (r *Replica) SlowDown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Avoid repeatedly setting the limit if already slowed down
	if !r.isSlowedDown {
		log.Printf("Slowing down replica %s to %v req/sec", r.URL.Host, r.slowRate)
		r.limiter.SetLimit(r.slowRate)
		r.limiter.SetBurst(r.slowBurst)
		r.isSlowedDown = true
		// Optional: Start a timer/goroutine here to attempt recovery later
	}
}

// Optional: Add SpeedUp() method for recovery logic
// func (r *Replica) SpeedUp() { ... }
