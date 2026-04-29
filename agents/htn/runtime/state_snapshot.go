package runtime

import (
	"encoding/json"
	"fmt"

	"codeburg.org/lexbit/relurpify/agents/plan"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// LoadStateFromEnvelope reconstructs the canonical HTN state snapshot from
// namespaced envelope working memory keys and validates it.
func LoadStateFromEnvelope(env *contextdata.Envelope) (*HTNState, bool, error) {
	if env == nil {
		return nil, false, nil
	}
	snapshot := HTNState{SchemaVersion: htnSchemaVersion}
	loaded := false
	if raw, ok := env.GetWorkingValue(contextKeyTask); ok {
		if decodeContextValue(raw, &snapshot.Task) {
			loaded = true
		}
	}
	if raw, ok := env.GetWorkingValue(contextKeySelectedMethod); ok {
		if decodeContextValue(raw, &snapshot.Method) {
			loaded = true
		}
	}
	if raw, ok := env.GetWorkingValue(contextKeyPlan); ok {
		var planValue plan.Plan
		if decodeContextValue(raw, &planValue) {
			snapshot.Plan = clonePlan(&planValue)
			loaded = true
		}
	}
	snapshot.Execution = loadExecutionState(env)
	if raw, ok := env.GetWorkingValue(contextKeyMetrics); ok {
		_ = decodeContextValue(raw, &snapshot.Metrics)
	}
	if raw, ok := env.GetWorkingValue(contextKeyPreflightReport); ok && raw != nil {
		var report graph.PreflightReport
		if decodeContextValue(raw, &report) {
			snapshot.Preflight.Report = &report
		}
	}
	if raw, ok := env.GetWorkingValue(contextKeyPreflightError); ok {
		if s, ok := raw.(string); ok {
			snapshot.Preflight.Error = s
		}
	}
	if raw, ok := env.GetWorkingValue(contextKeyRetrievalApplied); ok {
		_ = decodeContextValue(raw, &snapshot.RetrievalApplied)
	}
	if raw, ok := env.GetWorkingValue(contextKeyResumeCheckpointID); ok {
		if s, ok := raw.(string); ok {
			snapshot.ResumeCheckpointID = s
		}
	}
	if raw, ok := env.GetWorkingValue(contextKeyTermination); ok {
		if s, ok := raw.(string); ok {
			snapshot.Termination = s
		}
	}
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

// mustPublishHTNState publishes the current HTN state snapshot to envelope working memory.
func mustPublishHTNState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	snapshot, loaded, err := LoadStateFromEnvelope(env)
	if err != nil {
		env.SetWorkingValue(contextKeyStateError, err.Error(), contextdata.MemoryClassTask)
		return
	}
	if !loaded || snapshot == nil {
		return
	}
	env.SetWorkingValue(contextKeyState, *snapshot, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKeyStateError, "", contextdata.MemoryClassTask)
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
func validatePlanShape(p *plan.Plan) error {
	if p == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(p.Steps))
	for _, step := range p.Steps {
		if step.ID == "" {
			return fmt.Errorf("htn: plan step id required")
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("htn: duplicate plan step id %q", step.ID)
		}
		seen[step.ID] = struct{}{}
	}
	for stepID, deps := range p.Dependencies {
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
func clonePlan(p *plan.Plan) *plan.Plan {
	if p == nil {
		return nil
	}
	cloned := *p
	cloned.Steps = append([]plan.PlanStep(nil), p.Steps...)
	if p.Dependencies != nil {
		cloned.Dependencies = make(map[string][]string, len(p.Dependencies))
		for key, deps := range p.Dependencies {
			cloned.Dependencies[key] = append([]string(nil), deps...)
		}
	}
	cloned.Files = append([]string(nil), p.Files...)
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
	subtaskCount := 0
	if resolved.Method != nil {
		subtaskCount = len(resolved.Method.Subtasks)
	} else if resolved.Spec.SubtaskCount != 0 {
		subtaskCount = resolved.Spec.SubtaskCount
	}
	return MethodState{
		Name:                 resolved.Spec.Name,
		TaskType:             resolved.Spec.TaskType,
		Priority:             resolved.Spec.Priority,
		SubtaskCount:         subtaskCount,
		OperatorCount:        len(resolved.Operators),
		RequiredCapabilities: dedupeSelectors(resolved.Spec.RequiredCapabilities),
	}
}
