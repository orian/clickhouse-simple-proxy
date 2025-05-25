// --- proxy.go --- (Main proxy logic)
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SimpleProxy struct {
	config        *Config
	replicas      []*Replica
	nextReplica   uint32       // For simple round-robin
	groupLimiters sync.Map     // map[string]*GroupLimiter
	httpClient    *http.Client // For the reverse proxy transport
}

func NewSimpleProxy(cfg *Config) (*SimpleProxy, error) {
	if cfg.HeaderName == "" || len(cfg.Replicas) == 0 {
		return nil, errors.New("header_name and replicas are required")
	}

	replicas := make([]*Replica, 0, len(cfg.Replicas))
	for _, addr := range cfg.Replicas {
		r, err := NewReplica(addr, cfg.ReplicaScheme, cfg.SlowdownRate, cfg.SlowdownBurst)
		if err != nil {
			return nil, fmt.Errorf("failed to create replica %s: %w", addr, err)
		}
		replicas = append(replicas, r)
	}

	// Basic HTTP client transport configuration
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		// Add other settings like TLS config if needed
	}

	// --- Use configured timeout, with a default ---
	proxyTimeout := cfg.ProxyTimeout
	if proxyTimeout <= 0 {
		proxyTimeout = 120 * time.Second // Default if not set or invalid
		log.Printf("Proxy timeout not configured, using default: %s", proxyTimeout)
	}

	return &SimpleProxy{
		config:   cfg,
		replicas: replicas,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   proxyTimeout,
		},
	}, nil
}

func (p *SimpleProxy) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 1. Get Group Key
	groupKey := r.Header.Get(p.config.HeaderName)
	if groupKey == "" {
		slog.Debug("Missing header", "header", p.config.HeaderName)
		http.Error(rw, fmt.Sprintf("Missing header: %s", p.config.HeaderName), http.StatusBadRequest)
		return
	}

	// 2. Get or Create Limiter for the group
	limiterUntyped, _ := p.groupLimiters.LoadOrStore(groupKey, NewGroupLimiter(
		p.config.MaxConcurrent,
		p.config.MaxQueue,
		p.config.QueueTimeout,
	))
	limiter := limiterUntyped.(*GroupLimiter)

	// 3. Acquire Concurrency Slot (handles queueing)
	ctx := r.Context()
	if err := limiter.Acquire(ctx); err != nil {
		log.Printf("Group %q: Failed to acquire slot: %v", groupKey, err)
		statusCode := http.StatusServiceUnavailable
		if errors.Is(err, ErrQueueFull) || errors.Is(err, ErrQueueTimeout) {
			statusCode = http.StatusTooManyRequests
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			statusCode = 499 // Client Closed Request
		}
		http.Error(rw, err.Error(), statusCode)
		return
	}
	defer limiter.Release() // IMPORTANT: Release the slot when done

	// 4. Select Replica (Simple Round Robin - NEEDS HEALTH CHECKS FOR PROD)
	replica := p.selectReplica()
	if replica == nil {
		log.Printf("Group %q: No replicas available", groupKey)
		http.Error(rw, "No available backend replicas", http.StatusServiceUnavailable)
		return
	}

	// 5. Wait for Replica's Rate Limiter
	if err := replica.Wait(ctx); err != nil {
		log.Printf("Group %q: Replica %s rate limit wait error: %v", groupKey, replica.URL.Host, err)
		statusCode := http.StatusServiceUnavailable
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			statusCode = 499
		}
		http.Error(rw, "Backend replica rate limited or request cancelled", statusCode)
		return
	}

	// 6. Setup Reverse Proxy
	reverseProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = replica.URL.Scheme
			req.URL.Host = replica.URL.Host
			req.URL.Path = r.URL.Path   // Use original path
			req.Host = replica.URL.Host // Set Host header
			// Optional: Remove the grouping header
			req.Header.Del(p.config.HeaderName)
			if p.config.UserAgent != "" {
				req.Header.Set("User-Agent", p.config.UserAgent)
			}
			req.URL.RawQuery = r.URL.RawQuery
		},
		Transport: p.httpClient.Transport,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("Group %q: Proxy error to %s: %v", groupKey, replica.URL.Host, err)
			// Check for specific errors like context cancellation or timeout
			statusCode := http.StatusBadGateway
			// Check if the error is a timeout from the http client
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				statusCode = http.StatusGatewayTimeout // More specific error for timeouts
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				statusCode = 499 // Client closed request or deadline exceeded before sending
			}
			// Ensure header isn't already sent before writing header
			// (httputil usually handles this, but good practice)
			if _, ok := w.(http.Hijacker); !ok { // Check if response hasn't been hijacked
				w.WriteHeader(statusCode)
			} else {
				log.Printf("Group %q: Cannot write header for error on hijacked connection to %s", groupKey, replica.URL.Host)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Check if this response indicates the need to slow down
			// WARNING: Reading the body is tricky. Best if CH provides a header or specific status code.
			shouldSlowDown := false
			if p.config.SlowdownCode > 0 && resp.StatusCode == p.config.SlowdownCode {
				shouldSlowDown = true
			}

			// If checking body content is necessary:
			if !shouldSlowDown && p.config.SlowdownError != "" && (resp.StatusCode >= 500 || resp.StatusCode == http.StatusServiceUnavailable) {
				// Read body (up to a limit to avoid memory issues)
				const maxBodyRead = 1 * 1024 * 1024 // 1MB limit
				bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
				// Always close the original body reader *after* trying to read
				if closeErr := resp.Body.Close(); closeErr != nil {
					log.Printf("Group %q: Error closing original response body from %s: %v", groupKey, replica.URL.Host, closeErr)
				}

				if readErr == nil {
					// Restore the body for the client
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					resp.ContentLength = int64(len(bodyBytes)) // May need adjustment if original was chunked/unknown
					// Ensure Transfer-Encoding is removed if ContentLength is set.
					// The Go http server usually handles this, but explicit can be safer.
					resp.Header.Del("Transfer-Encoding")

					// Check if the error message is present
					if strings.Contains(string(bodyBytes), p.config.SlowdownError) {
						shouldSlowDown = true
					}
				} else {
					log.Printf("Group %q: Error reading response body from %s for slowdown check: %v", groupKey, replica.URL.Host, readErr)
					// Cannot check body, maybe return error to proxy?
					// If we return an error here, the client gets a generic Bad Gateway.
					// return fmt.Errorf("failed to read response body: %w", readErr)
					// If we return nil, the potentially erroneous (but unreadable) response goes to client.
				}
			}

			if shouldSlowDown {
				log.Printf("Group %q: Triggering slowdown for replica %s", groupKey, replica.URL.Host)
				replica.SlowDown()
			}
			return nil // Return nil even if slowdown triggered
		},
		// Add FlushInterval for streaming responses if needed
		// FlushInterval: -1, // Use Go's default flushing
	}

	// 7. Serve the request
	log.Printf("Group %q: Proxying to %s (Queued/Limited: %t)", groupKey, replica.URL.Host, time.Since(startTime) > 10*time.Millisecond) // Basic indicator if it waited
	reverseProxy.ServeHTTP(rw, r)
	log.Printf("Group %q: Finished request to %s (Duration: %s)", groupKey, replica.URL.Host, time.Since(startTime))
}

// selectReplica implements simple round-robin. NEEDS HEALTH CHECKS.
func (p *SimpleProxy) selectReplica() *Replica {
	numReplicas := uint32(len(p.replicas))
	if numReplicas == 0 {
		return nil
	}

	// Atomically get the next index
	idx := atomic.AddUint32(&p.nextReplica, 1) - 1

	// Basic round robin - wrap around
	selected := p.replicas[idx%numReplicas]

	// !!! Placeholder for health check !!!
	// In production, you would loop here, checking health:
	// for i := uint32(0); i < numReplicas; i++ {
	//    currentIdx := (idx + i) % numReplicas
	//    if p.replicas[currentIdx].IsHealthy() { // Assuming IsHealthy() exists
	//        return p.replicas[currentIdx]
	//    }
	// }
	// return nil // Or handle case where no replicas are healthy

	return selected
}
