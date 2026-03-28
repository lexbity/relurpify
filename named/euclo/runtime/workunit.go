package runtime

import (
	"fmt"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
)

// BuildUnitOfWork assembles Euclo's active execution bundle from intake,
// classification, selected mode/profile, and currently available runtime state.
func BuildUnitOfWork(
	task *core.Task,
	state *core.Context,
	envelope TaskEnvelope,
	classification TaskClassification,
	mode ModeResolution,
	profile ExecutionProfileSelection,
	modeRegistry *ModeRegistry,
	semanticInputs SemanticInputBundle,
	resolvedPolicy ResolvedExecutionPolicy,
	executor WorkUnitExecutorDescriptor,
) UnitOfWork {
	now := time.Now().UTC()
	planBinding := planBindingFromState(task, state)
	if planBinding != nil {
		planBinding.IsPlanBacked = true
		if mode.ModeID == "planning" || profile.ProfileID == "plan_stage_execute" {
			planBinding.IsLongRunning = true
		}
	}
	if executor.ExecutorID == "" {
		executor = SelectExecutorDescriptor(mode, profile, classification, resolvedPolicy, planBinding)
	}
	uow := UnitOfWork{
		ID:                   firstNonEmpty(stateString(state, "euclo.unit_of_work_id"), envelope.TaskID),
		WorkflowID:           workflowIDFromTaskState(task, state),
		RunID:                runIDFromTaskState(task, state),
		ExecutionID:          executionID(task, state),
		ModeID:               mode.ModeID,
		ObjectiveKind:        objectiveKindForWork(mode, profile, classification),
		BehaviorFamily:       behaviorFamilyForWork(mode, profile, classification, resolvedPolicy),
		ContextStrategyID:    contextStrategyForWork(mode, modeRegistry),
		VerificationPolicyID: verificationPolicyForWork(mode, profile, resolvedPolicy),
		DeferralPolicyID:     deferralPolicyForWork(mode, profile, classification),
		CheckpointPolicyID:   checkpointPolicyForWork(mode, profile, resolvedPolicy),
		SemanticInputs:       semanticInputs,
		ResolvedPolicy:       resolvedPolicy,
		ExecutorDescriptor:   executor,
		PlanBinding:          planBinding,
		ContextBundle:        contextBundleFromState(task, state, envelope, semanticInputs),
		RoutineBindings:      routineBindingsForWork(mode, profile, classification, resolvedPolicy),
		SkillBindings:        skillBindingsForWork(task, state, resolvedPolicy),
		ToolBindings:         toolBindingsForWork(envelope),
		CapabilityBindings:   capabilityBindingsForWork(profile, resolvedPolicy),
		Status:               UnitOfWorkStatusReady,
		ResultClass:          "",
		DeferredIssueIDs:     deferredIssueIDsFromState(state),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if existing, ok := existingUnitOfWork(state); ok {
		if !existing.CreatedAt.IsZero() {
			uow.CreatedAt = existing.CreatedAt
		}
		if strings.TrimSpace(existing.ID) != "" {
			uow.ID = existing.ID
		}
		if uow.PlanBinding == nil && existing.PlanBinding != nil {
			uow.PlanBinding = clonePlanBinding(existing.PlanBinding)
		}
		if len(uow.SemanticInputs.PatternRefs) == 0 && len(existing.SemanticInputs.PatternRefs) > 0 {
			uow.SemanticInputs = existing.SemanticInputs
		}
		if !uow.ResolvedPolicy.ResolvedFromSkillPolicy && existing.ResolvedPolicy.ResolvedFromSkillPolicy {
			uow.ResolvedPolicy = existing.ResolvedPolicy
		}
		if uow.ExecutorDescriptor.ExecutorID == "" && existing.ExecutorDescriptor.ExecutorID != "" {
			uow.ExecutorDescriptor = existing.ExecutorDescriptor
		}
		if len(uow.ContextBundle.Sources) == 0 && existing.ContextBundle.ContextBudgetClass != "" {
			uow.ContextBundle = existing.ContextBundle
		}
		if len(uow.RoutineBindings) == 0 && len(existing.RoutineBindings) > 0 {
			uow.RoutineBindings = append([]UnitOfWorkRoutineBinding(nil), existing.RoutineBindings...)
		}
		if len(uow.SkillBindings) == 0 && len(existing.SkillBindings) > 0 {
			uow.SkillBindings = append([]UnitOfWorkSkillBinding(nil), existing.SkillBindings...)
		}
		if len(uow.ToolBindings) == 0 && len(existing.ToolBindings) > 0 {
			uow.ToolBindings = append([]UnitOfWorkToolBinding(nil), existing.ToolBindings...)
		}
		if len(uow.CapabilityBindings) == 0 && len(existing.CapabilityBindings) > 0 {
			uow.CapabilityBindings = append([]UnitOfWorkCapabilityBinding(nil), existing.CapabilityBindings...)
		}
		if len(uow.DeferredIssueIDs) == 0 && len(existing.DeferredIssueIDs) > 0 {
			uow.DeferredIssueIDs = append([]string(nil), existing.DeferredIssueIDs...)
		}
	}
	if uow.ID == "" {
		uow.ID = firstNonEmpty(uow.ExecutionID, uow.RunID, uow.WorkflowID, "euclo-work")
	}
	return uow
}

func existingUnitOfWork(state *core.Context) (UnitOfWork, bool) {
	if state == nil {
		return UnitOfWork{}, false
	}
	raw, ok := state.Get("euclo.unit_of_work")
	if !ok || raw == nil {
		return ReconstructUnitOfWorkFromCompiledExecution(state)
	}
	switch typed := raw.(type) {
	case UnitOfWork:
		return typed, true
	case *UnitOfWork:
		if typed != nil {
			return *typed, true
		}
	}
	return ReconstructUnitOfWorkFromCompiledExecution(state)
}

func workflowIDFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
			return workflowID
		}
	}
	if task != nil && task.Context != nil {
		if workflowID := strings.TrimSpace(stringValue(task.Context["workflow_id"])); workflowID != "" {
			return workflowID
		}
	}
	return ""
}

func runIDFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if runID := strings.TrimSpace(state.GetString("euclo.run_id")); runID != "" {
			return runID
		}
	}
	if task != nil && task.Context != nil {
		if runID := strings.TrimSpace(stringValue(task.Context["run_id"])); runID != "" {
			return runID
		}
	}
	return ""
}

func executionID(task *core.Task, state *core.Context) string {
	if state != nil {
		if id := strings.TrimSpace(state.GetString("euclo.execution_id")); id != "" {
			return id
		}
		if runID := strings.TrimSpace(state.GetString("euclo.run_id")); runID != "" {
			return "exec:" + runID
		}
	}
	if task != nil {
		if strings.TrimSpace(task.ID) != "" {
			return "exec:" + strings.TrimSpace(task.ID)
		}
	}
	return ""
}

func objectiveKindForWork(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification) string {
	switch {
	case profile.ProfileID == "plan_stage_execute" || mode.ModeID == "planning":
		return "plan_execution"
	case mode.ModeID == "debug":
		return "investigation"
	case mode.ModeID == "review":
		return "review"
	case profile.ProfileID == "test_driven_generation":
		return "verification_repair"
	case classification.RequiresEvidenceBeforeMutation:
		return "investigation"
	default:
		return "direct_execution"
	}
}

func behaviorFamilyForWork(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy) string {
	if len(policy.ReviewCriteria) > 0 && mode.ModeID == "review" {
		return "tension_assessment"
	}
	if len(policy.PreferredPlanningCapabilities) > 0 && (mode.ModeID == "planning" || profile.ProfileID == "plan_stage_execute") {
		return "gap_analysis"
	}
	if len(policy.PreferredVerifyCapabilities) > 0 && profile.VerificationRequired {
		return "verification_repair"
	}
	switch profile.ProfileID {
	case "edit_verify_repair":
		return "direct_change_execution"
	case "reproduce_localize_patch":
		return "stale_assumption_detection"
	case "test_driven_generation":
		return "verification_repair"
	case "review_suggest_implement":
		return "tension_assessment"
	case "plan_stage_execute":
		return "gap_analysis"
	case "trace_execute_analyze":
		return "tension_assessment"
	}
	if mode.ModeID == "planning" {
		return "gap_analysis"
	}
	if classification.RequiresEvidenceBeforeMutation {
		return "stale_assumption_detection"
	}
	return "direct_change_execution"
}

func contextStrategyForWork(mode ModeResolution, modeRegistry *ModeRegistry) string {
	if modeRegistry != nil {
		if descriptor, ok := modeRegistry.Lookup(mode.ModeID); ok && strings.TrimSpace(descriptor.ContextStrategy) != "" {
			return descriptor.ContextStrategy
		}
	}
	switch mode.ModeID {
	case "planning":
		return "narrow_to_wide"
	case "debug":
		return "localize_then_expand"
	case "review":
		return "read_heavy"
	default:
		return "targeted"
	}
}

func deferralPolicyForWork(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification) string {
	switch {
	case profile.ProfileID == "plan_stage_execute" || mode.ModeID == "planning":
		return "continue_with_artifacted_deferrals"
	case classification.RequiresEvidenceBeforeMutation:
		return "prefer_defer_over_replan"
	default:
		return "defer_when_nonfatal"
	}
}

func verificationPolicyForWork(mode ModeResolution, profile ExecutionProfileSelection, policy ResolvedExecutionPolicy) string {
	if len(policy.PreferredVerifyCapabilities) > 0 || len(policy.VerificationSuccessCapabilities) > 0 {
		return fmt.Sprintf("skill-aware/%s/%s", mode.ModeID, profile.ProfileID)
	}
	return fmt.Sprintf("%s/%s", mode.ModeID, profile.ProfileID)
}

func checkpointPolicyForWork(mode ModeResolution, profile ExecutionProfileSelection, policy ResolvedExecutionPolicy) string {
	switch {
	case policy.RequireVerificationStep:
		return "verification_gated"
	case profile.VerificationRequired:
		return "pre_and_post_verification"
	case mode.ModeID == "planning":
		return "phase_boundary"
	default:
		return "post_execution"
	}
}

func planBindingFromState(task *core.Task, state *core.Context) *UnitOfWorkPlanBinding {
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return nil
	}
	binding := &UnitOfWorkPlanBinding{WorkflowID: workflowID}
	if state != nil {
		if stepID := strings.TrimSpace(state.GetString("euclo.current_plan_step_id")); stepID != "" {
			binding.ActiveStepID = stepID
		}
	}
	if state == nil {
		if binding.ActiveStepID == "" {
			return nil
		}
		return binding
	}
	raw, ok := state.Get("euclo.active_plan_version")
	if !ok || raw == nil {
		if binding.ActiveStepID == "" {
			return nil
		}
		binding.IsPlanBacked = true
		return binding
	}
	switch typed := raw.(type) {
	case *archaeodomain.VersionedLivingPlan:
		populatePlanBinding(binding, typed)
	case archaeodomain.VersionedLivingPlan:
		populatePlanBinding(binding, &typed)
	}
	if binding.PlanID == "" && binding.ActiveStepID == "" {
		return nil
	}
	binding.IsPlanBacked = true
	return binding
}

func populatePlanBinding(binding *UnitOfWorkPlanBinding, version *archaeodomain.VersionedLivingPlan) {
	if binding == nil || version == nil {
		return
	}
	binding.PlanID = strings.TrimSpace(version.Plan.ID)
	binding.PlanVersion = version.Version
	binding.StepIDs = append([]string(nil), version.Plan.StepOrder...)
	if binding.ActiveStepID == "" {
		for _, stepID := range version.Plan.StepOrder {
			if step, ok := version.Plan.Steps[stepID]; ok && step != nil && step.Status == "in_progress" {
				binding.ActiveStepID = stepID
				break
			}
		}
	}
	binding.ArchaeoRefs = map[string][]string{
		"plan_versions": {fmt.Sprintf("%d", version.Version)},
	}
}

func contextBundleFromState(task *core.Task, state *core.Context, envelope TaskEnvelope, semanticInputs SemanticInputBundle) UnitOfWorkContextBundle {
	bundle := UnitOfWorkContextBundle{
		ArtifactKinds:      append([]string(nil), envelope.PreviousArtifactKinds...),
		ContextBudgetClass: contextBudgetClass(envelope),
		PatternRefs:        append([]string(nil), semanticInputs.PatternRefs...),
		TensionRefs:        append([]string(nil), semanticInputs.TensionRefs...),
		ProvenanceRefs:     append([]string(nil), semanticInputs.ProvenanceRefs...),
		LearningRefs:       append([]string(nil), semanticInputs.LearningInteractionRefs...),
	}
	if strings.TrimSpace(envelope.Workspace) != "" {
		bundle.Sources = append(bundle.Sources, UnitOfWorkContextSource{
			Kind:    "workspace",
			Ref:     envelope.Workspace,
			Summary: "workspace root",
		})
	}
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
			bundle.Sources = append(bundle.Sources, UnitOfWorkContextSource{
				Kind:    "workflow",
				Ref:     workflowID,
				Summary: "workflow-backed execution state",
			})
		}
		if raw, ok := state.Get("euclo.pending_learning_ids"); ok {
			bundle.LearningRefs = append(bundle.LearningRefs, stringSliceAny(raw)...)
		}
		if raw, ok := state.Get("euclo.blocking_learning_ids"); ok {
			bundle.LearningRefs = append(bundle.LearningRefs, stringSliceAny(raw)...)
		}
		if raw, ok := state.Get("pipeline.workflow_retrieval"); ok && raw != nil {
			bundle.RetrievalRefs = append(bundle.RetrievalRefs, "pipeline.workflow_retrieval")
			bundle.Sources = append(bundle.Sources, UnitOfWorkContextSource{
				Kind:    "workflow_retrieval",
				Ref:     "pipeline.workflow_retrieval",
				Summary: summarizeMapSummary(raw),
			})
		}
		if raw, ok := state.Get("euclo.active_exploration_id"); ok && raw != nil {
			ref := strings.TrimSpace(fmt.Sprint(raw))
			if ref != "" {
				bundle.PatternRefs = append(bundle.PatternRefs, ref)
			}
		}
		if raw, ok := state.Get("euclo.archaeo_phase_state"); ok && raw != nil {
			bundle.ProvenanceRefs = append(bundle.ProvenanceRefs, "euclo.archaeo_phase_state")
		}
	}
	if task != nil && task.Context != nil {
		for _, key := range []string{"path", "file", "target_path"} {
			if value := strings.TrimSpace(stringValue(task.Context[key])); value != "" {
				bundle.WorkspacePaths = append(bundle.WorkspacePaths, value)
			}
		}
		if raw, ok := task.Context["paths"]; ok {
			bundle.WorkspacePaths = append(bundle.WorkspacePaths, stringSliceAny(raw)...)
		}
	}
	bundle.WorkspacePaths = uniqueStrings(bundle.WorkspacePaths)
	bundle.LearningRefs = uniqueStrings(bundle.LearningRefs)
	bundle.RetrievalRefs = uniqueStrings(bundle.RetrievalRefs)
	bundle.PatternRefs = uniqueStrings(bundle.PatternRefs)
	bundle.TensionRefs = uniqueStrings(bundle.TensionRefs)
	bundle.ProvenanceRefs = uniqueStrings(bundle.ProvenanceRefs)
	bundle.CompactionEligible = bundle.ContextBudgetClass == "heavy"
	bundle.RestoreRequired = bundle.CompactionEligible
	return bundle
}

func summarizeMapSummary(raw any) string {
	typed, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue(typed["summary"]))
}

func contextBudgetClass(envelope TaskEnvelope) string {
	switch {
	case envelope.ExecutionProfile == "plan_stage_execute":
		return "heavy"
	case len(envelope.PreviousArtifactKinds) > 0:
		return "medium"
	default:
		return "light"
	}
}

func routineBindingsForWork(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy) []UnitOfWorkRoutineBinding {
	family := behaviorFamilyForWork(mode, profile, classification, policy)
	bindings := []UnitOfWorkRoutineBinding{{
		RoutineID: family,
		Family:    family,
		Reason:    "primary execution behavior",
		Priority:  100,
		Required:  true,
	}}
	if classification.RequiresEvidenceBeforeMutation && family != "stale_assumption_detection" {
		bindings = append(bindings, UnitOfWorkRoutineBinding{
			RoutineID: "stale_assumption_detection",
			Family:    "stale_assumption_detection",
			Reason:    "evidence-first classification",
			Priority:  80,
			Required:  false,
		})
	}
	if profile.VerificationRequired && family != "verification_repair" {
		bindings = append(bindings, UnitOfWorkRoutineBinding{
			RoutineID: "verification_repair",
			Family:    "verification_repair",
			Reason:    "profile requires verification",
			Priority:  70,
			Required:  false,
		})
	}
	for _, capabilityID := range policy.PreferredPlanningCapabilities {
		bindings = append(bindings, UnitOfWorkRoutineBinding{
			RoutineID: capabilityID,
			Family:    "planning_capability",
			Reason:    "skill policy planning preference",
			Priority:  60,
			Required:  false,
		})
	}
	for _, capabilityID := range policy.PreferredVerifyCapabilities {
		bindings = append(bindings, UnitOfWorkRoutineBinding{
			RoutineID: capabilityID,
			Family:    "verification_capability",
			Reason:    "skill policy verification preference",
			Priority:  55,
			Required:  false,
		})
	}
	return bindings
}

func skillBindingsForWork(task *core.Task, state *core.Context, policy ResolvedExecutionPolicy) []UnitOfWorkSkillBinding {
	var bindings []UnitOfWorkSkillBinding
	if task != nil && task.Context != nil {
		if raw, ok := task.Context["skills"]; ok {
			for _, skillID := range stringSliceAny(raw) {
				bindings = append(bindings, UnitOfWorkSkillBinding{
					SkillID:  skillID,
					Reason:   "task-scoped skill binding",
					Required: false,
				})
			}
		}
	}
	if state != nil {
		if raw, ok := state.Get("euclo.skills"); ok {
			for _, skillID := range stringSliceAny(raw) {
				bindings = append(bindings, UnitOfWorkSkillBinding{
					SkillID:  skillID,
					Reason:   "state-scoped skill binding",
					Required: false,
				})
			}
		}
	}
	if policy.ResolvedFromSkillPolicy {
		bindings = append(bindings, UnitOfWorkSkillBinding{
			SkillID:  "agent_skill_policy",
			Reason:   "resolved from agent runtime skill policy",
			Required: true,
		})
	}
	return dedupeSkillBindings(bindings)
}

func toolBindingsForWork(envelope TaskEnvelope) []UnitOfWorkToolBinding {
	return []UnitOfWorkToolBinding{
		{
			ToolID:  "workspace_write",
			Allowed: envelope.CapabilitySnapshot.HasWriteTools,
			Reason:  "capability snapshot",
		},
		{
			ToolID:  "verification",
			Allowed: envelope.CapabilitySnapshot.HasVerificationTools,
			Reason:  "capability snapshot",
		},
	}
}

func capabilityBindingsForWork(profile ExecutionProfileSelection, policy ResolvedExecutionPolicy) []UnitOfWorkCapabilityBinding {
	bindings := make([]UnitOfWorkCapabilityBinding, 0, len(profile.PhaseRoutes)+len(policy.PhaseCapabilityConstraints))
	for phase, family := range profile.PhaseRoutes {
		bindings = append(bindings, UnitOfWorkCapabilityBinding{
			CapabilityID: phase,
			Family:       family,
			Required:     true,
		})
	}
	for phase, capabilities := range policy.PhaseCapabilityConstraints {
		for _, capabilityID := range capabilities {
			bindings = append(bindings, UnitOfWorkCapabilityBinding{
				CapabilityID: capabilityID,
				Family:       phase,
				Required:     true,
			})
		}
	}
	return dedupeCapabilityBindings(bindings)
}

func deferredIssueIDsFromState(state *core.Context) []string {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get("euclo.deferred_issue_ids"); ok {
		return uniqueStrings(stringSliceAny(raw))
	}
	return nil
}

func stateString(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString(key))
}

func stringSliceAny(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" && value != "<nil>" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func dedupeSkillBindings(bindings []UnitOfWorkSkillBinding) []UnitOfWorkSkillBinding {
	if len(bindings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(bindings))
	out := make([]UnitOfWorkSkillBinding, 0, len(bindings))
	for _, binding := range bindings {
		key := strings.TrimSpace(binding.SkillID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, binding)
	}
	return out
}

func dedupeCapabilityBindings(bindings []UnitOfWorkCapabilityBinding) []UnitOfWorkCapabilityBinding {
	if len(bindings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(bindings))
	out := make([]UnitOfWorkCapabilityBinding, 0, len(bindings))
	for _, binding := range bindings {
		key := strings.TrimSpace(binding.CapabilityID) + "|" + strings.TrimSpace(binding.Family)
		if key == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, binding)
	}
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
