package agenttest

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// preflightCaseBackend performs provider-agnostic preflight using ManagedBackend interface.
// It calls ListModels() to find the requested model and Health() to verify backend status.
// Soft-allows backends with empty model lists but healthy status.
// The context should have a timeout to prevent indefinite hangs.
func preflightCaseBackend(ctx context.Context, backend llm.ManagedBackend, model string, telemetry core.Telemetry, logger *log.Logger) (*BackendModelProvenance, error) {
	startTime := time.Now()
	if logger != nil {
		logger.Printf("[preflight] starting backend preflight for model=%q at=%s", model, startTime.Format(time.RFC3339))
	}
	emitPreflightEvent(telemetry, "preflight_start", model, map[string]interface{}{
		"start_time": startTime.Format(time.RFC3339),
	})

	// Check for context cancellation/deadline before starting
	if err := ctx.Err(); err != nil {
		duration := time.Since(startTime)
		emitPreflightEvent(telemetry, "preflight_context_error", model, map[string]interface{}{
			"error":    err.Error(),
			"duration": duration.Milliseconds(),
		})
		return nil, fmt.Errorf("preflight context error before health check: %w (timeout may have occurred)", err)
	}

	// Check backend health first
	healthStart := time.Now()
	if logger != nil {
		logger.Printf("[preflight] calling backend.Health() for model=%q", model)
	}
	health, err := backend.Health(ctx)
	healthDuration := time.Since(healthStart)
	if err != nil {
		duration := time.Since(startTime)
		emitPreflightEvent(telemetry, "preflight_health_failed", model, map[string]interface{}{
			"error":           err.Error(),
			"health_duration": healthDuration.Milliseconds(),
			"total_duration":  duration.Milliseconds(),
		})
		return nil, fmt.Errorf("backend health check failed after %v: %w (endpoint may be unreachable or misconfigured)", healthDuration, err)
	}
	if health == nil {
		duration := time.Since(startTime)
		emitPreflightEvent(telemetry, "preflight_health_nil", model, map[string]interface{}{
			"health_duration": healthDuration.Milliseconds(),
			"total_duration":  duration.Milliseconds(),
		})
		return nil, fmt.Errorf("backend health check returned nil report after %v", healthDuration)
	}
	if logger != nil {
		logger.Printf("[preflight] backend health check passed state=%q duration=%v", health.State, healthDuration)
	}

	// List available models
	listStart := time.Now()
	if logger != nil {
		logger.Printf("[preflight] calling backend.ListModels() for model=%q", model)
	}
	models, err := backend.ListModels(ctx)
	listDuration := time.Since(listStart)
	if err != nil {
		duration := time.Since(startTime)
		emitPreflightEvent(telemetry, "preflight_list_failed", model, map[string]interface{}{
			"error":          err.Error(),
			"health_state":   health.State,
			"list_duration":  listDuration.Milliseconds(),
			"total_duration": duration.Milliseconds(),
		})
		return nil, fmt.Errorf("backend model list failed after %v: %w (endpoint may be misconfigured or model unavailable)", listDuration, err)
	}
	if logger != nil {
		logger.Printf("[preflight] backend ListModels() returned %d models duration=%v", len(models), listDuration)
	}

	// Search for the requested model
	model = strings.TrimSpace(model)
	if model == "" {
		duration := time.Since(startTime)
		emitPreflightEvent(telemetry, "preflight_empty_model", "", map[string]interface{}{
			"total_duration": duration.Milliseconds(),
		})
		return nil, fmt.Errorf("model name is empty (check testsuite YAML configuration)")
	}

	for _, m := range models {
		if strings.EqualFold(strings.TrimSpace(m.Name), model) {
			// Model found - return provenance
			duration := time.Since(startTime)
			if logger != nil {
				logger.Printf("[preflight] model %q found in backend list total_duration=%v", model, duration)
			}
			emitPreflightEvent(telemetry, "preflight_success", model, map[string]interface{}{
				"found_model":    m.Name,
				"family":         m.Family,
				"context_size":   m.ContextSize,
				"total_duration": duration.Milliseconds(),
			})
			return &BackendModelProvenance{
				RequestedModel: model,
				LoadedName:     m.Name,
				LoadedModel:    m.Name,
				Details: map[string]any{
					"family":         m.Family,
					"parameter_size": m.ParameterSize,
					"context_size":   m.ContextSize,
					"quantization":   m.Quantization,
					"has_gpu":        m.HasGPU,
					"preflight_ms":   duration.Milliseconds(),
				},
			}, nil
		}
	}

	// Model not found in list
	// Soft-allow: if backend is healthy and list is empty, proceed anyway
	if len(models) == 0 && health.State == llm.BackendHealthReady {
		duration := time.Since(startTime)
		if logger != nil {
			logger.Printf("[preflight] model list empty but backend healthy, soft-allowing model=%q duration=%v", model, duration)
		}
		emitPreflightEvent(telemetry, "preflight_soft_allow", model, map[string]interface{}{
			"health_state":   health.State,
			"note":           "model list was empty but backend was healthy - proceeding",
			"total_duration": duration.Milliseconds(),
		})
		return &BackendModelProvenance{
			RequestedModel: model,
			LoadedName:     model,
			LoadedModel:    model,
			Details: map[string]any{
				"note":         "model list was empty but backend was healthy - proceeding",
				"preflight_ms": duration.Milliseconds(),
			},
		}, nil
	}

	duration := time.Since(startTime)
	if logger != nil {
		logger.Printf("[preflight] model %q not found in %d available models duration=%v", model, len(models), duration)
	}

	// Build list of available models for error context (limit to prevent huge error messages)
	availableModels := make([]string, 0, min(len(models), 10))
	for i, m := range models {
		if i >= 10 {
			availableModels = append(availableModels, fmt.Sprintf("... and %d more", len(models)-10))
			break
		}
		availableModels = append(availableModels, m.Name)
	}

	emitPreflightEvent(telemetry, "preflight_model_not_found", model, map[string]interface{}{
		"available_count":  len(models),
		"available_models": availableModels,
		"health_state":     health.State,
		"total_duration":   duration.Milliseconds(),
	})

	return nil, fmt.Errorf("model %q not found in backend model list (found %d models: %v) - verify the model is loaded and the endpoint is correct", model, len(models), availableModels)
}

// emitPreflightEvent emits a telemetry event for preflight operations if telemetry is available.
func emitPreflightEvent(telemetry core.Telemetry, eventType string, model string, metadata map[string]interface{}) {
	if telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["model"] = model
	telemetry.Emit(core.Event{
		Type:      core.EventType("preflight_" + eventType),
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("preflight %s: %s", eventType, model),
		Metadata:  metadata,
	})
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
