package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// LoadStateFromContext reconstructs the canonical HTN state snapshot from
// namespaced context keys and validates it.
func LoadStateFromContext(state *core.Context) (*HTNState, bool, error) {
	if state == nil {
		return nil, false, nil
	}
	snapshot := HTNState{SchemaVersion: htnSchemaVersion}
	loaded := false
	if raw, ok := state.Get(contextKeyTask); ok {
		if decodeContextValue(raw, &snapshot.Task) {
			loaded = true
		}
	}
	if raw, ok := state.Get(contextKeySelectedMethod); ok {
		if decodeContextValue(raw, &snapshot.Method) {
			loaded = true
		}
	}
	if raw, ok := state.Get(contextKeyPlan); ok {
		var plan core.Plan
		if decodeContextValue(raw, &plan) {
			snapshot.Plan = clonePlan(&plan)
			loaded = true
		}
	}
	snapshot.Execution = loadExecutionState(state)
	if raw, ok := state.Get(contextKeyMetrics); ok {
		_ = decodeContextValue(raw, &snapshot.Metrics)
	}
	if raw, ok := state.Get(contextKeyPreflightReport); ok && raw != nil {
		var report graph.PreflightReport
		if decodeContextValue(raw, &report) {
			snapshot.Preflight.Report = &report
		}
	}
	snapshot.Preflight.Error = state.GetString(contextKeyPreflightError)
	if raw, ok := state.Get(contextKeyRetrievalApplied); ok {
		_ = decodeContextValue(raw, &snapshot.RetrievalApplied)
	}
	snapshot.ResumeCheckpointID = state.GetString(contextKeyResumeCheckpointID)
	snapshot.Termination = state.GetString(contextKeyTermination)
	if !loaded && len(snapshot.Execution.CompletedSteps) == 0 && snapshot.Termination == "" && !snapshot.RetrievalApplied {
		return nil, false, nil
	}
	normalizeHTNState(&snapshot)
	if err := snapshot.Validate(); err != nil {
		return nil, true, err
	}
	return &snapshot, true, nil
}

// Validate enforces basic HTN state invariants for persistence and resume.
func (s *HTNState) Validate() error {
	if s == nil {
		return nil
	}
	if s.SchemaVersion == 0 {
		return fmt.Errorf("htn: schema version required")
	}
	if s.Task.Type == "" && s.Method.Name != "" {
		return fmt.Errorf("htn: method selected without resolved task type")
	}
	if s.Method.Name != "" && s.Method.TaskType != "" && s.Task.Type != "" && s.Method.TaskType != s.Task.Type {
		return fmt.Errorf("htn: method %q task type %q does not match task type %q", s.Method.Name, s.Method.TaskType, s.Task.Type)
	}
	if s.Plan != nil {
		if err := validatePlanShape(s.Plan); err != nil {
			return err
		}
		if s.Method.Name != "" {
			expectedPrefix := s.Method.Name + "."
			for _, step := range s.Plan.Steps {
				if step.ID == "" {
					return fmt.Errorf("htn: plan contains step with empty id")
				}
				if len(expectedPrefix) > 1 && len(step.ID) >= len(expectedPrefix) && step.ID[:len(expectedPrefix)] != expectedPrefix {
					return fmt.Errorf("htn: step %q does not belong to selected method %q", step.ID, s.Method.Name)
				}
			}
		}
	}
	seen := make(map[string]struct{}, len(s.Execution.CompletedSteps))
	for _, stepID := range s.Execution.CompletedSteps {
		if stepID == "" {
			return fmt.Errorf("htn: completed step id cannot be empty")
		}
		if _, ok := seen[stepID]; ok {
			return fmt.Errorf("htn: duplicate completed step id %q", stepID)
		}
		seen[stepID] = struct{}{}
	}
	if s.Plan != nil {
		validSteps := make(map[string]struct{}, len(s.Plan.Steps))
		for _, step := range s.Plan.Steps {
			validSteps[step.ID] = struct{}{}
		}
		for _, stepID := range s.Execution.CompletedSteps {
			if _, ok := validSteps[stepID]; !ok {
				return fmt.Errorf("htn: completed step %q not present in plan", stepID)
			}
		}
	}
	if s.Execution.CompletedStepCount != len(s.Execution.CompletedSteps) {
		return fmt.Errorf("htn: completed step count mismatch")
	}
	if s.Execution.PlannedStepCount > 0 && s.Execution.PlannedStepCount < s.Execution.CompletedStepCount {
		return fmt.Errorf("htn: completed step count exceeds planned steps")
	}
	if s.Plan != nil && s.Execution.PlannedStepCount != len(s.Plan.Steps) {
		return fmt.Errorf("htn: planned step count mismatch")
	}
	if s.Execution.Resumed && s.Execution.ResumeCheckpointID == "" && s.ResumeCheckpointID == "" {
		return fmt.Errorf("htn: resumed execution missing checkpoint id")
	}
	return nil
}

// mustPublishHTNState publishes the current HTN state snapshot to context.
func mustPublishHTNState(state *core.Context) {
	if state == nil {
		return
	}
	snapshot, loaded, err := LoadStateFromContext(state)
	if err != nil {
		state.Set(contextKeyStateError, err.Error())
		return
	}
	if !loaded || snapshot == nil {
		return
	}
	state.Set(contextKeyState, *snapshot)
	state.Set(contextKeyStateError, "")
}

// normalizeHTNState ensures all HTN state fields are consistent and complete.
func normalizeHTNState(snapshot *HTNState) {
	if snapshot == nil {
		return
	}
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = htnSchemaVersion
	}
	snapshot.Execution.CompletedSteps = append([]string(nil), snapshot.Execution.CompletedSteps...)
	snapshot.Execution.CompletedStepCount = len(snapshot.Execution.CompletedSteps)
	if snapshot.Execution.CompletedStepCount > 0 && snapshot.Execution.LastCompletedStep == "" {
		snapshot.Execution.LastCompletedStep = snapshot.Execution.CompletedSteps[len(snapshot.Execution.CompletedSteps)-1]
	}
	if snapshot.Plan != nil {
		snapshot.Plan = clonePlan(snapshot.Plan)
		snapshot.Execution.PlannedStepCount = len(snapshot.Plan.Steps)
	}
	snapshot.Metrics = Metrics{
		PlannedStepCount:   snapshot.Execution.PlannedStepCount,
		CompletedStepCount: snapshot.Execution.CompletedStepCount,
	}
	if snapshot.ResumeCheckpointID == "" {
		snapshot.ResumeCheckpointID = snapshot.Execution.ResumeCheckpointID
	}
	if snapshot.Execution.ResumeCheckpointID == "" {
		snapshot.Execution.ResumeCheckpointID = snapshot.ResumeCheckpointID
	}
}

// validatePlanShape checks that plan steps and dependencies are consistent.
func validatePlanShape(plan *core.Plan) error {
	if plan == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" {
			return fmt.Errorf("htn: plan step id required")
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("htn: duplicate plan step id %q", step.ID)
		}
		seen[step.ID] = struct{}{}
	}
	for stepID, deps := range plan.Dependencies {
		if _, ok := seen[stepID]; !ok {
			return fmt.Errorf("htn: dependency declared for unknown step %q", stepID)
		}
		for _, dep := range deps {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("htn: dependency %q for step %q does not exist", dep, stepID)
			}
		}
	}
	return nil
}

// clonePlan creates a deep copy of a plan.
func clonePlan(plan *core.Plan) *core.Plan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.Steps = append([]core.PlanStep(nil), plan.Steps...)
	if plan.Dependencies != nil {
		cloned.Dependencies = make(map[string][]string, len(plan.Dependencies))
		for key, deps := range plan.Dependencies {
			cloned.Dependencies[key] = append([]string(nil), deps...)
		}
	}
	cloned.Files = append([]string(nil), plan.Files...)
	return &cloned
}

// decodeContextValue unmarshals context values for type safety.
func decodeContextValue(raw any, target any) bool {
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

// mapsClone creates a shallow copy of a string map.
func mapsClone(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

// methodStateFromResolved converts a ResolvedMethod to MethodState for serialization.
func methodStateFromResolved(resolved ResolvedMethod) MethodState {
	return MethodState{
		Name:                 resolved.Spec.Name,
		TaskType:             resolved.Spec.TaskType,
		Priority:             resolved.Spec.Priority,
		SubtaskCount:         len(resolved.Method.Subtasks),
		OperatorCount:        len(resolved.Operators),
		RequiredCapabilities: dedupeSelectors(resolved.Spec.RequiredCapabilities),
	}
}
