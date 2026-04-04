package anitd

import (
	"context"
	"fmt"
)

// Open initializes a complete workspace session: platform checks, store
// opening, service graph construction, agent registration, and background
// indexing. The returned *Workspace is ready for agent construction.
//
// Open is the single composition root for all Relurpify entry points.
// app/relurpish, app/dev-agent-cli, and integration tests all call Open().
func Open(ctx context.Context, cfg WorkspaceConfig) (*Workspace, error) {
	// Phase A: Configuration Validation
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid workspace config: %w", err)
	}

	// Phase B: Platform Runtime Checks
	results := ProbeWorkspace(cfg)
	for _, r := range results {
		if r.Required && !r.OK {
			return nil, fmt.Errorf("platform check failed: %s", r.Message)
		}
	}

	// TODO: Implement remaining phases
	// For now, return a stub workspace
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			// Will be populated in later phases
		},
	}
	return ws, nil
}

func validateConfig(cfg WorkspaceConfig) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("Workspace is required")
	}
	if cfg.ManifestPath == "" {
		return fmt.Errorf("ManifestPath is required")
	}
	if cfg.OllamaEndpoint == "" {
		return fmt.Errorf("OllamaEndpoint is required")
	}
	if cfg.OllamaModel == "" {
		return fmt.Errorf("OllamaModel is required")
	}
	return nil
}
