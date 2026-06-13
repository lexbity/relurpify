package session

import (
	"fmt"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/execution/prompt"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const promptBlockIDKey = "block_id"

// BuildPromptRegistry constructs the workspace prompt registry and loads all
// .prompt files from templates/prompts/ (framework, agents, named) first, then relurpify_cfg/prompts/.
// Provider registration is deferred to named-agent Initialize() calls.
// ValidateProviders() must be called after all agents have initialized.
//
// tel may be nil; a no-op sink is used in that case.
func BuildPromptRegistry(workspacePath string, tel telemetry.Telemetry) (prompt.Registry, error) {
	var registry prompt.Registry
	if tel != nil {
		registry = prompt.NewRegistryWithTelemetry(promptTelemetryAdapter{inner: tel})
	} else {
		registry = prompt.NewRegistry()
	}

	// Load templates tree first (distributed with project)
	// Load from framework subdirectory
	frameworkDir := filepath.Join(workspacePath, "templates", "prompts", "framework")
	if err := registry.LoadDir(frameworkDir); err != nil {
		return nil, fmt.Errorf("load framework prompts: %w", err)
	}

	// Load from agents subdirectory
	agentsDir := filepath.Join(workspacePath, "templates", "prompts", "agents")
	if err := registry.LoadDir(agentsDir); err != nil {
		return nil, fmt.Errorf("load agent prompts: %w", err)
	}

	// Load from named subdirectory
	namedDir := filepath.Join(workspacePath, "templates", "prompts", "named")
	if err := registry.LoadDir(namedDir); err != nil {
		return nil, fmt.Errorf("load named prompts: %w", err)
	}

	// Load workspace tree second (user-specific prompts)
	promptDir := filepath.Join(workspacePath, "relurpify_cfg", "prompts")
	if err := registry.LoadDir(promptDir); err != nil {
		return nil, fmt.Errorf("load workspace prompts: %w", err)
	}

	return registry, nil
}

// promptTelemetryAdapter wraps telemetry.Telemetry to satisfy prompt.PromptTelemetry.
type promptTelemetryAdapter struct {
	inner telemetry.Telemetry
}

func (a promptTelemetryAdapter) EmitPromptResolved(e prompt.ResolvedEvent) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:    telemetry.EventType("prompt.resolved"),
		TaskID:  e.ID,
		Message: fmt.Sprintf("prompt %s resolved: %d chars, %d blocks", e.ID, e.OutputLength, e.BlocksIncluded),
		Metadata: map[string]any{
			"paradigm":        e.Paradigm,
			"blocks_included": e.BlocksIncluded,
			"blocks_excluded": e.BlocksExcluded,
			"providers_used":  e.ProvidersUsed,
			"duration_ms":     e.DurationMs,
			"cache_hit":       e.CacheHit,
		},
	})
}

func (a promptTelemetryAdapter) EmitPromptResolveFailed(e prompt.ResolveFailedEvent) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:    telemetry.EventType("prompt.resolve_failed"),
		TaskID:  e.ID,
		Message: fmt.Sprintf("prompt %s resolve failed: %s", e.ID, e.Error),
		Metadata: map[string]any{
			"paradigm":    e.Paradigm,
			"error":       e.Error,
			"duration_ms": e.DurationMs,
		},
	})
}

func (a promptTelemetryAdapter) EmitPromptContextMissing(e prompt.ContextMissingEvent) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:    telemetry.EventType("prompt.context_missing"),
		TaskID:  e.PromptID,
		Message: fmt.Sprintf("prompt %s/%s: %s", e.PromptID, e.BlockID, e.Message),
		Metadata: map[string]any{
			promptBlockIDKey: e.BlockID,
			"key":            e.Key,
		},
	})
}

func (a promptTelemetryAdapter) EmitPromptValidationIssue(e prompt.ValidationIssueEvent) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:    telemetry.EventType("prompt.validation_issue"),
		TaskID:  e.Issue.PromptID,
		Message: e.Issue.Error(),
		Metadata: map[string]any{
			"severity":       e.Issue.Severity.String(),
			promptBlockIDKey: e.Issue.BlockID,
		},
	})
}

func (a promptTelemetryAdapter) EmitPromptProviderFailed(e prompt.ProviderFailedEvent) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(telemetry.Event{
		Type:    telemetry.EventType("prompt.provider_failed"),
		TaskID:  e.PromptID,
		Message: fmt.Sprintf("prompt %s/%s provider %s failed: %s", e.PromptID, e.BlockID, e.ProviderName, e.Error),
		Metadata: map[string]any{
			promptBlockIDKey: e.BlockID,
			"provider_name":  e.ProviderName,
			"error":          e.Error,
		},
	})
}
