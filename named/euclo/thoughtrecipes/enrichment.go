package thoughtrecipes

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// hydrateEnrichmentSource hydrates an enrichment source into the state.
// This is a placeholder implementation - real enrichment would call archaeology,
// AST index, and BKC services.
func hydrateEnrichmentSource(ctx context.Context, source EnrichmentSource, state *core.Context, task *core.Task) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	switch source {
	case EnrichmentAST:
		// In a real implementation, this would query the AST index
		state.Set("enrichment.ast", "AST enrichment: codebase structure analyzed")
		return nil

	case EnrichmentArchaeology:
		// In a real implementation, this would call archaeology explore
		state.Set("enrichment.archaeology", "Archaeology enrichment: code patterns and provenance analyzed")
		return nil

	case EnrichmentBKC:
		// In a real implementation, this would load BKC semantic context
		state.Set("enrichment.bkc", "BKC enrichment: semantic knowledge context loaded")
		return nil

	default:
		return fmt.Errorf("unknown enrichment source: %s", source)
	}
}

// buildEnrichmentBundle creates an EnrichmentBundle from the current state.
func buildEnrichmentBundle(state *core.Context) EnrichmentBundle {
	bundle := EnrichmentBundle{}
	if state == nil {
		return bundle
	}

	if val, ok := state.Get("enrichment.ast"); ok {
		if s, ok := val.(string); ok {
			bundle.AST = s
		}
	}
	if val, ok := state.Get("enrichment.archaeology"); ok {
		if s, ok := val.(string); ok {
			bundle.Archaeology = s
		}
	}
	if val, ok := state.Get("enrichment.bkc"); ok {
		if s, ok := val.(string); ok {
			bundle.BKC = s
		}
	}

	return bundle
}

// artifactsFromResult extracts artifacts from a paradigm execution result.
func artifactsFromResult(result *core.Result) []euclotypes.Artifact {
	if result == nil || result.Data == nil {
		return nil
	}

	// Try to extract artifacts from result data
	if arts, ok := result.Data["artifacts"].([]euclotypes.Artifact); ok {
		return arts
	}

	// Also check for single artifact
	if art, ok := result.Data["artifact"].(euclotypes.Artifact); ok {
		return []euclotypes.Artifact{art}
	}

	return nil
}
