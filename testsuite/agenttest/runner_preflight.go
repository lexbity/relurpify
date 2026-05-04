package agenttest

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

// preflightCaseBackend performs provider-agnostic preflight using ManagedBackend interface.
// It calls ListModels() to find the requested model and Health() to verify backend status.
// Soft-allows backends with empty model lists but healthy status.
func preflightCaseBackend(ctx context.Context, backend llm.ManagedBackend, model string) (*BackendModelProvenance, error) {
	// Check backend health first
	health, err := backend.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("backend health check failed: %w", err)
	}
	if health == nil {
		return nil, fmt.Errorf("backend health check returned nil report")
	}

	// List available models
	models, err := backend.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("backend model list failed: %w", err)
	}

	// Search for the requested model
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model name is empty")
	}

	for _, m := range models {
		if strings.EqualFold(strings.TrimSpace(m.Name), model) {
			// Model found - return provenance
			return &BackendModelProvenance{
				RequestedModel: model,
				LoadedName:     m.Name,
				LoadedModel:    m.Name,
				Details: map[string]any{
					"family":        m.Family,
					"parameter_size": m.ParameterSize,
					"context_size":   m.ContextSize,
					"quantization":  m.Quantization,
					"has_gpu":       m.HasGPU,
				},
			}, nil
		}
	}

	// Model not found in list
	// Soft-allow: if backend is healthy and list is empty, proceed anyway
	if len(models) == 0 && health.State == llm.BackendHealthReady {
		return &BackendModelProvenance{
			RequestedModel: model,
			LoadedName:     model,
			LoadedModel:    model,
			Details: map[string]any{
				"note": "model list was empty but backend was healthy - proceeding",
			},
		}, nil
	}

	return nil, fmt.Errorf("model %q not found in backend model list (found %d models)", model, len(models))
}
