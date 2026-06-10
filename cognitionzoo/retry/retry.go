package retry

import (
	"context"
	"math"
	"math/rand"
	"time"
)

type Policy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	JitterFraction    float64
}

func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 1.5,
		JitterFraction:    0.1,
	}
}

type BackoffCalculator struct {
	policy  Policy
	attempt int
}

func NewBackoffCalculator(p Policy) *BackoffCalculator {
	return &BackoffCalculator{
		policy:  p,
		attempt: 0,
	}
}

func (bc *BackoffCalculator) NextBackoff() time.Duration {
	bc.attempt++

	baseDuration := float64(bc.policy.InitialBackoff)
	multiplier := math.Pow(bc.policy.BackoffMultiplier, float64(bc.attempt-1))
	computedBackoff := time.Duration(baseDuration * multiplier)

	computedBackoff = min(computedBackoff, bc.policy.MaxBackoff)

	if bc.policy.JitterFraction > 0 && bc.attempt > 1 {
		jitter := computeJitter(computedBackoff, bc.policy.JitterFraction)
		computedBackoff += jitter
	}

	computedBackoff = min(computedBackoff, bc.policy.MaxBackoff)

	return computedBackoff
}

func computeJitter(baseDuration time.Duration, jitterFraction float64) time.Duration {
	if baseDuration <= 0 || jitterFraction <= 0 {
		return 0
	}
	jitterMs := int64(baseDuration.Milliseconds()) * int64(jitterFraction*100) / 100
	if jitterMs <= 0 {
		return 0
	}
	randomJitter := rand.Int63n(2*jitterMs) - jitterMs
	return time.Duration(randomJitter) * time.Millisecond
}

func (bc *BackoffCalculator) Reset() {
	bc.attempt = 0
}

func Sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
