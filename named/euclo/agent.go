package euclo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Agent is the named coding-runtime boundary. The initial implementation keeps
// the public surface narrow while delegating execution to generic agent
// machinery underneath.
type Agent struct {
	Config         *core.Config
	Delegate       *reactpkg.ReActAgent
	CheckpointPath string
	Memory         memory.MemoryStore
	Environment    agentenv.AgentEnvironment
	GraphDB        *graphdb.Engine
	RetrievalDB    *sql.DB
	PlanStore      frameworkplan.PlanStore
	PatternStore   patterns.PatternStore
	CommentStore   patterns.CommentStore
	ConvVerifier   frameworkplan.ConvergenceVerifier
	GuidanceBroker *guidance.GuidanceBroker
	DeferralPlan   *guidance.DeferralPlan
	DeferralPolicy guidance.DeferralPolicy
	DoomLoop       *capability.DoomLoopDetector
	doomLoopWired  bool

	ModeRegistry        *euclotypes.ModeRegistry
	ProfileRegistry     *euclotypes.ExecutionProfileRegistry
	InteractionRegistry *interaction.ModeMachineRegistry
	CodingCapabilities  *capabilities.EucloCapabilityRegistry
	ProfileCtrl         *orchestrate.ProfileController
	RecoveryCtrl        *orchestrate.RecoveryController
	Emitter             interaction.FrameEmitter // live emitter from TUI; nil means use task-scoped emitter
}

func New(env agentenv.AgentEnvironment) *Agent {
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *Agent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Config = env.Config
	a.Memory = env.Memory
	a.Environment = env
	if env.IndexManager != nil && env.IndexManager.GraphDB != nil {
		a.GraphDB = env.IndexManager.GraphDB
	}
	if a.Delegate == nil {
		a.Delegate = reactpkg.New(env)
	} else if err := a.Delegate.InitializeEnvironment(env); err != nil {
		return err
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.InteractionRegistry == nil {
		a.InteractionRegistry = defaultInteractionRegistry()
	}
	if a.CodingCapabilities == nil {
		a.CodingCapabilities = capabilities.NewDefaultCapabilityRegistry(env)
	}
	if a.DoomLoop == nil {
		a.DoomLoop = capability.NewDoomLoopDetector(capability.DefaultDoomLoopConfig())
	}
	a.ensureGuidanceWiring()

	// Wire the snapshot function for orchestrate package.
	orchestrate.SetDefaultSnapshotFunc(func(reg interface{}) euclotypes.CapabilitySnapshot {
		if registry, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(registry)
		}
		return euclotypes.CapabilitySnapshot{}
	})

	if a.RecoveryCtrl == nil {
		a.RecoveryCtrl = orchestrate.NewRecoveryController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			a.ProfileRegistry,
			a.ModeRegistry,
			env,
		)
	}
	if a.ProfileCtrl == nil {
		a.ProfileCtrl = orchestrate.NewProfileController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			gate.DefaultPhaseGates(),
			env,
			a.ProfileRegistry,
			a.RecoveryCtrl,
		)
	}
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Delegate == nil {
		a.Delegate = &reactpkg.ReActAgent{}
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.CheckpointPath != "" {
		a.Delegate.CheckpointPath = a.CheckpointPath
	}
	return a.Delegate.Initialize(cfg)
}

func (a *Agent) Capabilities() []core.Capability {
	if a == nil || a.Delegate == nil {
		return nil
	}
	return a.Delegate.Capabilities()
}

func (a *Agent) CapabilityRegistry() *capability.Registry {
	if a == nil || a.Delegate == nil {
		return nil
	}
	return a.Delegate.Tools
}

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Delegate == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	env, classification, mode, profile := a.runtimeState(task, nil)
	return a.Delegate.BuildGraph(a.eucloTask(task, env, classification, mode, profile))
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if a.Delegate == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}
	a.ensureGuidanceWiring()
	seedPersistedInteractionState(task, state)
	// Session scoping: prevent recursive Euclo invocations.
	sessionID := generateSessionID()
	if scopeErr := enforceSessionScoping(state, sessionID); scopeErr != nil {
		return &core.Result{Success: false, Error: scopeErr}, scopeErr
	}
	envelope, classification, mode, profile := a.runtimeState(task, state)
	state.Set("euclo.envelope", envelope)
	state.Set("euclo.classification", classification)
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.mode", mode.ModeID)
	state.Set("euclo.execution_profile", profile.ProfileID)
	a.ensureDeferralPlan(task, state)
	livingPlan, activeStep, preflightResult, planErr := a.prepareLivingPlan(ctx, task, state)
	if planErr != nil {
		if preflightResult != nil {
			return preflightResult, planErr
		}
		return &core.Result{Success: false, Error: planErr}, planErr
	}
	if preflightResult != nil {
		if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
			state.Set("euclo.deferral_plan", a.DeferralPlan)
		}
		return preflightResult, planErr
	}
	a.hydratePersistedArtifacts(ctx, task, state)
	var err error
	retrievalPolicy := eucloruntime.ResolveRetrievalPolicy(mode, profile)
	state.Set("euclo.retrieval_policy", retrievalPolicy)
	routing := eucloruntime.RouteCapabilityFamilies(mode, profile)
	state.Set("euclo.capability_family_routing", routing)
	executionTask := a.eucloTask(task, envelope, classification, mode, profile)
	if surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory); surfaces.Workflow != nil {
		workflowID := state.GetString("euclo.workflow_id")
		if workflowID == "" && task != nil && task.Context != nil {
			if value, ok := task.Context["workflow_id"]; ok {
				workflowID = stringValue(value)
			}
		}
		if expansion, expandErr := eucloruntime.ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, state, retrievalPolicy); expandErr == nil {
			executionTask = eucloruntime.ApplyContextExpansion(state, executionTask, expansion)
		} else {
			err = expandErr
		}
	}
	var result *core.Result
	var execErr error
	execEnvelope := eucloruntime.BuildExecutionEnvelope(
		executionTask, state, mode, profile, a.Environment,
		nil, "", "", a.ConfigTelemetry(),
	)
	seedInteractionPrepass(state, executionTask, classification, mode)
	if a.InteractionRegistry != nil {
		var emitter interaction.FrameEmitter
		var withTransitions bool
		if a.Emitter != nil {
			// Live emitter from TUI: use it directly without state transitions
			emitter = a.Emitter
			withTransitions = false
		} else {
			// Task-scoped emitter: check for test script or use NoopEmitter
			emitter, withTransitions = interactionEmitterForTask(executionTask)
		}
		var interactionErr error
		if withTransitions {
			_, _, interactionErr = a.ProfileCtrl.ExecuteInteractiveWithTransitions(
				ctx,
				a.InteractionRegistry,
				mode,
				execEnvelope,
				emitter,
				interactionMaxTransitions(executionTask),
			)
		} else {
			_, _, interactionErr = a.ProfileCtrl.ExecuteInteractive(
				ctx,
				a.InteractionRegistry,
				mode,
				execEnvelope,
				emitter,
			)
		}
		if interactionErr != nil && err == nil {
			err = interactionErr
		}
	}
	result, _, execErr = a.ProfileCtrl.ExecuteProfile(ctx, profile, mode, execEnvelope)
	if err == nil {
		err = execErr
	}
	policy := eucloruntime.ResolveVerificationPolicy(mode, profile)
	state.Set("euclo.verification_policy", policy)
	if err == nil && profile.MutationAllowed {
		if _, applyErr := eucloruntime.ApplyEditIntentArtifacts(ctx, a.CapabilityRegistry(), state); applyErr != nil {
			err = applyErr
		}
	}
	evidence := eucloruntime.NormalizeVerificationEvidence(state)
	state.Set("euclo.verification", evidence)
	var editRecord *eucloruntime.EditExecutionRecord
	if raw, ok := state.Get("euclo.edit_execution"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.EditExecutionRecord); ok {
			editRecord = &typed
		}
	}
	successGate := eucloruntime.EvaluateSuccessGate(policy, evidence, editRecord)
	state.Set("euclo.success_gate", successGate)
	if result != nil {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["verification"] = evidence
		result.Data["success_gate"] = successGate
	}
	if err == nil && !successGate.Allowed {
		err = fmt.Errorf("euclo success gate blocked completion: %s", successGate.Reason)
	}
	if result != nil {
		result.Success = err == nil && successGate.Allowed && result.Success
		if err != nil {
			result.Error = err
		}
	}
	artifacts := euclotypes.CollectArtifactsFromState(state)
	actionLog := eucloruntime.BuildActionLog(state, artifacts)
	state.Set("euclo.action_log", actionLog)
	proofSurface := eucloruntime.BuildProofSurface(state, artifacts)
	state.Set("euclo.proof_surface", proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(state)
	state.Set("euclo.artifacts", artifacts)
	if persistErr := a.persistArtifacts(ctx, task, state, artifacts); persistErr != nil && err == nil {
		err = persistErr
		if result != nil {
			result.Success = false
			result.Error = err
		}
	}
	finalReport := euclotypes.AssembleFinalReport(artifacts)
	state.Set("euclo.final_report", finalReport)
	eucloruntime.EmitObservabilityTelemetry(a.ConfigTelemetry(), task, actionLog, proofSurface)
	if result != nil {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["final_report"] = finalReport
		result.Data["action_log"] = actionLog
		result.Data["proof_surface"] = proofSurface
	}
	a.finalizeLivingPlan(ctx, task, state, livingPlan, activeStep, result, err)
	if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
		state.Set("euclo.deferral_plan", a.DeferralPlan)
	}
	return result, err
}

func (a *Agent) prepareLivingPlan(ctx context.Context, task *core.Task, state *core.Context) (*frameworkplan.LivingPlan, *frameworkplan.PlanStep, *core.Result, error) {
	if a == nil || a.PlanStore == nil {
		return nil, nil, nil, nil
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return nil, nil, nil, nil
	}
	plan, err := a.PlanStore.LoadPlanByWorkflow(ctx, workflowID)
	if err != nil || plan == nil {
		return plan, nil, nil, err
	}
	state.Set("euclo.living_plan", plan)
	step := activeLivingPlanStep(task, plan)
	if step == nil {
		return plan, nil, nil, nil
	}
	if a.DoomLoop != nil {
		a.DoomLoop.Reset()
	}
	gateState, gateErr := a.evaluatePlanStepGate(ctx, task, state, plan, step)
	if gateErr != nil {
		log.Printf("euclo: blocking plan step %s: %v", step.ID, gateErr)
		step.History = append(step.History, frameworkplan.StepAttempt{
			AttemptedAt:   time.Now().UTC(),
			Outcome:       "blocked",
			FailureReason: gateErr.Error(),
		})
		if gateState.shouldInvalidate {
			step.Status = frameworkplan.PlanStepInvalidated
		}
		step.UpdatedAt = time.Now().UTC()
		state.Set("euclo.living_plan", plan)
		_ = a.PlanStore.UpdateStep(ctx, plan.ID, step.ID, step)
		if len(gateState.invalidatedStepIDs) > 0 {
			a.persistPlanStepUpdates(ctx, plan)
		}
		return plan, nil, &core.Result{Success: false, Error: gateErr, Data: map[string]any{"plan_step_status": "blocked"}}, gateErr
	}
	if gateState.result != nil {
		state.Set("euclo.living_plan", plan)
		_ = a.PlanStore.UpdateStep(ctx, plan.ID, step.ID, step)
		return plan, nil, gateState.result, gateState.err
	}
	if gateState.confidenceUpdated {
		step.UpdatedAt = time.Now().UTC()
		state.Set("euclo.living_plan", plan)
		_ = a.PlanStore.UpdateStep(ctx, plan.ID, step.ID, step)
	}
	state.Set("euclo.current_plan_step_id", step.ID)
	return plan, step, nil, nil
}

func (a *Agent) finalizeLivingPlan(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, result *core.Result, execErr error) {
	if a == nil || a.PlanStore == nil || plan == nil {
		return
	}
	if step != nil {
		now := time.Now().UTC()
		attempt := frameworkplan.StepAttempt{
			AttemptedAt:   now,
			GitCheckpoint: gitCheckpoint(ctx, task),
		}
		if execErr == nil {
			step.Status = frameworkplan.PlanStepCompleted
			attempt.Outcome = "completed"
		} else {
			step.Status = frameworkplan.PlanStepFailed
			attempt.Outcome = "failed"
			if execErr != nil {
				attempt.FailureReason = execErr.Error()
			} else if result != nil && result.Error != nil {
				attempt.FailureReason = result.Error.Error()
			}
		}
		step.History = append(step.History, attempt)
		step.UpdatedAt = now
		if execErr == nil {
			a.propagateScopeInvalidations(ctx, plan, step)
		}
		state.Set("euclo.living_plan", plan)
		_ = a.PlanStore.UpdateStep(ctx, plan.ID, step.ID, step)
	}
	if execErr == nil && a.ConvVerifier != nil && plan.ConvergenceTarget != nil {
		failure, err := a.ConvVerifier.Verify(ctx, *plan.ConvergenceTarget)
		if err != nil {
			log.Printf("euclo: convergence verifier failed: %v", err)
		} else if failure == nil {
			now := time.Now().UTC()
			plan.ConvergenceTarget.VerifiedAt = &now
			plan.UpdatedAt = now
			state.Set("euclo.living_plan", plan)
			_ = a.PlanStore.SavePlan(ctx, plan)
		} else {
			log.Printf("euclo: convergence target unmet: %s", failure.Description)
			state.Set("euclo.convergence_failure", *failure)
			if result != nil {
				if result.Data == nil {
					result.Data = map[string]any{}
				}
				result.Data["convergence_failure"] = failure
			}
		}
	}
}

func (a *Agent) requiredSymbolsPresent(step *frameworkplan.PlanStep) bool {
	if step == nil || step.EvidenceGate == nil || len(step.EvidenceGate.RequiredSymbols) == 0 {
		return true
	}
	if a == nil || a.GraphDB == nil {
		return true
	}
	for _, symbolID := range step.EvidenceGate.RequiredSymbols {
		if _, ok := a.GraphDB.GetNode(symbolID); !ok {
			return false
		}
	}
	return true
}

type planStepGateState struct {
	confidenceUpdated  bool
	invalidatedStepIDs []string
	shouldInvalidate   bool
	result             *core.Result
	err                error
}

func (a *Agent) evaluatePlanStepGate(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (planStepGateState, error) {
	var gateState planStepGateState
	if step == nil {
		return gateState, nil
	}
	missingSymbols := a.missingPlanSymbols(step)
	if len(missingSymbols) > 0 {
		for _, symbolID := range missingSymbols {
			gateState.invalidatedStepIDs = append(gateState.invalidatedStepIDs, a.applyInvalidationEvent(plan, frameworkplan.InvalidationEvent{
				Kind:   frameworkplan.InvalidationSymbolChanged,
				Target: symbolID,
				At:     time.Now().UTC(),
			}, step.ID)...)
		}
		gateState.invalidatedStepIDs = uniqueStrings(gateState.invalidatedStepIDs)
		gateState.shouldInvalidate = true
		if len(gateState.invalidatedStepIDs) > 0 {
			_ = a.persistPlanStepUpdates(ctx, plan)
		}
		return gateState, fmt.Errorf("living plan step %s blocked by missing required symbols: %s", step.ID, strings.Join(missingSymbols, ", "))
	}

	activeAnchors, driftedAnchors, err := a.anchorGateState(ctx, task)
	if err != nil {
		return gateState, err
	}
	driftedDeps := intersectStrings(step.AnchorDependencies, driftedAnchors)
	if len(driftedDeps) > 0 {
		for _, anchorID := range driftedDeps {
			gateState.invalidatedStepIDs = append(gateState.invalidatedStepIDs, a.applyInvalidationEvent(plan, frameworkplan.InvalidationEvent{
				Kind:   frameworkplan.InvalidationAnchorDrifted,
				Target: anchorID,
				At:     time.Now().UTC(),
			}, step.ID)...)
		}
		gateState.invalidatedStepIDs = uniqueStrings(gateState.invalidatedStepIDs)
		gateState.shouldInvalidate = true
	}

	confidence := frameworkplan.RecalculateConfidence(step, driftedDeps, missingSymbols, frameworkplan.DefaultConfidenceDegradation())
	if confidence != step.ConfidenceScore {
		step.ConfidenceScore = confidence
		gateState.confidenceUpdated = true
	}
	if confidence < frameworkplan.DefaultConfidenceDegradation().Threshold {
		decision := a.requestGuidance(ctx, guidance.GuidanceRequest{
			Kind:        guidance.GuidanceConfidence,
			Title:       "Low confidence on plan step",
			Description: fmt.Sprintf("Step %q has confidence %.2f (threshold %.2f).", step.ID, confidence, frameworkplan.DefaultConfidenceDegradation().Threshold),
			Choices: []guidance.GuidanceChoice{
				{ID: "proceed", Label: "Proceed", IsDefault: true},
				{ID: "skip", Label: "Skip this step"},
				{ID: "replan", Label: "Re-plan this step"},
			},
			TimeoutBehavior: a.guidanceTimeoutBehavior(guidance.GuidanceConfidence, len(step.Scope)),
			Context: map[string]any{
				"confidence":      confidence,
				"threshold":       frameworkplan.DefaultConfidenceDegradation().Threshold,
				"drifted_anchors": driftedDeps,
				"missing_symbols": missingSymbols,
			},
		}, "proceed")
		if shortCircuitResult, shortCircuitErr, handled := a.applyGuidanceDecision(plan, step, decision, "low confidence on plan step"); handled {
			gateState.result = shortCircuitResult
			gateState.err = shortCircuitErr
			return gateState, nil
		}
	}
	if a.shouldCheckBlastRadius(step) {
		impact := a.GraphDB.ImpactSet(step.Scope, nil, 3)
		expected := len(step.Scope)
		actual := len(impact.Affected)
		if expected > 0 && actual > blastRadiusExpansionThreshold(expected) {
			decision := a.requestGuidance(ctx, guidance.GuidanceRequest{
				Kind:        guidance.GuidanceScopeExpansion,
				Title:       "Larger blast radius than planned",
				Description: fmt.Sprintf("Step %q affects %d symbols, above the planned scope of %d.", step.ID, actual, expected),
				Choices: []guidance.GuidanceChoice{
					{ID: "proceed", Label: "Proceed", IsDefault: true},
					{ID: "skip", Label: "Skip this step"},
					{ID: "replan", Label: "Re-plan this step"},
				},
				TimeoutBehavior: a.guidanceTimeoutBehavior(guidance.GuidanceScopeExpansion, actual),
				Context: map[string]any{
					"expected_symbols": expected,
					"actual_symbols":   actual,
					"affected":         truncateStrings(impact.Affected, 20),
				},
			}, "proceed")
			if shortCircuitResult, shortCircuitErr, handled := a.applyGuidanceDecision(plan, step, decision, "blast radius larger than planned"); handled {
				gateState.result = shortCircuitResult
				gateState.err = shortCircuitErr
				return gateState, nil
			}
		}
	}

	evidence, hasEvidence := mixedEvidenceForStep(state, step)
	if !hasEvidence && step.EvidenceGate != nil && step.EvidenceGate.MaxTotalLoss > 0 {
		return gateState, fmt.Errorf("living plan step %s blocked by missing grounding evidence", step.ID)
	}
	if step.EvidenceGate != nil && !frameworkplan.EvidenceGateAllows(step.EvidenceGate, evidence, activeAnchors, availableSymbolMap(step, a.GraphDB)) {
		if len(step.EvidenceGate.RequiredAnchors) > 0 {
			return gateState, fmt.Errorf("living plan step %s blocked by inactive required anchors", step.ID)
		}
		if step.EvidenceGate.MaxTotalLoss > 0 {
			return gateState, fmt.Errorf("living plan step %s blocked by evidence derivation loss", step.ID)
		}
	}
	return gateState, nil
}

func (a *Agent) ensureGuidanceWiring() {
	if a == nil || a.DoomLoop == nil || a.CapabilityRegistry() == nil {
		return
	}
	registry := a.CapabilityRegistry()
	if !a.doomLoopWired {
		registry.AddPrecheck(a.DoomLoop)
		registry.AddPostcheck(a.DoomLoop)
		a.doomLoopWired = true
	}
	if a.GuidanceBroker != nil {
		registry.SetGuidanceBroker(guidanceRecoveryAdapter{broker: a.GuidanceBroker})
	}
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

func (a *Agent) guidanceTimeoutBehavior(kind guidance.GuidanceKind, blastRadius int) guidance.GuidanceTimeoutBehavior {
	policy := a.DeferralPolicy
	if policy.MaxBlastRadiusForDefer == 0 && len(policy.DeferrableKinds) == 0 {
		policy = guidance.DefaultDeferralPolicy()
	}
	if kind == guidance.GuidanceRecovery {
		return guidance.GuidanceTimeoutFail
	}
	if blastRadius > policy.MaxBlastRadiusForDefer {
		return guidance.GuidanceTimeoutFail
	}
	for _, allowed := range policy.DeferrableKinds {
		if allowed == kind {
			return guidance.GuidanceTimeoutDefer
		}
	}
	return guidance.GuidanceTimeoutFail
}

func (a *Agent) requestGuidance(ctx context.Context, req guidance.GuidanceRequest, fallbackChoice string) guidance.GuidanceDecision {
	if a == nil || a.GuidanceBroker == nil {
		log.Printf("euclo: guidance broker unavailable for %s; proceeding with %s", req.Kind, fallbackChoice)
		return guidance.GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  fallbackChoice,
			DecidedBy: "no-broker",
			DecidedAt: time.Now().UTC(),
		}
	}
	decision, err := a.GuidanceBroker.Request(ctx, req)
	if err != nil || decision == nil {
		log.Printf("euclo: guidance request failed for %s: %v", req.Kind, err)
		return guidance.GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  fallbackChoice,
			DecidedBy: "guidance-error",
			DecidedAt: time.Now().UTC(),
		}
	}
	return *decision
}

func (a *Agent) applyGuidanceDecision(plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, decision guidance.GuidanceDecision, reason string) (*core.Result, error, bool) {
	if plan == nil || step == nil {
		return nil, nil, false
	}
	now := time.Now().UTC()
	switch decision.ChoiceID {
	case "skip":
		step.Status = frameworkplan.PlanStepSkipped
		step.History = append(step.History, frameworkplan.StepAttempt{
			AttemptedAt:   now,
			Outcome:       "skipped",
			FailureReason: reason,
		})
		step.UpdatedAt = now
		return &core.Result{
			Success: true,
			Data: map[string]any{
				"plan_step_status": "skipped",
				"guidance_decision": map[string]any{
					"choice_id":  decision.ChoiceID,
					"decided_by": decision.DecidedBy,
				},
			},
		}, nil, true
	case "replan":
		replanErr := fmt.Errorf("replan requested for step %s", step.ID)
		step.Status = frameworkplan.PlanStepFailed
		step.History = append(step.History, frameworkplan.StepAttempt{
			AttemptedAt:   now,
			Outcome:       "failed",
			FailureReason: replanErr.Error(),
		})
		step.UpdatedAt = now
		return &core.Result{
			Success: false,
			Error:   replanErr,
			Data: map[string]any{
				"plan_step_status": "failed",
				"guidance_decision": map[string]any{
					"choice_id":  decision.ChoiceID,
					"decided_by": decision.DecidedBy,
				},
			},
		}, replanErr, true
	default:
		return nil, nil, false
	}
}

func (a *Agent) shouldCheckBlastRadius(step *frameworkplan.PlanStep) bool {
	return a != nil && a.GraphDB != nil && step != nil && len(step.Scope) > 0
}

func blastRadiusExpansionThreshold(expected int) int {
	if expected <= 0 {
		return 0
	}
	return maxInt(expected*2, expected+5)
}

func truncateStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:limit]...)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type guidanceRecoveryAdapter struct {
	broker *guidance.GuidanceBroker
}

func (a guidanceRecoveryAdapter) RequestRecovery(ctx context.Context, req capability.RecoveryGuidanceRequest) (*capability.RecoveryGuidanceDecision, error) {
	if a.broker == nil {
		return nil, fmt.Errorf("guidance broker unavailable")
	}
	decision, err := a.broker.Request(ctx, guidance.GuidanceRequest{
		Kind:        guidance.GuidanceRecovery,
		Title:       req.Title,
		Description: req.Description,
		Choices: []guidance.GuidanceChoice{
			{ID: "continue", Label: "Continue"},
			{ID: "replan", Label: "Re-plan this step"},
			{ID: "skip", Label: "Skip step"},
			{ID: "stop", Label: "Stop", IsDefault: true},
		},
		TimeoutBehavior: guidance.GuidanceTimeoutFail,
		Context:         req.Context,
	})
	if err != nil {
		return nil, err
	}
	return &capability.RecoveryGuidanceDecision{ChoiceID: decision.ChoiceID}, nil
}

func (a *Agent) missingPlanSymbols(step *frameworkplan.PlanStep) []string {
	if step == nil || a == nil || a.GraphDB == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, symbolID := range append(append([]string{}, step.Scope...), requiredSymbols(step)...) {
		if strings.TrimSpace(symbolID) == "" {
			continue
		}
		if _, ok := seen[symbolID]; ok {
			continue
		}
		seen[symbolID] = struct{}{}
		if _, ok := a.GraphDB.GetNode(symbolID); !ok {
			out = append(out, symbolID)
		}
	}
	return out
}

func (a *Agent) anchorGateState(ctx context.Context, task *core.Task) (map[string]bool, map[string]struct{}, error) {
	active := make(map[string]bool)
	drifted := make(map[string]struct{})
	if a == nil || a.RetrievalDB == nil {
		return active, drifted, nil
	}
	corpusScope := corpusScopeForTask(task)
	driftedRecords, err := retrieval.DriftedAnchors(ctx, a.RetrievalDB, corpusScope)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range driftedRecords {
		drifted[record.AnchorID] = struct{}{}
	}
	activeRecords, err := retrieval.ActiveAnchors(ctx, a.RetrievalDB, corpusScope)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range activeRecords {
		if record.SupersededBy != nil {
			continue
		}
		if _, blocked := drifted[record.AnchorID]; blocked {
			continue
		}
		active[record.AnchorID] = true
	}
	return active, drifted, nil
}

func corpusScopeForTask(task *core.Task) string {
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(stringValue(task.Context["corpus_scope"])); value != "" {
			return value
		}
	}
	return "workspace"
}

func mixedEvidenceForStep(state *core.Context, step *frameworkplan.PlanStep) (retrieval.MixedEvidenceResult, bool) {
	if state == nil || step == nil {
		return retrieval.MixedEvidenceResult{}, false
	}
	raw, ok := state.Get("pipeline.workflow_retrieval")
	if !ok || raw == nil {
		return retrieval.MixedEvidenceResult{}, false
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return retrieval.MixedEvidenceResult{}, false
	}
	rawResults, ok := payload["results"].([]any)
	if !ok {
		if typed, ok := payload["results"].([]map[string]any); ok {
			rawResults = make([]any, 0, len(typed))
			for _, item := range typed {
				rawResults = append(rawResults, item)
			}
		} else {
			return retrieval.MixedEvidenceResult{}, false
		}
	}
	results := make([]retrieval.MixedEvidenceResult, 0, len(rawResults))
	for _, item := range rawResults {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		data, err := json.Marshal(itemMap)
		if err != nil {
			continue
		}
		var result retrieval.MixedEvidenceResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return retrieval.MixedEvidenceResult{}, false
	}
	requiredAnchorSet := make(map[string]struct{})
	for _, anchorID := range step.AnchorDependencies {
		requiredAnchorSet[anchorID] = struct{}{}
	}
	if step.EvidenceGate != nil {
		for _, anchorID := range step.EvidenceGate.RequiredAnchors {
			requiredAnchorSet[anchorID] = struct{}{}
		}
	}
	for _, result := range results {
		for _, anchor := range result.Anchors {
			if _, ok := requiredAnchorSet[anchor.AnchorID]; ok {
				return result, true
			}
		}
	}
	return results[0], true
}

func availableSymbolMap(step *frameworkplan.PlanStep, graph *graphdb.Engine) map[string]bool {
	symbols := make(map[string]bool)
	if step == nil {
		return symbols
	}
	for _, symbolID := range append(append([]string{}, step.Scope...), requiredSymbols(step)...) {
		if strings.TrimSpace(symbolID) == "" {
			continue
		}
		if graph == nil {
			symbols[symbolID] = true
			continue
		}
		_, ok := graph.GetNode(symbolID)
		symbols[symbolID] = ok
	}
	return symbols
}

func requiredSymbols(step *frameworkplan.PlanStep) []string {
	if step == nil || step.EvidenceGate == nil {
		return nil
	}
	return step.EvidenceGate.RequiredSymbols
}

func (a *Agent) applyInvalidationEvent(plan *frameworkplan.LivingPlan, event frameworkplan.InvalidationEvent, excludeStepID string) []string {
	invalidated := frameworkplan.PropagateInvalidation(plan, event)
	if excludeStepID == "" {
		return invalidated
	}
	out := make([]string, 0, len(invalidated))
	for _, stepID := range invalidated {
		if stepID == excludeStepID {
			continue
		}
		out = append(out, stepID)
	}
	return out
}

func (a *Agent) persistPlanStepUpdates(ctx context.Context, plan *frameworkplan.LivingPlan) error {
	if a == nil || a.PlanStore == nil || plan == nil {
		return nil
	}
	for stepID, step := range plan.Steps {
		if step == nil {
			continue
		}
		if err := a.PlanStore.UpdateStep(ctx, plan.ID, stepID, step); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) propagateScopeInvalidations(ctx context.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) {
	if a == nil || plan == nil || step == nil || len(step.Scope) == 0 {
		return
	}
	changed := make([]string, 0)
	for _, symbolID := range step.Scope {
		changed = append(changed, a.applyInvalidationEvent(plan, frameworkplan.InvalidationEvent{
			Kind:   frameworkplan.InvalidationSymbolChanged,
			Target: symbolID,
			At:     time.Now().UTC(),
		}, step.ID)...)
	}
	if len(changed) == 0 {
		return
	}
	plan.UpdatedAt = time.Now().UTC()
	_ = a.persistPlanStepUpdates(ctx, plan)
}

func intersectStrings(values []string, allowed map[string]struct{}) []string {
	if len(values) == 0 || len(allowed) == 0 {
		return nil
	}
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
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

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func activeLivingPlanStep(task *core.Task, plan *frameworkplan.LivingPlan) *frameworkplan.PlanStep {
	if task == nil || task.Context == nil || plan == nil {
		return nil
	}
	if raw, ok := task.Context["current_step_id"]; ok {
		if stepID := strings.TrimSpace(stringValue(raw)); stepID != "" {
			return plan.Steps[stepID]
		}
	}
	return nil
}

func gitCheckpoint(ctx context.Context, task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	workspace := strings.TrimSpace(stringValue(task.Context["workspace"]))
	if workspace == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func seedPersistedInteractionState(task *core.Task, state *core.Context) {
	if task == nil || task.Context == nil || state == nil {
		return
	}
	if _, ok := state.Get("euclo.interaction_state"); !ok {
		if raw, ok := task.Context["euclo.interaction_state"]; ok && raw != nil {
			state.Set("euclo.interaction_state", raw)
		}
	}
}

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (eucloruntime.TaskEnvelope, eucloruntime.TaskClassification, euclotypes.ModeResolution, euclotypes.ExecutionProfileSelection) {
	envelope := eucloruntime.NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, a.ModeRegistry)
	profile := eucloruntime.SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	return envelope, classification, mode, profile
}

func (a *Agent) eucloTask(task *core.Task, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection) *core.Task {
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	cloned.Context["mode"] = mode.ModeID
	cloned.Context["euclo.mode"] = mode.ModeID
	cloned.Context["euclo.execution_profile"] = profile.ProfileID
	cloned.Context["euclo.envelope"] = envelope
	cloned.Context["euclo.classification"] = eucloruntime.ClassificationContextPayload(classification)
	return cloned
}

func seedInteractionPrepass(state *core.Context, task *core.Task, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution) {
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

func (a *Agent) hydratePersistedArtifacts(ctx context.Context, task *core.Task, state *core.Context) {
	if state == nil {
		return
	}
	if raw, ok := state.Get("euclo.artifacts"); ok && raw != nil {
		return
	}
	surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil {
		return
	}
	workflowID := state.GetString("euclo.workflow_id")
	if workflowID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["workflow_id"]; ok {
			workflowID = stringValue(value)
		}
	}
	if workflowID == "" {
		return
	}
	runID := state.GetString("euclo.run_id")
	if runID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["run_id"]; ok {
			runID = stringValue(value)
		}
	}
	artifacts, err := euclotypes.LoadPersistedArtifacts(ctx, surfaces.Workflow, workflowID, runID)
	if err != nil || len(artifacts) == 0 {
		return
	}
	state.Set("euclo.artifacts", artifacts)
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
}

func (a *Agent) persistArtifacts(ctx context.Context, task *core.Task, state *core.Context, artifacts []euclotypes.Artifact) error {
	surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil || len(artifacts) == 0 {
		return nil
	}
	workflowID, runID, err := eucloruntime.EnsureWorkflowRun(ctx, surfaces.Workflow, task, state)
	if err != nil {
		return err
	}
	if workflowID == "" {
		return nil
	}
	return euclotypes.PersistWorkflowArtifacts(ctx, surfaces.Workflow, workflowID, runID, artifacts)
}

func (a *Agent) ConfigTelemetry() core.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

// stringValue extracts a string from an interface value.
func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}
