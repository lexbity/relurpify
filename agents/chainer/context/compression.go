package context

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
)

// CompressionListener reacts to budget events by compressing context.
//
// Compression strategies:
//   - OnBudgetWarning: apply adaptive compression (preserve first + recent interactions)
//   - OnBudgetExceeded: apply aggressive compression (metadata-only) or halt
type CompressionListener struct {
	strategy     CompressionStrategy
	lastWarning  int // tokens at last warning
	warningCount int // number of warnings triggered
}

// CompressionStrategy defines how to compress context.
type CompressionStrategy string

const (
	// StrategyAdaptive preserves first interactions + recent (80/20 rule)
	StrategyAdaptive CompressionStrategy = "adaptive"
	// StrategyAggressive removes old interactions, keeps metadata only
	StrategyAggressive CompressionStrategy = "aggressive"
	// StrategyConservative keeps everything, minimal compression
	StrategyConservative CompressionStrategy = "conservative"
)

// NewCompressionListener creates a listener with adaptive compression.
func NewCompressionListener() *CompressionListener {
	return &CompressionListener{
		strategy:    StrategyAdaptive,
		lastWarning: 0,
		warningCount: 0,
	}
}

// NewCompressionListenerWithStrategy creates a listener with specified strategy.
func NewCompressionListenerWithStrategy(strategy CompressionStrategy) *CompressionListener {
	return &CompressionListener{
		strategy:    strategy,
		lastWarning: 0,
		warningCount: 0,
	}
}

// OnBudgetWarning is called when token usage reaches warning threshold.
func (c *CompressionListener) OnBudgetWarning(remaining int, limit int) error {
	if c == nil {
		return fmt.Errorf("compression listener not initialized")
	}

	// Only compress if remaining space has shrunk significantly
	if c.lastWarning > 0 && remaining > c.lastWarning/2 {
		return nil // Still have room, no action needed
	}

	c.warningCount++
	c.lastWarning = remaining

	// Log compression event
	percent := (remaining * 100) / limit
	compressed := limit - remaining
	return fmt.Errorf(
		"budget warning: %d tokens used (%d%%), %d tokens remaining. "+
			"Apply %s compression. Warnings: %d",
		compressed, 100-percent, remaining, c.strategy, c.warningCount,
	)
}

// OnBudgetExceeded is called when token usage exceeds limit.
func (c *CompressionListener) OnBudgetExceeded(remaining int, limit int) error {
	if c == nil {
		return fmt.Errorf("compression listener not initialized")
	}

	// Budget exceeded - need aggressive action
	return fmt.Errorf(
		"budget exceeded: token limit %d reached. Cannot proceed without aggressive compression. "+
			"Strategy: %s. Warnings before exceed: %d",
		limit, c.strategy, c.warningCount,
	)
}

// CompressContext applies compression to a context based on strategy.
//
// Phase 3 stub - will be integrated with contextmgr.Compress in later phases.
func CompressContext(ctx *core.Context, strategy CompressionStrategy) (*core.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context required")
	}

	switch strategy {
	case StrategyAdaptive:
		return compressAdaptive(ctx)
	case StrategyAggressive:
		return compressAggressive(ctx)
	case StrategyConservative:
		return ctx, nil // No compression
	default:
		return nil, fmt.Errorf("unknown compression strategy: %s", strategy)
	}
}

// compressAdaptive preserves first + recent interactions (80/20 rule).
func compressAdaptive(ctx *core.Context) (*core.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context required")
	}

	// Phase 3 stub: Framework's contextmgr will handle actual compression
	// For now, return a copy indicating compression was "applied"
	compressed := ctx.Clone()
	compressed.Set("__compression_applied", "adaptive")
	return compressed, nil
}

// compressAggressive removes old interactions, keeps metadata only.
func compressAggressive(ctx *core.Context) (*core.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context required")
	}

	// Phase 3 stub: Framework's contextmgr will handle actual compression
	compressed := ctx.Clone()
	compressed.Set("__compression_applied", "aggressive")
	return compressed, nil
}

// WarningCount returns the number of budget warnings triggered.
func (c *CompressionListener) WarningCount() int {
	if c == nil {
		return 0
	}
	return c.warningCount
}

// Reset clears warning state.
func (c *CompressionListener) Reset() {
	if c == nil {
		return
	}
	c.lastWarning = 0
	c.warningCount = 0
}
