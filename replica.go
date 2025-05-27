// --- replica.go ---
package main

import (
	"clickhouse-test/config"
	"context"
	"log"
	"net/url"
	"sync"
	"sync/atomic"

	"golang.org/x/time/rate"
)

type Node struct {
	URL     *url.URL
	Address string
	Replica *Replica
}

// Replica represents a backend ClickHouse node with its own rate limiter
type Replica struct {
	Name         string
	Nodes        []*Node
	limiter      *rate.Limiter
	mu           sync.Mutex // Protects limiter state changes
	isSlowedDown bool
	slowRate     rate.Limit
	slowBurst    int
	// Add health status if needed later
	nextNode uint32
}

func NewReplica(nodesConfig []config.NodeConfig, scheme string, slowRate float64, slowBurst int) (*Replica, error) {
	if scheme == "" {
		scheme = "http"
	}
	var (
		nodes []*Node
		// Start with no rate limit (Infinite rate, burst of 1 is effectively no limit)
		limiter = rate.NewLimiter(rate.Inf, 1)
		replica = &Replica{
			limiter:   limiter,
			slowRate:  rate.Limit(slowRate),
			slowBurst: slowBurst,
		}
	)
	for _, node := range nodesConfig {
		fullAddr := scheme + "://" + node.Address
		parsedURL, err := url.Parse(fullAddr)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, &Node{URL: parsedURL, Address: parsedURL.String(), Replica: replica})
	}
	replica.Nodes = nodes

	return replica, nil
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
		log.Printf("Slowing down replica %s to %v req/sec", r.Nodes[0].Address, r.slowRate)
		r.limiter.SetLimit(r.slowRate)
		r.limiter.SetBurst(r.slowBurst)
		r.isSlowedDown = true
		// Optional: Start a timer/goroutine here to attempt recovery later
	}
}

func (r *Replica) NextNode() *Node {
	idx := atomic.AddUint32(&r.nextNode, 1) - 1
	return r.Nodes[idx%uint32(len(r.Nodes))]
}

// Optional: Add SpeedUp() method for recovery logic
// func (r *Replica) SpeedUp() { ... }
