package assurance

import (
	"context"
	"fmt"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclodispatch "github.com/lexcodex/relurpify/named/euclo/runtime/dispatch"
	"github.com/lexcodex/relurpify/named/euclo/runtime/orchestrate"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

type EmitterResolver func(*core.Task, interaction.FrameEmitter) (interaction.FrameEmitter, bool, int)
type PrepassSeeder func(*core.Context, *core.Task, eucloruntime.TaskClassification, euclotypes.ModeResolution)
type ArtifactPersister func(context.Context, *core.Task, *core.Context, []euclotypes.Artifact) error
type BeforeVerificationHook func(context.Context, *core.Task, *core.Context) error
type MutationCheckpointHook func(context.Context, archaeodomain.MutationCheckpoint, *core.Task, *core.Context) error

type Runtime struct {
	Memory              memory.MemoryStore
	Environment         agentenv.AgentEnvironment
	ProfileCtrl         *orchestrate.ProfileController
	BehaviorDispatcher  *euclodispatch.Dispatcher
	InteractionRegistry *interaction.ModeMachineRegistry
	Emitter             interaction.FrameEmitter
	ResolveEmitter      EmitterResolver
	SeedInteraction     PrepassSeeder
	PersistArtifacts    ArtifactPersister
	BeforeVerification  BeforeVerificationHook
	Checkpoint          MutationCheckpointHook
	ResetDoomLoop       func()
}

type Input struct {
	Task             *core.Task
	ExecutionTask    *core.Task
	WorkflowExecutor graph.WorkflowExecutor
	State            *core.Context
	Classification   eucloruntime.TaskClassification
	Mode             euclotypes.ModeResolution
	Profile          euclotypes.ExecutionProfileSelection
	Telemetry        core.Telemetry
	Work             eucloruntime.UnitOfWork
	ServiceBundle    eucloexec.ServiceBundle
}

type Output struct {
	Result              *core.Result
	Err                 error
	Artifacts           []euclotypes.Artifact
	ActionLog           []eucloruntime.ActionLogEntry
	ProofSurface        eucloruntime.ProofSurface
	FinalReport         map[string]any
	MutationCheckpoints []archaeodomain.MutationCheckpointSummary
}

type ShortCircuitInput struct {
	Task            *core.Task
	State           *core.Context
	Mode            euclotypes.ModeResolution
	Profile         euclotypes.ExecutionProfileSelection
	Telemetry       core.Telemetry
	Result          *core.Result
	SkipSuccessGate bool
}

func Execute(s Runtime, ctx context.Context, in Input) Output {
	var out Output
	if in.State == nil {
		return out
	}
	if s.BehaviorDispatcher == nil {
		out.Err = fmt.Errorf("relurpic behavior service unavailable")
		out.Result = &core.Result{Success: false, Error: out.Err}
		return out
	}
	executionTask, expandErr := s.expandContext(ctx, in)
	if expandErr != nil {
		out.Err = expandErr
	}
	if s.SeedInteraction != nil {
		s.SeedInteraction(in.State, executionTask, in.Classification, in.Mode)
	}
	if s.ProfileCtrl != nil && s.InteractionRegistry != nil {
		execEnvelope := eucloruntime.BuildExecutionEnvelope(
			executionTask, in.State, in.Mode, in.Profile, s.Environment,
			in.ServiceBundle.PlanStore, nil, "", "", in.Telemetry,
		)
		if interactionErr := s.runInteractive(ctx, executionTask, execEnvelope, in.Mode); interactionErr != nil && out.Err == nil {
			out.Err = interactionErr
		}
		if s.ResetDoomLoop != nil {
			s.ResetDoomLoop()
		}
	}
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreDispatch, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}
	var (
		result  *core.Result
		execErr error
	)
	if s.BehaviorDispatcher != nil && in.Work.PrimaryRelurpicCapabilityID != "" {
		result, execErr = s.BehaviorDispatcher.Execute(ctx, eucloexec.ExecuteInput{
			Task:             in.Task,
			ExecutionTask:    executionTask,
			State:            in.State,
			Mode:             in.Mode,
			Profile:          in.Profile,
			Work:             in.Work,
			Environment:      s.Environment,
			ServiceBundle:    in.ServiceBundle,
			WorkflowExecutor: in.WorkflowExecutor,
			Telemetry:        in.Telemetry,
		})
	} else if in.WorkflowExecutor != nil {
		result, execErr = in.WorkflowExecutor.Execute(ctx, executionTask, in.State)
	} else {
		execErr = fmt.Errorf("workflow executor unavailable for primary relurpic capability %q", in.Work.PrimaryRelurpicCapabilityID)
		result = &core.Result{Success: false, Error: execErr}
	}
	if out.Err == nil {
		out.Err = execErr
	}
	out.Result = result
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPostExecution, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreVerification, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}
	if out.Err == nil && s.BeforeVerification != nil {
		if checkpointErr := s.BeforeVerification(ctx, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}
	s.applyVerificationAndArtifacts(ctx, in, &out)
	out.MutationCheckpoints = archaeoexec.MutationCheckpointSummaries(in.State)
	return out
}

func ShortCircuit(s Runtime, ctx context.Context, in ShortCircuitInput) Output {
	out := Output{Result: in.Result}
	if in.State == nil {
		return out
	}
	if out.Result == nil {
		out.Result = &core.Result{Success: true, Data: map[string]any{}}
	}
	if out.Result.Data == nil {
		out.Result.Data = map[string]any{}
	}
	policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
	euclostate.SetVerificationPolicy(in.State, policy)
	artifacts := euclotypes.CollectArtifactsFromState(in.State)
	actionLog := eucloreporting.BuildActionLog(in.State, artifacts)
	euclostate.SetActionLog(in.State, actionLog)
	proofSurface := eucloreporting.BuildProofSurface(in.State, artifacts)
	euclostate.SetProofSurface(in.State, proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(in.State)
	euclostate.SetArtifacts(in.State, artifacts)
	if s.PersistArtifacts != nil {
		if persistErr := s.PersistArtifacts(ctx, in.Task, in.State, artifacts); persistErr != nil {
			out.Err = persistErr
			out.Result.Success = false
			out.Result.Error = persistErr
		}
	}
	finalReport := euclotypes.AssembleFinalReport(artifacts)
	if nextActions := assembleDeferredNextActions(in.State, artifacts); len(nextActions) > 0 {
		finalReport["deferred_next_actions"] = nextActions
	}
	if raw, ok := euclostate.GetProviderRestore(in.State); ok && raw != nil {
		finalReport["provider_restore"] = raw
	}
	if raw, ok := euclostate.GetContextRuntime(in.State); ok {
		finalReport["context_runtime"] = raw
	}
	if runtime, ok := euclostate.GetSecurityRuntime(in.State); ok {
		finalReport["security_runtime"] = runtime
	}
	if runtime, ok := euclostate.GetSharedContextRuntime(in.State); ok {
		finalReport["shared_context_runtime"] = runtime
	}
	euclostate.SetFinalReport(in.State, finalReport)
	eucloreporting.EmitObservabilityTelemetry(in.Telemetry, in.Task, actionLog, proofSurface)
	out.Result.Data["final_report"] = finalReport
	out.Result.Data["action_log"] = actionLog
	out.Result.Data["proof_surface"] = proofSurface
	out.Artifacts = artifacts
	out.ActionLog = actionLog
	out.ProofSurface = proofSurface
	out.FinalReport = finalReport
	out.MutationCheckpoints = archaeoexec.MutationCheckpointSummaries(in.State)
	if !in.SkipSuccessGate {
		if out.Result.Success && out.Err == nil {
			out.Result.Success = true
		}
	}
	return out
}

func (s Runtime) Execute(ctx context.Context, in Input) Output {
	return Execute(s, ctx, in)
}

func (s Runtime) ShortCircuit(ctx context.Context, in ShortCircuitInput) Output {
	return ShortCircuit(s, ctx, in)
}

func (s Runtime) expandContext(ctx context.Context, in Input) (*core.Task, error) {
	executionTask := in.ExecutionTask
	if executionTask == nil {
		executionTask = in.Task
	}
	surfaces := euclorestore.ResolveRuntimeSurfaces(s.Memory)
	if surfaces.Workflow == nil {
		return executionTask, nil
	}
	workflowID, _ := euclostate.GetWorkflowID(in.State)
	if workflowID == "" && in.Task != nil && in.Task.Context != nil {
		if value, ok := in.Task.Context["workflow_id"]; ok {
			if id, ok := value.(string); ok {
				workflowID = id
			}
		}
	}
	policy := euclopolicy.ResolveRetrievalPolicy(in.Mode, in.Profile)
	euclostate.SetRetrievalPolicy(in.State, policy)
	if expansion, err := eucloruntime.ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, in.State, policy); err != nil {
		return executionTask, err
	} else {
		return eucloruntime.ApplyContextExpansion(in.State, executionTask, expansion), nil
	}
}

func (s Runtime) runInteractive(ctx context.Context, executionTask *core.Task, env euclotypes.ExecutionEnvelope, mode euclotypes.ModeResolution) error {
	if s.InteractionRegistry == nil || s.ProfileCtrl == nil {
		return nil
	}
	emitter, withTransitions, maxTransitions := s.resolveEmitter(executionTask)
	if withTransitions {
		_, _, err := s.ProfileCtrl.ExecuteInteractiveWithTransitions(ctx, s.InteractionRegistry, mode, env, emitter, maxTransitions)
		return err
	}
	_, _, err := s.ProfileCtrl.ExecuteInteractive(ctx, s.InteractionRegistry, mode, env, emitter)
	return err
}

func (s Runtime) resolveEmitter(task *core.Task) (interaction.FrameEmitter, bool, int) {
	if s.ResolveEmitter != nil {
		return s.ResolveEmitter(task, s.Emitter)
	}
	if s.Emitter != nil {
		return s.Emitter, false, 0
	}
	return &interaction.NoopEmitter{}, false, 0
}

func (s Runtime) applyVerificationAndArtifacts(ctx context.Context, in Input, out *Output) {
	policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
	euclostate.SetVerificationPolicy(in.State, policy)
	if out.Err == nil && in.Profile.MutationAllowed {
		if _, applyErr := eucloruntime.ApplyEditIntentArtifacts(ctx, s.Environment.Registry, in.State); applyErr != nil {
			out.Err = applyErr
		}
	}
	evidence := eucloruntime.NormalizeVerificationEvidence(in.State)
	euclostate.SetVerification(in.State, evidence)
	var editRecord *eucloruntime.EditExecutionRecord
	if raw, ok := euclostate.GetEditExecution(in.State); ok {
		editRecord = &raw
	}
	successGate := eucloruntime.EvaluateSuccessGate(policy, evidence, editRecord, in.State)
	if _, ok := euclostate.GetExecutionWaiver(in.State); ok {
		originalReason := successGate.Reason
		successGate.WaiverApplied = true
		successGate.DegradationMode = "operator_waiver"
		successGate.DegradationReason = "operator_waiver"
		successGate.AutomaticDegradation = false
		successGate.Allowed = true
		if originalReason != "" && originalReason != "manual_verification_allowed" {
			successGate.Details = append(successGate.Details, "waived_reason="+originalReason)
			successGate.Reason = "operator_waiver_applied"
		}
		successGate.AssuranceClass = eucloruntime.AssuranceClassOperatorDeferred
	} else if mode, reason, degraded := eucloruntime.DetectAutomaticVerificationDegradation(policy, in.State, evidence); degraded {
		successGate.AutomaticDegradation = true
		successGate.DegradationMode = mode
		successGate.DegradationReason = reason
	}
	if trace, ok := euclostate.GetRecoveryTrace(in.State); ok {
		switch trace.Status {
		case "repair_exhausted":
			successGate.AssuranceClass = eucloruntime.AssuranceClassRepairExhausted
			if successGate.Reason == "" || successGate.Reason == "verification_status_rejected" {
				successGate.Reason = "repair_exhausted"
			}
			if trace.AttemptCount > 0 {
				successGate.Details = append(successGate.Details, fmt.Sprintf("repair_attempt_count=%d", trace.AttemptCount))
			}
		case "repaired":
			if trace.AttemptCount > 0 {
				successGate.Details = append(successGate.Details, fmt.Sprintf("repair_attempt_count=%d", trace.AttemptCount))
			}
		}
	}
	euclostate.SetSuccessGate(in.State, successGate)
	euclostate.SetAssuranceClass(in.State, successGate.AssuranceClass)
	if raw, ok := euclostate.GetExecutionWaiver(in.State); ok {
		euclostate.SetWaiver(in.State, raw)
	}
	if out.Result != nil {
		if out.Result.Data == nil {
			out.Result.Data = map[string]any{}
		}
		out.Result.Data["verification"] = evidence
		out.Result.Data["success_gate"] = successGate
		out.Result.Data["assurance_class"] = successGate.AssuranceClass
	}
	if out.Err == nil && !successGate.Allowed {
		out.Err = fmt.Errorf("euclo success gate blocked completion: %s", successGate.Reason)
	}
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreFinalization, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}
	if out.Result != nil {
		out.Result.Success = out.Err == nil && successGate.Allowed && out.Result.Success
		if out.Err != nil {
			out.Result.Error = out.Err
		}
	}
	artifacts := euclotypes.CollectArtifactsFromState(in.State)
	actionLog := eucloreporting.BuildActionLog(in.State, artifacts)
	euclostate.SetActionLog(in.State, actionLog)
	proofSurface := eucloreporting.BuildProofSurface(in.State, artifacts)
	euclostate.SetProofSurface(in.State, proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(in.State)
	euclostate.SetArtifacts(in.State, artifacts)
	if s.PersistArtifacts != nil {
		if persistErr := s.PersistArtifacts(ctx, in.Task, in.State, artifacts); persistErr != nil && out.Err == nil {
			out.Err = persistErr
			if out.Result != nil {
				out.Result.Success = false
				out.Result.Error = out.Err
			}
		}
	}
	finalReport := euclotypes.AssembleFinalReport(artifacts)
<<<<<<< HEAD
	if assuranceClass, ok := euclostate.GetAssuranceClass(in.State); ok {
		finalReport["assurance_class"] = assuranceClass
=======
	if nextActions := assembleDeferredNextActions(in.State, artifacts); len(nextActions) > 0 {
		finalReport["deferred_next_actions"] = nextActions
	}
	if raw, ok := in.State.Get("euclo.assurance_class"); ok && raw != nil {
		finalReport["assurance_class"] = raw
>>>>>>> 71ba95d (workspace language detection, deferred issue lifecyle, cross session learning)
	}
	if raw, ok := in.State.Get("euclo.waiver"); ok && raw != nil {
		finalReport["waiver"] = raw
	}
	if successGate.DegradationMode != "" {
		finalReport["degradation_mode"] = successGate.DegradationMode
	}
	if successGate.DegradationReason != "" {
		finalReport["degradation_reason"] = successGate.DegradationReason
	}
	euclostate.SetFinalReport(in.State, finalReport)
	eucloreporting.EmitObservabilityTelemetry(in.Telemetry, in.Task, actionLog, proofSurface)
	if out.Result != nil {
		if out.Result.Data == nil {
			out.Result.Data = map[string]any{}
		}
		out.Result.Data["final_report"] = finalReport
		out.Result.Data["action_log"] = actionLog
		out.Result.Data["proof_surface"] = proofSurface
	}
	out.Artifacts = artifacts
	out.ActionLog = actionLog
	out.ProofSurface = proofSurface
	out.FinalReport = finalReport
	out.MutationCheckpoints = archaeoexec.MutationCheckpointSummaries(in.State)
}

func (s Runtime) runCheckpoint(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context) error {
	if s.Checkpoint == nil {
		return nil
	}
	return s.Checkpoint(ctx, checkpoint, task, state)
}

func assembleDeferredNextActions(state *core.Context, artifacts []euclotypes.Artifact) []eucloruntime.DeferralNextAction {
	issues := deferredIssuesFromState(state)
	if len(issues) == 0 {
		issues = deferredIssuesFromArtifacts(artifacts)
	}
	if len(issues) == 0 {
		return nil
	}
	return eucloruntime.AssembleDeferralNextActions(issues)
}

func deferredIssuesFromState(state *core.Context) []eucloruntime.DeferredExecutionIssue {
	if state == nil {
		return nil
	}
	issues, ok := euclostate.GetDeferredIssues(state)
	if !ok {
		return nil
	}
	return append([]eucloruntime.DeferredExecutionIssue(nil), issues...)
}

func deferredIssuesFromArtifacts(artifacts []euclotypes.Artifact) []eucloruntime.DeferredExecutionIssue {
	for _, artifact := range artifacts {
		if artifact.Kind != euclotypes.ArtifactKindDeferredExecutionIssues {
			continue
		}
		switch typed := artifact.Payload.(type) {
		case []eucloruntime.DeferredExecutionIssue:
			return append([]eucloruntime.DeferredExecutionIssue(nil), typed...)
		case []any:
			issues := make([]eucloruntime.DeferredExecutionIssue, 0, len(typed))
			for _, item := range typed {
				if issue, ok := item.(eucloruntime.DeferredExecutionIssue); ok {
					issues = append(issues, issue)
				}
			}
			return issues
		}
	}
	return nil
}
