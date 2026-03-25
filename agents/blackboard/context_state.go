package blackboard

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const (
	contextKeyGoals                      = "blackboard.goals"
	contextKeyFacts                      = "blackboard.facts"
	contextKeyHypotheses                 = "blackboard.hypotheses"
	contextKeyIssues                     = "blackboard.issues"
	contextKeyPendingActions             = "blackboard.pending_actions"
	contextKeyCompletedActions           = "blackboard.completed_actions"
	contextKeyArtifacts                  = "blackboard.artifacts"
	contextKeyController                 = "blackboard.controller"
	contextKeyControllerNext             = "blackboard.controller.next"
	contextKeyControllerCycle            = "blackboard.controller.cycle"
	contextKeyControllerEligible         = "blackboard.controller.eligible"
	contextKeyControllerLastError        = "blackboard.controller.last_error"
	contextKeyControllerSelectedSpec     = "blackboard.controller.selected_spec"
	contextKeyControllerSelectedContract = "blackboard.controller.selected_contract"
	contextKeyControllerContenders       = "blackboard.controller.contenders"
	contextKeyControllerExecutionMode    = "blackboard.controller.execution_mode"
	contextKeyControllerSelectionPolicy  = "blackboard.controller.selection_policy"
	contextKeyControllerMergePolicy      = "blackboard.controller.merge_policy"
	contextKeyRuntimeActive              = "blackboard.runtime.active"
	contextKeyResumeCheckpointID         = "blackboard.resume_checkpoint_id"
	contextKeyResumeLatest               = "blackboard.resume_latest"
	contextKeyMetrics                    = "blackboard.metrics"
	contextKeySummary                    = "blackboard.summary"
	contextKeySummaryRef                 = "blackboard.summary_ref"
	contextKeySummaryArtifactSummary     = "blackboard.summary_artifact_summary"
	contextKeyTermination                = "blackboard.termination_reason"
	contextKeyPersistenceSummary         = "blackboard.persistence.summary"
	contextKeyPersistenceDecision        = "blackboard.persistence.decision"
	contextKeyPersistenceRoutine         = "blackboard.persistence.routine"
	contextKeyAuditTrail                 = "blackboard.audit"
	contextKeyExecutionSummary           = "blackboard.execution_summary"
	contextKeyCheckpointRef              = "blackboard.checkpoint_ref"
	contextKnowledgeSummary              = "blackboard.summary"
	contextKnowledgeTermination          = "blackboard.termination_reason"
	contextKnowledgeLastSource           = "blackboard.last_source"
	contextKnowledgeLastSourcePriority   = "blackboard.last_source_priority"
	contextKnowledgeGoalSatisfied        = "blackboard.goal_satisfied"
	contextKnowledgeCounts               = "blackboard.counts"
)

// LoadFromContext hydrates a blackboard from namespaced core.Context keys.
func LoadFromContext(state *core.Context, goal string) *Blackboard {
	bb := NewBlackboard(goal)
	if state == nil {
		return bb
	}
	if loaded, ok := loadNamespacedBlackboard(state); ok {
		if len(loaded.Goals) == 0 && goal != "" {
			loaded.Goals = []string{goal}
		}
		return loaded
	}
	return bb
}

// PublishToContext writes the current blackboard into namespaced context keys.
func PublishToContext(state *core.Context, bb *Blackboard, controller ControllerState) {
	if state == nil || bb == nil {
		return
	}
	bb.Normalize()
	if err := bb.Validate(); err != nil {
		panic(fmt.Sprintf("blackboard: invalid state during context publish: %v", err))
	}
	snapshot := bb.Clone()
	metrics := snapshot.Metrics()
	controller.GoalSatisfied = snapshot.IsGoalSatisfied()
	controller.LastUpdatedAt = time.Now().UTC()

	// Stamp blackboard_inject derivation on facts
	facts := make([]Fact, 0, len(snapshot.Facts))
	for _, fact := range snapshot.Facts {
		if fact.Derivation == nil {
			origin := core.OriginDerivation("blackboard")
			derived := origin.Derive("blackboard_inject", "blackboard", 0.0, "")
			fact.Derivation = &derived
		}
		facts = append(facts, fact)
	}

	// Stamp blackboard_inject derivation on hypotheses
	hypotheses := make([]Hypothesis, 0, len(snapshot.Hypotheses))
	for _, hyp := range snapshot.Hypotheses {
		if hyp.Derivation == nil {
			origin := core.OriginDerivation("blackboard")
			derived := origin.Derive("blackboard_inject", "blackboard", 0.0, "")
			hyp.Derivation = &derived
		}
		hypotheses = append(hypotheses, hyp)
	}

	// Stamp blackboard_inject derivation on issues
	issues := make([]Issue, 0, len(snapshot.Issues))
	for _, issue := range snapshot.Issues {
		if issue.Derivation == nil {
			origin := core.OriginDerivation("blackboard")
			derived := origin.Derive("blackboard_inject", "blackboard", 0.0, "")
			issue.Derivation = &derived
		}
		issues = append(issues, issue)
	}

	state.Set(contextKeyGoals, append([]string(nil), snapshot.Goals...))
	state.Set(contextKeyFacts, facts)
	state.Set(contextKeyHypotheses, hypotheses)
	state.Set(contextKeyIssues, issues)
	state.Set(contextKeyPendingActions, append([]ActionRequest(nil), snapshot.PendingActions...))
	state.Set(contextKeyCompletedActions, append([]ActionResult(nil), snapshot.CompletedActions...))
	state.Set(contextKeyArtifacts, append([]Artifact(nil), snapshot.Artifacts...))
	state.Set(contextKeyController, controller)
	state.Set(contextKeyControllerExecutionMode, string(ExecutionModeSingleFireSerial))
	state.Set(contextKeyControllerSelectionPolicy, "highest_priority_then_name")
	state.Set(contextKeyControllerMergePolicy, string(BranchMergePolicyRejectConflicts))
	state.Set(contextKeyMetrics, metrics)
	state.Set(contextKeySummary, summaryText(snapshot, metrics))
	state.Set(contextKeyTermination, controller.Termination)
	state.Set(contextKeyExecutionSummary, executionSummary(snapshot, controller, metrics))
	publishPersistenceCandidates(state, snapshot, controller, metrics)

	state.SetKnowledge(contextKnowledgeSummary, summaryText(snapshot, metrics))
	state.SetKnowledge(contextKnowledgeTermination, controller.Termination)
	state.SetKnowledge(contextKnowledgeLastSource, controller.LastSource)
	state.SetKnowledge(contextKnowledgeGoalSatisfied, controller.GoalSatisfied)
	state.SetKnowledge(contextKnowledgeCounts, map[string]any{
		"goals":      metrics.GoalCount,
		"facts":      metrics.FactCount,
		"issues":     metrics.IssueCount,
		"pending":    metrics.PendingCount,
		"completed":  metrics.CompletedCount,
		"artifacts":  metrics.ArtifactCount,
		"verified":   metrics.VerifiedCount,
		"hypothesis": metrics.HypothesisCount,
	})
}

func loadNamespacedBlackboard(state *core.Context) (*Blackboard, bool) {
	if state == nil {
		return nil, false
	}
	loaded := false
	bb := &Blackboard{}
	if goals, ok := state.Get(contextKeyGoals); ok {
		var typed []string
		if decodeContextValue(goals, &typed) {
			bb.Goals = append([]string(nil), typed...)
			loaded = true
		}
	}
	if facts, ok := state.Get(contextKeyFacts); ok {
		var typed []Fact
		if decodeContextValue(facts, &typed) {
			bb.Facts = append([]Fact(nil), typed...)
			loaded = true
		}
	}
	if hypotheses, ok := state.Get(contextKeyHypotheses); ok {
		var typed []Hypothesis
		if decodeContextValue(hypotheses, &typed) {
			bb.Hypotheses = append([]Hypothesis(nil), typed...)
			loaded = true
		}
	}
	if issues, ok := state.Get(contextKeyIssues); ok {
		var typed []Issue
		if decodeContextValue(issues, &typed) {
			bb.Issues = append([]Issue(nil), typed...)
			loaded = true
		}
	}
	if pending, ok := state.Get(contextKeyPendingActions); ok {
		var typed []ActionRequest
		if decodeContextValue(pending, &typed) {
			bb.PendingActions = append([]ActionRequest(nil), typed...)
			loaded = true
		}
	}
	if completed, ok := state.Get(contextKeyCompletedActions); ok {
		var typed []ActionResult
		if decodeContextValue(completed, &typed) {
			bb.CompletedActions = append([]ActionResult(nil), typed...)
			loaded = true
		}
	}
	if artifacts, ok := state.Get(contextKeyArtifacts); ok {
		var typed []Artifact
		if decodeContextValue(artifacts, &typed) {
			bb.Artifacts = append([]Artifact(nil), typed...)
			loaded = true
		}
	}
	if !loaded {
		return nil, false
	}
	bb.Normalize()
	return bb, true
}

func summaryText(bb *Blackboard, metrics Metrics) string {
	if bb == nil {
		return ""
	}
	goal := ""
	if len(bb.Goals) > 0 {
		goal = bb.Goals[0]
	}
	return fmt.Sprintf(
		"goal=%q facts=%d issues=%d pending=%d completed=%d artifacts=%d verified=%d",
		goal,
		metrics.FactCount,
		metrics.IssueCount,
		metrics.PendingCount,
		metrics.CompletedCount,
		metrics.ArtifactCount,
		metrics.VerifiedCount,
	)
}

func decodeContextValue(raw any, target any) bool {
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

func executionSummary(bb *Blackboard, controller ControllerState, metrics Metrics) map[string]any {
	goal := ""
	if bb != nil && len(bb.Goals) > 0 {
		goal = bb.Goals[0]
	}
	return map[string]any{
		"goal":           goal,
		"termination":    controller.Termination,
		"cycle":          controller.Cycle,
		"max_cycles":     controller.MaxCycles,
		"last_source":    controller.LastSource,
		"goal_satisfied": controller.GoalSatisfied,
		"counts": map[string]any{
			"facts":      metrics.FactCount,
			"issues":     metrics.IssueCount,
			"pending":    metrics.PendingCount,
			"completed":  metrics.CompletedCount,
			"artifacts":  metrics.ArtifactCount,
			"verified":   metrics.VerifiedCount,
			"hypothesis": metrics.HypothesisCount,
		},
	}
}
