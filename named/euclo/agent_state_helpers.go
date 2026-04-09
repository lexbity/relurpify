package euclo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime"
	euclopretask "github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

func enrichBundleWithContextKnowledge(bundle runtime.SemanticInputBundle, state *core.Context) runtime.SemanticInputBundle {
	if state == nil {
		return bundle
	}
	raw, ok := state.Get("context.knowledge_items")
	if !ok || raw == nil {
		return bundle
	}
	items, ok := raw.([]euclopretask.KnowledgeEvidenceItem)
	if !ok || len(items) == 0 {
		return bundle
	}
	for _, item := range items {
		finding := runtime.SemanticFindingSummary{
			RefID:       item.RefID,
			Kind:        "context_retrieved_" + string(item.Kind),
			Status:      "retrieved",
			Title:       item.Title,
			Summary:     item.Summary,
			RelatedRefs: append([]string(nil), item.RelatedRefs...),
		}
		bundle.PatternFindings = append(bundle.PatternFindings, finding)
	}
	return bundle
}

func seedInteractionPrepass(state *core.Context, task *core.Task, classification runtime.TaskClassification, mode euclotypes.ModeResolution) {
	if state == nil {
		return
	}
	instruction := ""
	if task != nil {
		instruction = strings.ToLower(strings.TrimSpace(task.Instruction))
	}
	state.Set("requires_evidence_before_mutation", classification.RequiresEvidenceBeforeMutation)
	switch mode.ModeID {
	case "debug":
		if hasInstructionEvidence(instruction, classification.ReasonCodes) {
			state.Set("has_evidence", true)
			state.Set("evidence_in_instruction", true)
		}
	case "code":
		if strings.Contains(instruction, "just do it") {
			state.Set("just_do_it", true)
		}
	case "planning":
		if strings.Contains(instruction, "just plan it") || strings.Contains(instruction, "skip to plan") {
			state.Set("just_plan_it", true)
		}
	}
}

func hasInstructionEvidence(instruction string, reasonCodes []string) bool {
	for _, reason := range reasonCodes {
		if strings.HasPrefix(reason, "error_text:") {
			return true
		}
	}
	for _, token := range []string{"panic:", "stacktrace", "stack trace", "goroutine ", ".go:", "failing test", "runtime error"} {
		if strings.Contains(instruction, token) {
			return true
		}
	}
	return false
}

func interactionEmitterForTask(task *core.Task) (interaction.FrameEmitter, bool) {
	script := interactionScriptFromTask(task)
	if len(script) == 0 {
		return &interaction.NoopEmitter{}, false
	}
	return interaction.NewTestFrameEmitter(script...), true
}

func interactionScriptFromTask(task *core.Task) []interaction.ScriptedResponse {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["euclo.interaction_script"]
	if !ok || raw == nil {
		return nil
	}
	rows, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			rows = make([]any, 0, len(typed))
			for _, item := range typed {
				rows = append(rows, item)
			}
		} else {
			return nil
		}
	}
	script := make([]interaction.ScriptedResponse, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			continue
		}
		action := stringValue(entry["action"])
		if action == "" {
			continue
		}
		script = append(script, interaction.ScriptedResponse{
			Phase:    stringValue(entry["phase"]),
			Kind:     stringValue(entry["kind"]),
			ActionID: action,
			Text:     stringValue(entry["text"]),
		})
	}
	return script
}

func interactionMaxTransitions(task *core.Task) int {
	if task == nil || task.Context == nil {
		return 5
	}
	raw, ok := task.Context["euclo.max_interactive_transitions"]
	if !ok || raw == nil {
		return 5
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 5
}

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (runtime.TaskEnvelope, runtime.TaskClassification, euclotypes.ModeResolution, euclotypes.ExecutionProfileSelection, runtime.UnitOfWork) {
	envelope := runtime.NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := runtime.ClassifyTask(envelope)
	mode := runtime.ResolveMode(envelope, classification, a.ModeRegistry)
	profile := runtime.SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	skillPolicy := runtime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), mode, profile)
	semanticInputs := a.semanticInputBundle(task, state, mode)
	work := euclowork.BuildUnitOfWork(task, state, envelope, classification, mode, profile, a.ModeRegistry, semanticInputs, skillPolicy, runtime.WorkUnitExecutorDescriptor{})
	return envelope, classification, mode, profile, work
}

func (a *Agent) seedRuntimeState(state *core.Context, envelope runtime.TaskEnvelope, classification runtime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection, work runtime.UnitOfWork) {
	if state == nil {
		return
	}
	history := []runtime.UnitOfWorkHistoryEntry(nil)
	if raw, ok := state.Get("euclo.unit_of_work_history"); ok && raw != nil {
		if typed, ok := raw.([]runtime.UnitOfWorkHistoryEntry); ok {
			history = append(history, typed...)
		}
	}
	if len(history) == 0 {
		if raw, ok := state.Get("euclo.unit_of_work"); ok && raw != nil {
			if existing, ok := raw.(runtime.UnitOfWork); ok && existing.ID != "" {
				history = runtime.UpdateUnitOfWorkHistory(history, existing, existing.UpdatedAt)
			}
		}
	}
	state.Set("euclo.envelope", envelope)
	state.Set("euclo.classification", classification)
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.mode", mode.ModeID)
	state.Set("euclo.execution_profile", profile.ProfileID)
	state.Set("euclo.semantic_inputs", work.SemanticInputs)
	state.Set("euclo.resolved_execution_policy", work.ResolvedPolicy)
	state.Set("euclo.executor_descriptor", work.ExecutorDescriptor)
	state.Set("euclo.unit_of_work", work)
	state.Set("euclo.unit_of_work_id", work.ID)
	state.Set("euclo.root_unit_of_work_id", work.RootID)
	state.Set("euclo.unit_of_work_transition", work.TransitionState)
	state.Set("euclo.unit_of_work_history", runtime.UpdateUnitOfWorkHistory(history, work, work.UpdatedAt))
}

func (a *Agent) ensureWorkflowRun(ctx context.Context, task *core.Task, state *core.Context) {
	if a == nil || state == nil {
		return
	}
	store := a.workflowStore()
	if store == nil {
		return
	}
	_, _, _ = euclorestore.EnsureWorkflowRun(ctx, store, task, state)
}

func (a *Agent) ensureDeferralPlan(task *core.Task, state *core.Context) {
	if a == nil || a.GuidanceBroker == nil {
		return
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		workflowID = "session"
	}
	if a.DeferralPlan == nil || a.DeferralPlan.WorkflowID != workflowID {
		now := time.Now().UTC()
		a.DeferralPlan = &guidance.DeferralPlan{
			ID:         fmt.Sprintf("deferral-%d", now.UnixNano()),
			WorkflowID: workflowID,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	a.GuidanceBroker.SetDeferralPlan(a.DeferralPlan)
}
