package intake

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

// NormalizeEnvelope normalizes a task into a TaskEnvelope.
// This is extracted from runtime/classification.go NormalizeTaskEnvelope.
func NormalizeEnvelope(task *core.Task, state *core.Context, registry *capability.Registry) eucloruntime.TaskEnvelope {
	envelope := eucloruntime.TaskEnvelope{
		EditPermitted:      true,
		CapabilitySnapshot: snapshotCapabilities(registry),
	}
	if task == nil {
		envelope.EditPermitted = envelope.CapabilitySnapshot.HasWriteTools
		return envelope
	}
	envelope.TaskID = task.ID
	envelope.Instruction = strings.TrimSpace(task.Instruction)
	if task.Type != "" {
		envelope.TaskType = task.Type
	} else {
		envelope.TaskType = core.TaskTypeAnalysis
	}
	if task.Context != nil {
		envelope.Workspace = stringValue(task.Context["workspace"])
		envelope.ModeHint = normalizedModeHint(
			task.Context["euclo.mode"],
			task.Context["mode"],
			task.Context["mode_hint"],
		)
		envelope.ExplicitVerification = strings.TrimSpace(fmt.Sprint(task.Context["verification"]))
		if envelope.ExplicitVerification == "<nil>" {
			envelope.ExplicitVerification = ""
		}
	}
	if state != nil {
		envelope.ResumedMode = resumedModeFromState(state)
		envelope.PreviousArtifactKinds = previousArtifactKinds(state)
		if seq, ok := euclostate.GetPreClassifiedCapabilitySequence(state); ok && len(seq) > 0 {
			envelope.CapabilitySequence = append([]string(nil), seq...)
		}
		if src, ok := euclostate.GetClassificationSource(state); ok && src != "" {
			envelope.CapabilityClassificationSource = src
		}
		if meta, ok := euclostate.GetClassificationMeta(state); ok && meta != "" {
			envelope.CapabilityClassificationMeta = meta
		}
		if op, ok := euclostate.GetCapabilitySequenceOperator(state); ok && op != "" {
			envelope.CapabilitySequenceOperator = op
		}

		// Load user recipe signals from state
		envelope.UserRecipes = loadUserRecipeSignals(state)
	}
	envelope.EditPermitted = envelope.CapabilitySnapshot.HasWriteTools
	return envelope
}

// resumedModeFromState extracts the resumed mode from state.
func resumedModeFromState(state *core.Context) string {
	if state == nil {
		return ""
	}
	if mode, ok := euclostate.GetMode(state); ok && mode != "" {
		return mode
	}
	// Check interaction state
	if raw, ok := euclostate.GetInteractionState(state); ok && raw != nil {
		switch typed := raw.(type) {
		case map[string]any:
			return normalizedModeHint(typed["mode"])
		}
	}
	return ""
}

// previousArtifactKinds extracts previous artifact kinds from state.
func previousArtifactKinds(state *core.Context) []string {
	if state == nil {
		return nil
	}
	artifacts, ok := euclostate.GetArtifacts(state)
	if !ok || len(artifacts) == 0 {
		return nil
	}
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Kind == "" {
			continue
		}
		out = append(out, string(artifact.Kind))
	}
	return out
}

// loadUserRecipeSignals loads user recipe signals from state.
func loadUserRecipeSignals(state *core.Context) []eucloruntime.UserRecipeSignalSource {
	if state == nil {
		return nil
	}
	signals, ok := euclostate.GetUserRecipeSignals(state)
	if !ok || len(signals) == 0 {
		return nil
	}
	return signals
}

// snapshotCapabilities creates a capability snapshot from the registry.
func snapshotCapabilities(registry *capability.Registry) euclotypes.CapabilitySnapshot {
	snapshot := euclotypes.CapabilitySnapshot{}
	if registry == nil {
		return snapshot
	}
	tools := registry.ModelCallableTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			continue
		}
		names = append(names, name)
		perms := tool.Permissions().Permissions
		if perms != nil {
			if len(perms.Network) > 0 {
				snapshot.HasNetworkTools = true
			}
			if len(perms.Executables) > 0 {
				snapshot.HasExecuteTools = true
			}
			for _, fs := range perms.FileSystem {
				switch fs.Action {
				case core.FileSystemRead, core.FileSystemList:
					snapshot.HasReadTools = true
				case core.FileSystemWrite:
					snapshot.HasWriteTools = true
				case core.FileSystemExecute:
					snapshot.HasExecuteTools = true
				}
			}
		}
		lower := strings.ToLower(name)
		if containsAny(lower, "test", "build", "lint", "verify") {
			snapshot.HasVerificationTools = true
		}
		if containsAny(lower, "ast", "lsp") {
			snapshot.HasASTOrLSPTools = true
		}
	}
	if len(names) > 0 {
		sort.Strings(names)
		snapshot.ToolNames = names
	}
	return snapshot
}

// Helper functions

func normalizedModeHint(values ...any) string {
	for _, value := range values {
		mode := strings.TrimSpace(strings.ToLower(fmt.Sprint(value)))
		if mode == "" || mode == "<nil>" {
			continue
		}
		return mode
	}
	return ""
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

func containsAny(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}
