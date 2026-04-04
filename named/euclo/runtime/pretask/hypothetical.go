package pretask

import (
	"context"
)

// HypotheticalGenerator generates a grounded vocabulary sketch.
type HypotheticalGenerator struct{}

// Generate produces a vocabulary sketch grounded in Stage 1 evidence.
func (g *HypotheticalGenerator) Generate(
	ctx context.Context,
	query string,
	stage1 Stage1Result,
) (HypotheticalSketch, error) {
	// Stub implementation for Phase 1.
	return HypotheticalSketch{Grounded: false}, nil
}
