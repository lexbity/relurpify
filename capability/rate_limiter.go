package capability

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// toolRateBucket implements a simple token-bucket rate limiter for a single
// tool. It is not exported; use RateLimitedInvoker for per-token limiting.
type toolRateBucket struct {
	mu        sync.Mutex
	perSecond float64
	burst     int
	tokens    float64
	lastFill  time.Time
}

func newToolRateBucket(perSecond float64, burst int) *toolRateBucket {
	if perSecond <= 0 {
		perSecond = 1 // default: 1 per second
	}
	if burst <= 0 {
		burst = int(perSecond) // default: perSecond as burst
		if burst < 1 {
			burst = 1
		}
	}
	return &toolRateBucket{
		perSecond: perSecond,
		burst:     burst,
		tokens:    float64(burst),
		lastFill:  time.Now(),
	}
}

func (b *toolRateBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.lastFill = now
	b.tokens += elapsed * b.perSecond
	if b.tokens > float64(b.burst) {
		b.tokens = float64(b.burst)
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimitedInvoker wraps a CapabilityInvoker and enforces per-tool rate
// limits using the limits map keyed by tool name. Tools not in the map are
// not rate-limited. When a tool exceeds its rate, the context is checked
// for cancellation and ErrRateLimitExceeded is returned.
type RateLimitedInvoker struct {
	inner  capabilityInvoker
	limits map[string]*toolRateBucket
	mu     sync.RWMutex
}

// capabilityInvoker is the local interface defining InvokeCapability that
// RateLimitedInvoker wraps. Users can pass any value with this method.
type capabilityInvoker interface {
	InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]any) (*ports.ToolResult, error)
}

// ErrRateLimitExceeded is returned when a tool call is rate-limited.
var ErrRateLimitExceeded = fmt.Errorf("rate limit exceeded")

// NewRateLimitedInvoker creates a rate limiter that delegates to inner.
// The limits map associates tool names with per-second rate and burst.
func NewRateLimitedInvoker(inner capabilityInvoker, limits map[string]ports.ToolRateLimit) *RateLimitedInvoker {
	r := &RateLimitedInvoker{
		inner:  inner,
		limits: make(map[string]*toolRateBucket),
	}
	for name, l := range limits {
		r.limits[name] = newToolRateBucket(l.PerSecond, l.Burst)
	}
	return r
}

// InvokeCapability applies rate limiting before delegating to the inner
// invoker. If the tool is rate-limited, the call blocks until a token is
// available or the context is cancelled.
func (r *RateLimitedInvoker) InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	bucket := r.bucketFor(idOrName)
	if bucket != nil {
		for {
			if bucket.allow() {
				break
			}
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %w", ErrRateLimitExceeded, ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	return r.inner.InvokeCapability(ctx, env, idOrName, args)
}

func (r *RateLimitedInvoker) bucketFor(name string) *toolRateBucket {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.limits[name]
}
