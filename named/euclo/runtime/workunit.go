package runtime

import (
	"fmt"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
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
	primaryCapabilityID := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	if primaryCapabilityID == euclorelurpic.CapabilityArchaeologyImplement && (planBinding == nil || !planBinding.IsPlanBacked) {
		primaryCapabilityID = euclorelurpic.CapabilityArchaeologyCompilePlan
	}
	supportingCapabilityIDs := supportingRelurpicCapabilitiesForPrimary(primaryCapabilityID, envelope, classification, mode, profile)
	if executor.ExecutorID == "" {
		executor = SelectExecutorDescriptor(mode, profile, classification, resolvedPolicy, planBinding, primaryCapabilityID, supportingCapabilityIDs)
	}
	uow := UnitOfWork{
		ID:                              firstNonEmpty(stateString(state, "euclo.unit_of_work_id"), envelope.TaskID),
		WorkflowID:                      workflowIDFromTaskState(task, state),
		RunID:                           runIDFromTaskState(task, state),
		ExecutionID:                     executionID(task, state),
		RootID:                          stateString(state, "euclo.root_unit_of_work_id"),
		ModeID:                          mode.ModeID,
		ObjectiveKind:                   objectiveKindForWork(mode, profile, classification),
		BehaviorFamily:                  behaviorFamilyForWork(mode, profile, classification, resolvedPolicy),
		ContextStrategyID:               contextStrategyForWork(mode, modeRegistry),
		VerificationPolicyID:            verificationPolicyForWork(mode, profile, resolvedPolicy),
		DeferralPolicyID:                deferralPolicyForWork(mode, profile, classification),
		CheckpointPolicyID:              checkpointPolicyForWork(mode, profile, resolvedPolicy),
		PrimaryRelurpicCapabilityID:     primaryCapabilityID,
		SupportingRelurpicCapabilityIDs: supportingCapabilityIDs,
		SemanticInputs:                  semanticInputs,
		ResolvedPolicy:                  resolvedPolicy,
		ExecutorDescriptor:              executor,
		PlanBinding:                     planBinding,
		ContextBundle:                   contextBundleFromState(task, state, envelope, semanticInputs),
		RoutineBindings:                 routineBindingsForWork(mode, profile, classification, resolvedPolicy),
		SkillBindings:                   skillBindingsForWork(task, state, resolvedPolicy),
		ToolBindings:                    toolBindingsForWork(envelope),
		CapabilityBindings:              capabilityBindingsForWork(profile, resolvedPolicy),
		Status:                          UnitOfWorkStatusReady,
		ResultClass:                     "",
		DeferredIssueIDs:                deferredIssueIDsFromState(state),
		CreatedAt:                       now,
		UpdatedAt:                       now,
	}
	if existing, ok := existingUnitOfWork(state); ok {
		if !existing.CreatedAt.IsZero() {
			uow.CreatedAt = existing.CreatedAt
		}
		if strings.TrimSpace(existing.ID) != "" {
			uow.ID = existing.ID
		}
		if strings.TrimSpace(uow.RootID) == "" && strings.TrimSpace(existing.RootID) != "" {
			uow.RootID = existing.RootID
		}
		if uow.PlanBinding == nil && existing.PlanBinding != nil {
			uow.PlanBinding = clonePlanBinding(existing.PlanBinding)
		}
		if len(uow.SemanticInputs.PatternRefs) == 0 && len(existing.SemanticInputs.PatternRefs) > 0 &&
			(workUnitUsesArchaeoContext(uow) || uow.PrimaryRelurpicCapabilityID == existing.PrimaryRelurpicCapabilityID) {
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
		if strings.TrimSpace(uow.PredecessorUnitOfWorkID) == "" && strings.TrimSpace(existing.PredecessorUnitOfWorkID) != "" {
			uow.PredecessorUnitOfWorkID = existing.PredecessorUnitOfWorkID
		}
		if strings.TrimSpace(uow.PrimaryRelurpicCapabilityID) == "" && strings.TrimSpace(existing.PrimaryRelurpicCapabilityID) != "" {
			uow.PrimaryRelurpicCapabilityID = existing.PrimaryRelurpicCapabilityID
		}
		if len(uow.SupportingRelurpicCapabilityIDs) == 0 && len(existing.SupportingRelurpicCapabilityIDs) > 0 {
			uow.SupportingRelurpicCapabilityIDs = append([]string(nil), existing.SupportingRelurpicCapabilityIDs...)
		}
	}
	if existing, ok := existingUnitOfWork(state); ok {
		ApplyUnitOfWorkTransition(existing, &uow, now)
	}
	if uow.ID == "" {
		uow.ID = firstNonEmpty(uow.ExecutionID, uow.RunID, uow.WorkflowID, "euclo-work")
	}
	if uow.RootID == "" {
		uow.RootID = uow.ID
	}
	return uow
}

func primaryRelurpicCapabilityForWork(envelope TaskEnvelope, classification TaskClassification, mode ModeResolution, profile ExecutionProfileSelection) string {
	lower := strings.ToLower(strings.TrimSpace(envelope.Instruction))
	switch mode.ModeID {
	case "planning":
		switch {
		case planningExploreIntent(lower):
			return euclorelurpic.CapabilityArchaeologyExplore
		case planningCompileIntent(lower):
			return euclorelurpic.CapabilityArchaeologyCompilePlan
		case planningImplementIntent(lower):
			return euclorelurpic.CapabilityArchaeologyImplement
		case profile.ProfileID == "plan_stage_execute":
			if envelope.EditPermitted {
				return euclorelurpic.CapabilityArchaeologyImplement
			}
			return euclorelurpic.CapabilityArchaeologyCompilePlan
		default:
			return euclorelurpic.CapabilityArchaeologyExplore
		}
	case "chat":
		// Sub-capability selection based on classification signals, not re-scanning text.
		// If review signals fired, use inspect; otherwise use ask.
		if containsIntent(classification.IntentFamilies, "review") {
			return euclorelurpic.CapabilityChatInspect
		}
		return euclorelurpic.CapabilityChatAsk
	case "debug":
		// Simple repair routing: check classification signals instead of re-scanning text.
		if debugSimpleRepairIntentFromSignals(classification.ReasonCodes) {
			return euclorelurpic.CapabilityDebugRepairSimple
		}
		return euclorelurpic.CapabilityDebugInvestigateRepair
	case "review":
		return euclorelurpic.CapabilityChatInspect
	default:
		// For unclassified modes, use signal-based classification for sub-capability selection.
		if containsIntent(classification.IntentFamilies, "chat") {
			return euclorelurpic.CapabilityChatAsk
		}
		if containsIntent(classification.IntentFamilies, "review") {
			return euclorelurpic.CapabilityChatInspect
		}
		if mode.ModeID == "code" {
			return euclorelurpic.CapabilityChatImplement
		}
		if envelope.EditPermitted || profile.ProfileID == "edit_verify_repair" || profile.ProfileID == "test_driven_generation" {
			return euclorelurpic.CapabilityChatImplement
		}
		if !classification.EditPermitted && len(classification.IntentFamilies) == 0 {
			return euclorelurpic.CapabilityChatAsk
		}
		return euclorelurpic.CapabilityChatInspect
	}
}

func supportingRelurpicCapabilitiesForWork(envelope TaskEnvelope, classification TaskClassification, mode ModeResolution, profile ExecutionProfileSelection) []string {
	return supportingRelurpicCapabilitiesForPrimary(primaryRelurpicCapabilityForWork(envelope, classification, mode, profile), envelope, classification, mode, profile)
}

func supportingRelurpicCapabilitiesForPrimary(primaryID string, envelope TaskEnvelope, classification TaskClassification, mode ModeResolution, profile ExecutionProfileSelection) []string {
	reg := euclorelurpic.DefaultRegistry()
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || id == primaryID {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range reg.SupportingForPrimary(primaryID) {
		add(id)
	}
	switch primaryID {
	case euclorelurpic.CapabilityChatImplement:
		if classification.RequiresEvidenceBeforeMutation {
			add(euclorelurpic.CapabilityDebugInvestigateRepair)
		}
		if classification.Scope == "cross_cutting" {
			add(euclorelurpic.CapabilityArchaeologyExplore)
		}
	case euclorelurpic.CapabilityArchaeologyExplore:
		add(euclorelurpic.CapabilityArchaeologyCompilePlan)
	case euclorelurpic.CapabilityArchaeologyImplement:
		add(euclorelurpic.CapabilityArchaeologyCompilePlan)
		add(euclorelurpic.CapabilityArchaeologyExplore)
	case euclorelurpic.CapabilityDebugInvestigateRepair:
		if planningCompileIntent(strings.ToLower(strings.TrimSpace(envelope.Instruction))) {
			add(euclorelurpic.CapabilityArchaeologyCompilePlan)
		}
	}
	return out
}

// debugSimpleRepairIntentFromSignals checks classification signals for simple repair intent.
// This replaces text scanning with signal-based detection from the classification pass.
func debugSimpleRepairIntentFromSignals(reasonCodes []string) bool {
	for _, reason := range reasonCodes {
		if strings.HasPrefix(reason, "task_structure:simple_repair:") {
			return true
		}
	}
	return false
}

// debugSimpleRepairIntent detects if the prompt is asking for a simple/quick fix.
// Deprecated: Use debugSimpleRepairIntentFromSignals with classification signals instead.
// Kept for backward compatibility in existing tests.
func debugSimpleRepairIntent(lower string) bool {
	// First, check for investigative phrases - if present, this is NOT a simple repair
	investigativePhrases := []string{"investigate", "find the", "root cause", "trace the", "debug the"}
	for _, phrase := range investigativePhrases {
		if strings.Contains(lower, phrase) {
			return false
		}
	}

	simpleRepairPhrases := []string{
		"fix this",
		"apply a fix",
		"quick repair",
		"simple fix",
		"quick fix",
		"minor fix",
		"small fix",
		"patch this",
		"fix the bug",
		"fix it",
	}
	for _, phrase := range simpleRepairPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	// Check for patterns like "fix: <description>" or "fix - <description>"
	if strings.HasPrefix(lower, "fix ") || strings.HasPrefix(lower, "fix: ") || strings.HasPrefix(lower, "fix - ") {
		return true
	}
	return false
}

func planningCompileIntent(lower string) bool {
	return hasAnyPhrase(lower,
		"compile the plan",
		"compile plan",
		"finalize the plan",
		"finalize plan",
		"produce the plan",
		"create the plan",
		"write the plan",
	)
}

func planningExploreIntent(lower string) bool {
	return hasAnyPhrase(lower,
		"explore ",
		"identify the dominant",
		"surface code patterns",
		"surface patterns",
		"find patterns",
		"inspect the repository",
		"inspect the codebase",
	)
}

func planningImplementIntent(lower string) bool {
	return hasAnyPhrase(lower,
		"implement the plan",
		"implement the compiled plan",
		"execute the plan",
		"execute the compiled plan",
		"carry out the plan",
		"carry out the compiled plan",
		"stage and execute",
	)
}

func hasAnyPhrase(lower string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
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
		return "tdd_execution"
	case classification.RequiresEvidenceBeforeMutation:
		return "investigation"
	default:
		return "direct_execution"
	}
}

func behaviorFamilyForWork(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy) string {
	if mode.ModeID == "review" {
		if policy.ReviewApprovalRules.RequireVerificationEvidence || policy.ReviewApprovalRules.RejectOnUnresolvedErrors {
			return "approval_assessment"
		}
		if len(policy.ReviewCriteria) > 0 || len(policy.ReviewFocusTags) > 0 {
			return "coherence_assessment"
		}
		return "tension_assessment"
	}
	if len(policy.PreferredPlanningCapabilities) > 0 && (mode.ModeID == "planning" || profile.ProfileID == "plan_stage_execute") {
		return "gap_analysis"
	}
	if len(policy.PreferredVerifyCapabilities) > 0 && profile.VerificationRequired {
		return "failed_verification_repair"
	}
	switch profile.ProfileID {
	case "edit_verify_repair":
		return "direct_change_execution"
	case "reproduce_localize_patch":
		return "stale_assumption_detection"
	case "test_driven_generation":
		return "tdd_red_green_refactor"
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
	if planPayloadBacked(state) {
		binding.IsPlanBacked = true
		if binding.ActiveStepID == "" {
			if stepID := firstPlanStepID(state); stepID != "" {
				binding.ActiveStepID = stepID
			}
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

func planPayloadBacked(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, ok := state.Get("pipeline.plan")
	if !ok || raw == nil {
		return false
	}
	payload, ok := raw.(map[string]any)
	if !ok || len(payload) == 0 {
		return false
	}
	if plan, ok := payload["plan"].(map[string]any); ok {
		payload = plan
	}
	return len(anyMapSlice(payload["steps"])) > 0 || len(anyMapSlice(payload["items"])) > 0
}

func firstPlanStepID(state *core.Context) string {
	if state == nil {
		return ""
	}
	raw, ok := state.Get("pipeline.plan")
	if !ok || raw == nil {
		return ""
	}
	payload, ok := raw.(map[string]any)
	if !ok || len(payload) == 0 {
		return ""
	}
	if plan, ok := payload["plan"].(map[string]any); ok {
		payload = plan
	}
	for _, step := range anyMapSlice(payload["steps"]) {
		if stepID := strings.TrimSpace(stringValue(step["id"])); stepID != "" {
			return stepID
		}
	}
	for _, item := range anyMapSlice(payload["items"]) {
		if stepID := strings.TrimSpace(stringValue(item["id"])); stepID != "" {
			return stepID
		}
	}
	return ""
}

func anyMapSlice(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			record, ok := item.(map[string]any)
			if ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func populatePlanBinding(binding *UnitOfWorkPlanBinding, version *archaeodomain.VersionedLivingPlan) {
	if binding == nil || version == nil {
		return
	}
	binding.PlanID = strings.TrimSpace(version.Plan.ID)
	binding.PlanVersion = version.Version
	binding.RootChunkIDs = append([]string(nil), version.RootChunkIDs...)
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
	addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
		RoutineID: family,
		Family:    family,
		Reason:    "primary execution behavior",
		Priority:  100,
		Required:  true,
	})
	for _, binding := range modeScopedRoutineBindings(mode, profile, classification, policy) {
		addRoutineBinding(&bindings, binding)
	}
	if classification.RequiresEvidenceBeforeMutation && family != "stale_assumption_detection" {
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: "stale_assumption_detection",
			Family:    "stale_assumption_detection",
			Reason:    "evidence-first classification",
			Priority:  80,
			Required:  false,
		})
	}
	if profile.VerificationRequired && family != "failed_verification_repair" {
		if family == "tdd_red_green_refactor" {
			goto preferredPlanning
		}
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: "verification_repair",
			Family:    "failed_verification_repair",
			Reason:    "profile requires executed verification and bounded repair on failure",
			Priority:  70,
			Required:  false,
		})
	}
preferredPlanning:
	for _, capabilityID := range policy.PreferredPlanningCapabilities {
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: capabilityID,
			Family:    "planning_capability",
			Reason:    "skill policy planning preference",
			Priority:  60,
			Required:  false,
		})
	}
	for _, capabilityID := range policy.PreferredVerifyCapabilities {
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: capabilityID,
			Family:    "verification_capability",
			Reason:    "skill policy verification preference",
			Priority:  55,
			Required:  false,
		})
	}
	return bindings
}

func modeScopedRoutineBindings(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy) []UnitOfWorkRoutineBinding {
	var bindings []UnitOfWorkRoutineBinding
	switch mode.ModeID {
	case "planning":
		bindings = append(bindings,
			UnitOfWorkRoutineBinding{RoutineID: "pattern_surface_and_confirm", Family: "pattern_surface_and_confirm", Reason: "planning mode requires pattern grounding", Priority: 95, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "prospective_structure_assessment", Family: "prospective_structure_assessment", Reason: "planning mode explores likely structure", Priority: 90, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "convergence_guard", Family: "convergence_guard", Reason: "planning mode protects living-plan convergence", Priority: 88, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "coherence_assessment", Family: "coherence_assessment", Reason: "planning mode checks semantic coherence", Priority: 84, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "scope_expansion_assessment", Family: "scope_expansion_assessment", Reason: "planning mode detects scope growth", Priority: 80, Required: false},
		)
	case "debug":
		bindings = append(bindings,
			UnitOfWorkRoutineBinding{RoutineID: "tension_assessment", Family: "tension_assessment", Reason: "debug mode analyzes contradictions and tensions", Priority: 92, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "stale_assumption_detection", Family: "stale_assumption_detection", Reason: "debug mode checks stale assumptions", Priority: 90, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "verification_repair", Family: "failed_verification_repair", Reason: "debug mode verifies fixes and performs bounded repair on failure", Priority: 88, Required: false},
		)
	case "review":
		bindings = append(bindings,
			UnitOfWorkRoutineBinding{RoutineID: "tension_assessment", Family: "tension_assessment", Reason: "review mode highlights tensions", Priority: 92, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "coherence_assessment", Family: "coherence_assessment", Reason: "review mode checks coherence", Priority: 90, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "compatibility_assessment", Family: "compatibility_assessment", Reason: "review mode checks compatibility impact", Priority: 86, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "approval_assessment", Family: "approval_assessment", Reason: "review mode applies approval rules", Priority: 82, Required: false},
		)
	case "tdd":
		bindings = append(bindings,
			UnitOfWorkRoutineBinding{RoutineID: "tdd_red_green_refactor", Family: "tdd_red_green_refactor", Reason: "tdd mode executes red/green contract", Priority: 96, Required: false},
		)
	default:
		bindings = append(bindings,
			UnitOfWorkRoutineBinding{RoutineID: "gap_analysis", Family: "gap_analysis", Reason: "direct collect-context execution may need targeted gap analysis", Priority: 76, Required: false},
			UnitOfWorkRoutineBinding{RoutineID: "verification_repair", Family: "failed_verification_repair", Reason: "direct collect-context execution may need bounded failed-verification repair", Priority: 74, Required: false},
		)
	}
	if profile.ProfileID == "reproduce_localize_patch" {
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: "tension_assessment",
			Family:    "tension_assessment",
			Reason:    "reproduce/localize flows benefit from tension analysis",
			Priority:  89,
			Required:  false,
		})
	}
	if classification.RequiresDeterministicStages && mode.ModeID != "planning" {
		addRoutineBinding(&bindings, UnitOfWorkRoutineBinding{
			RoutineID: "scope_expansion_assessment",
			Family:    "scope_expansion_assessment",
			Reason:    "deterministic execution monitors scope drift",
			Priority:  72,
			Required:  false,
		})
	}
	return bindings
}

func addRoutineBinding(bindings *[]UnitOfWorkRoutineBinding, candidate UnitOfWorkRoutineBinding) {
	if bindings == nil {
		return
	}
	if candidate.RoutineID == "" && candidate.Family == "" {
		return
	}
	for _, existing := range *bindings {
		if existing.RoutineID == candidate.RoutineID && existing.Family == candidate.Family {
			return
		}
	}
	*bindings = append(*bindings, candidate)
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
