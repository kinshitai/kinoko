// Package circuitbreaker provides a thread-safe circuit breaker with
// exponential backoff on repeated half-open failures.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit breaker is open and not allowing requests.
var ErrOpen = errors.New("circuit breaker is open")

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// realClock uses the standard library.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type state int

const (
	stateClosed   state = iota
	stateOpen
	stateHalfOpen
)

// Config configures a Breaker.
type Config struct {
	// Threshold is the number of consecutive failures before opening.
	Threshold int
	// BaseDuration is the initial open duration.
	BaseDuration time.Duration
	// MaxDuration caps the escalating open duration.
	MaxDuration time.Duration
}

// Breaker is a thread-safe circuit breaker.
type Breaker struct {
	cfg   Config
	clock Clock

	mu              sync.Mutex
	state           state
	consecutiveFail int
	openedAt        time.Time
	openDuration    time.Duration
}

// New creates a Breaker. If clock is nil, real time is used.
func New(cfg Config, clock Clock) *Breaker {
	if clock == nil {
		clock = realClock{}
	}
	return &Breaker{
		cfg:          cfg,
		clock:        clock,
		openDuration: cfg.BaseDuration,
	}
}

// Allow checks whether a request is allowed. Returns ErrOpen if the circuit is open.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed:
		return nil
	case stateOpen:
		if b.clock.Now().Sub(b.openedAt) >= b.openDuration {
			b.state = stateHalfOpen
			return nil
		}
		return ErrOpen
	case stateHalfOpen:
		// Only one probe at a time; subsequent calls while half-open are rejected.
		return ErrOpen
	}
	return nil
}

// RecordSuccess records a successful request. Transitions half-open → closed.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state = stateClosed
	b.consecutiveFail = 0
	b.openDuration = b.cfg.BaseDuration
}

// RecordFailure records a failed request. May transition closed → open or half-open → open.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.consecutiveFail++

	switch b.state {
	case stateClosed:
		if b.consecutiveFail >= b.cfg.Threshold {
			b.state = stateOpen
			b.openedAt = b.clock.Now()
			b.openDuration = b.cfg.BaseDuration
		}
	case stateHalfOpen:
		// Re-open with escalated duration.
		next := b.openDuration * 2
		if next > b.cfg.MaxDuration {
			next = b.cfg.MaxDuration
		}
		b.openDuration = next
		b.state = stateOpen
		b.openedAt = b.clock.Now()
	}
}
