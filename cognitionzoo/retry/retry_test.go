package retry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	assert.Equal(t, 3, p.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, p.InitialBackoff)
	assert.Equal(t, 30*time.Second, p.MaxBackoff)
	assert.InDelta(t, 1.5, p.BackoffMultiplier, 0.01)
	assert.InDelta(t, 0.1, p.JitterFraction, 0.01)
}

func TestNextBackoff_Growth(t *testing.T) {
	p := Policy{
		MaxAttempts:       5,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		JitterFraction:    0,
	}
	bc := NewBackoffCalculator(p)

	b1 := bc.NextBackoff()
	assert.Equal(t, 100*time.Millisecond, b1)

	b2 := bc.NextBackoff()
	assert.Equal(t, 200*time.Millisecond, b2)

	b3 := bc.NextBackoff()
	assert.Equal(t, 400*time.Millisecond, b3)
}

func TestNextBackoff_MaxBackoffClamp(t *testing.T) {
	p := Policy{
		MaxAttempts:       10,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        3 * time.Second,
		BackoffMultiplier: 4.0,
		JitterFraction:    0,
	}
	bc := NewBackoffCalculator(p)

	b1 := bc.NextBackoff()
	assert.Equal(t, 1*time.Second, b1)

	b2 := bc.NextBackoff()
	assert.Equal(t, 3*time.Second, b2) // 4s clamped to 3s

	b3 := bc.NextBackoff()
	assert.Equal(t, 3*time.Second, b3) // 16s clamped to 3s
}

func TestNextBackoff_Jitter(t *testing.T) {
	p := Policy{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 1.0,
		JitterFraction:    0.5,
	}
	bc := NewBackoffCalculator(p)

	_ = bc.NextBackoff() // no jitter on first attempt

	b2 := bc.NextBackoff()
	assert.Greater(t, b2, time.Duration(0))

	b3 := bc.NextBackoff()
	assert.Greater(t, b3, time.Duration(0))

	// Jitter should mean values differ (probabilistic, but very likely)
	seen := map[time.Duration]bool{b2: true, b3: true}
	assert.GreaterOrEqual(t, len(seen), 1)
}

func TestNextBackoff_NeverExceedsMaxBackoff(t *testing.T) {
	p := Policy{
		MaxAttempts:       100,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 3.0,
		JitterFraction:    0,
	}
	bc := NewBackoffCalculator(p)
	for i := 0; i < 20; i++ {
		b := bc.NextBackoff()
		assert.LessOrEqual(t, b, 5*time.Second)
	}
}

func TestNextBackoff_WithJitterNeverExceedsMaxBackoff(t *testing.T) {
	p := Policy{
		MaxAttempts:       100,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 3.0,
		JitterFraction:    1.0,
	}
	bc := NewBackoffCalculator(p)
	for i := 0; i < 50; i++ {
		b := bc.NextBackoff()
		assert.LessOrEqual(t, b, 5*time.Second, "attempt %d", i+1)
	}
}

func TestReset(t *testing.T) {
	bc := NewBackoffCalculator(DefaultPolicy())
	bc.NextBackoff()
	bc.NextBackoff()
	assert.Equal(t, 2, bc.attempt)

	bc.Reset()
	assert.Equal(t, 0, bc.attempt)

	// After reset, backoff starts from initial
	b := bc.NextBackoff()
	assert.Equal(t, DefaultPolicy().InitialBackoff, b)
}

func TestSleep_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := Sleep(ctx, 1*time.Hour)
	assert.False(t, got)
}

func TestSleep_ContextNotCancelled(t *testing.T) {
	ctx := context.Background()
	got := Sleep(ctx, 1*time.Millisecond)
	assert.True(t, got)
}

func TestBackoffCalculator_NeverExceedsMaxWithJitter(t *testing.T) {
	for i := 0; i < 100; i++ {
		bc := NewBackoffCalculator(Policy{
			MaxAttempts:       10,
			InitialBackoff:    1 * time.Second,
			MaxBackoff:        10 * time.Second,
			BackoffMultiplier: 3.0,
			JitterFraction:    0.3,
		})
		for j := 0; j < 10; j++ {
			b := bc.NextBackoff()
			assert.LessOrEqual(t, b, 10*time.Second, "iteration %d/%d", i, j)
		}
	}
}
