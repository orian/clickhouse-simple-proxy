// --- limiter.go --- (Manages per-group concurrency and queueing)
package main

import (
	"context"
	"errors"
	"log"
	"time"
)

var ErrQueueFull = errors.New("request queue is full")
var ErrQueueTimeout = errors.New("request timed out in queue")

// GroupLimiter manages concurrency and queueing for a specific header value.
type GroupLimiter struct {
	concurrency  chan struct{} // Acts as a semaphore
	queue        chan struct{} // Buffered channel for queueing
	queueTimeout time.Duration
}

func NewGroupLimiter(maxConcurrent, maxQueue int, queueTimeout time.Duration) *GroupLimiter {
	if maxConcurrent <= 0 {
		maxConcurrent = 1 // Sensible default
	}
	// queue is nil if maxQueue is 0 or less, disabling queueing
	var queueChan chan struct{}
	if maxQueue > 0 {
		queueChan = make(chan struct{}, maxQueue)
	}

	return &GroupLimiter{
		concurrency:  make(chan struct{}, maxConcurrent),
		queue:        queueChan,
		queueTimeout: queueTimeout,
	}
}

// Acquire tries to get a slot. It blocks if the queue/concurrency limit is hit,
// until a slot is available or timeout occurs.
func (gl *GroupLimiter) Acquire(ctx context.Context) error {
	// 1. Try to enter the queue (non-blocking if queue disabled)
	if gl.queue != nil {
		select {
		case gl.queue <- struct{}{}:
			// Entered queue, now wait for concurrency slot
			defer func() { <-gl.queue }() // Free up queue slot when done waiting
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Queue is full
			return ErrQueueFull
		}
	}

	// 2. Wait for concurrency slot with timeout
	// Use a timer relative to the configured queue timeout
	queueTimer := time.NewTimer(gl.queueTimeout)
	defer queueTimer.Stop()

	select {
	case gl.concurrency <- struct{}{}:
		// Got concurrency slot
		return nil
	case <-queueTimer.C:
		return ErrQueueTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release gives back the concurrency slot.
func (gl *GroupLimiter) Release() {
	// Check if concurrency channel is initialized and not nil
	if gl.concurrency != nil {
		// Check channel length to prevent blocking if called incorrectly
		if len(gl.concurrency) < cap(gl.concurrency) {
			<-gl.concurrency // Release slot
		} else {
			// Log or handle potential error: trying to release when full?
			// This case shouldn't happen with proper Acquire/Release pairing.
			log.Printf("Warning: Attempted to release concurrency slot when channel was potentially empty or logic error occurred.")
		}
	} else {
		log.Printf("Warning: Attempted to release concurrency slot on a nil channel.")
	}
}
