package idr

import (
	"errors"
	"sync"
	"time"
)

// Circuit breaker states
const (
	StateClosed   = "closed"    // Normal operation
	StateOpen     = "open"      // Failing, rejecting requests
	StateHalfOpen = "half-open" // Testing if service recovered
)

// ErrCircuitOpen is returned when the circuit breaker is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold int           // Failures before opening circuit
	SuccessThreshold int           // Successes to close circuit from half-open
	Timeout          time.Duration // Time to wait before half-open
	MaxConcurrent    int           // Max concurrent requests (0 = unlimited)
	OnStateChange    func(from, to string)
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    100,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	mu              sync.RWMutex
	state           string
	failures        int
	successes       int
	lastFailureTime time.Time
	concurrent      int

	// Metrics
	totalRequests  int64
	totalFailures  int64
	totalSuccesses int64
	totalRejected  int64

	// Callback lifecycle management
	callbackWg sync.WaitGroup
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err)
	return err
}

// beforeRequest checks if the request should proceed
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++

	switch cb.state {
	case StateClosed:
		// Check concurrent limit
		if cb.config.MaxConcurrent > 0 && cb.concurrent >= cb.config.MaxConcurrent {
			cb.totalRejected++
			return errors.New("max concurrent requests exceeded")
		}
		cb.concurrent++
		return nil

	case StateOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.concurrent++
			return nil
		}
		cb.totalRejected++
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow limited requests through
		if cb.concurrent < 1 { // Only allow one request at a time in half-open
			cb.concurrent++
			return nil
		}
		cb.totalRejected++
		return ErrCircuitOpen
	}

	return nil
}

// afterRequest records the result of a request
func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.concurrent--

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

// recordFailure records a failed request
func (cb *CircuitBreaker) recordFailure() {
	cb.totalFailures++
	cb.failures++
	cb.successes = 0
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		cb.setState(StateOpen)
	}
}

// recordSuccess records a successful request
func (cb *CircuitBreaker) recordSuccess() {
	cb.totalSuccesses++
	cb.successes++

	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
			cb.failures = 0
		}
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState string) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.successes = 0

	if cb.config.OnStateChange != nil {
		// Track callback goroutine for graceful shutdown
		cb.callbackWg.Add(1)
		go func(from, to string) {
			defer cb.callbackWg.Done()
			cb.config.OnStateChange(from, to)
		}(oldState, newState)
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitBreakerStats{
		State:          cb.state,
		TotalRequests:  cb.totalRequests,
		TotalFailures:  cb.totalFailures,
		TotalSuccesses: cb.totalSuccesses,
		TotalRejected:  cb.totalRejected,
		Failures:       cb.failures,
		Concurrent:     cb.concurrent,
	}
}

// CircuitBreakerStats holds circuit breaker statistics
type CircuitBreakerStats struct {
	State          string `json:"state"`
	TotalRequests  int64  `json:"total_requests"`
	TotalFailures  int64  `json:"total_failures"`
	TotalSuccesses int64  `json:"total_successes"`
	TotalRejected  int64  `json:"total_rejected"`
	Failures       int    `json:"current_failures"`
	Concurrent     int    `json:"concurrent"`
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateClosed)
	cb.failures = 0
	cb.successes = 0
}

// ForceOpen forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateOpen)
	cb.lastFailureTime = time.Now()
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen
}

// Close waits for any pending state change callbacks to complete.
// Call this during graceful shutdown to ensure all callbacks finish.
func (cb *CircuitBreaker) Close() {
	cb.callbackWg.Wait()
}
