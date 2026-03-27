package routing

import (
	"context"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type CBState int

const (
	StateClosed   CBState = iota // Healthy
	StateOpen                    // Failing, do not route
	StateHalfOpen                // Probing to see if recovered
)

type Backend struct {
	ID      string
	Address string
	Client  grpc.ClientConnInterface
	conn    *grpc.ClientConn // Store actual connection for health checks

	// Circuit Breaker State[]
	cbState          CBState
	failureCount     int
	failureThreshold int
	openTime         time.Time
	openDuration     time.Duration
	cbMutex          sync.RWMutex

	// EWMA Latency Tracker
	ewmaLatency float64
	alpha       float64
	latencyMu   sync.RWMutex

	// Health Check
	healthCheckInterval time.Duration
	stopHealthCheck     chan struct{}
}

func NewBackend(id, address string, conn *grpc.ClientConn) *Backend {
	b := &Backend{
		ID:                  id,
		Address:             address,
		Client:              conn,
		conn:                conn,
		cbState:             StateClosed,
		failureThreshold:    3,                // Open circuit after 3 consecutive failures
		openDuration:        10 * time.Second, // Wait 10s before probing
		ewmaLatency:         50.0,             // Assume 50ms default starting latency
		alpha:               0.3,              // Weight recent requests heavily
		healthCheckInterval: 5 * time.Second,  // Health check every 5 seconds
		stopHealthCheck:     make(chan struct{}),
	}
	// Start active health checking in background
	go b.startHealthCheck()
	return b
}

// UpdateLatency is called after a successful streaming request completes
func (b *Backend) UpdateLatency(latency time.Duration) {
	b.latencyMu.Lock()
	defer b.latencyMu.Unlock()
	ms := float64(latency.Milliseconds())
	// EWMA Formula
	b.ewmaLatency = (b.alpha * ms) + ((1 - b.alpha) * b.ewmaLatency)
}

// RecordSuccess resets the circuit breaker
func (b *Backend) RecordSuccess() {
	b.cbMutex.Lock()
	defer b.cbMutex.Unlock()
	b.failureCount = 0
	b.cbState = StateClosed
}

// RecordFailure trips the circuit breaker if threshold is reached
func (b *Backend) RecordFailure() {
	b.cbMutex.Lock()
	defer b.cbMutex.Unlock()
	b.failureCount++

	// Trip circuit if we hit the threshold in Closed state, OR if a probe fails in HalfOpen state
	if (b.cbState == StateClosed && b.failureCount >= b.failureThreshold) || b.cbState == StateHalfOpen {
		log.Printf("[CircuitBreaker] Backend %s (%s) OPENED (failures: %d)", b.ID, b.Address, b.failureCount)
		b.cbState = StateOpen
		b.openTime = time.Now()
	} else if b.cbState == StateClosed {
		log.Printf("[CircuitBreaker] Backend %s (%s) failure count: %d/%d", b.ID, b.Address, b.failureCount, b.failureThreshold)
	}
}

// GetWeight returns the inverse latency (higher weight = faster backend). Returns 0 if circuit is open.
func (b *Backend) GetWeight() float64 {
	b.cbMutex.RLock()
	state := b.cbState
	timeSinceOpen := time.Since(b.openTime)
	b.cbMutex.RUnlock()

	if state == StateOpen {
		if timeSinceOpen > b.openDuration {
			// Time to probe. Transition to HalfOpen
			b.cbMutex.Lock()
			// Double-check inside lock to prevent race conditions from concurrent requests
			if b.cbState == StateOpen {
				log.Printf("[CircuitBreaker] Backend %s (%s) transitioning to HALF-OPEN (probing)", b.ID, b.Address)
				b.cbState = StateHalfOpen
			}
			b.cbMutex.Unlock()
			return 0.1 // Give it a very tiny weight just to probe
		}
		return 0.0 // Do not route here
	}

	// Keep weight tiny while probing so we don't flood it with traffic!
	if state == StateHalfOpen {
		return 0.1
	}

	b.latencyMu.RLock()
	latency := b.ewmaLatency
	b.latencyMu.RUnlock()

	if latency <= 0 {
		return 1.0 // Prevent division by zero
	}
	return 1000.0 / latency // Inverse EWMA
}

// startHealthCheck continuously checks backend health and updates circuit breaker state
func (b *Backend) startHealthCheck() {
	ticker := time.NewTicker(b.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.performHealthCheck()
		case <-b.stopHealthCheck:
			return
		}
	}
}

// performHealthCheck checks if the gRPC connection is ready
func (b *Backend) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Check gRPC connection state
	state := b.conn.GetState()
	
	// Try to transition to READY state if not already
	if state != connectivity.Ready {
		// Wait for state change or timeout
		b.conn.WaitForStateChange(ctx, state)
		state = b.conn.GetState()
	}

	if state == connectivity.Ready {
		// Connection is healthy
		b.cbMutex.Lock()
		currentState := b.cbState
		b.cbMutex.Unlock()

		// Only log state changes to reduce noise
		if currentState != StateClosed {
			log.Printf("[HealthCheck] Backend %s (%s) is healthy - transitioning to Closed", b.ID, b.Address)
			b.RecordSuccess()
		}
	} else {
		// Connection is unhealthy (TransientFailure, Connecting, Idle, or Shutdown)
		log.Printf("[HealthCheck] Backend %s (%s) is unhealthy (state: %v) - recording failure", b.ID, b.Address, state)
		b.RecordFailure()
	}
}

// Stop gracefully stops the health check goroutine
func (b *Backend) Stop() {
	close(b.stopHealthCheck)
}
