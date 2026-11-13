package euclotui

import "codeburg.org/lexbit/relurpify/app/relurpish/tui"

var _ tui.ExecutionReader = (*EucloSurface)(nil)

// ExecutionSnapshot adapts existing Euclo facts to the shared execution canvas.
// This path intentionally does not manufacture call IDs, prompt provenance, or
// occurrence history: the current projection has none of those contracts.
func (s *EucloSurface) ExecutionSnapshot() tui.CircuitSnapshot {
	observed := s.router.Snapshot()
	if observed.Recipe == nil {
		return tui.CircuitSnapshot{Notice: observed.ProjectionError}
	}
	recipe := observed.Recipe
	result := tui.CircuitSnapshot{
		RecipeID: recipe.RecipeID, Name: recipe.Name, Route: recipe.SelectedRoute,
		Notice: "Latest observed step state. Call context, prompt use, and occurrence history are not captured by this projection.",
	}
	for _, step := range recipe.Steps {
		runtime := observed.StepRuntime[step.StepID]
		result.Steps = append(result.Steps, tui.CircuitStep{
			ID: step.StepID, Strategy: step.Paradigm, Goal: step.Goal,
			Capability: step.CapabilityID, Dependencies: step.DependsOn, ToolScopes: step.ToolScopes,
			Group: step.GroupID, Gate: step.HITL, Status: string(runtime.Status),
			DurationMS: runtime.DurationMs, Error: runtime.Err,
		})
	}
	return result
}
