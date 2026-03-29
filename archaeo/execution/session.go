package execution

import (
	"context"
	"fmt"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/named/euclo/runtime/orchestrate"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
)

type EmitterResolver func(*core.Task, interaction.FrameEmitter) (interaction.FrameEmitter, bool, int)
type PrepassSeeder func(*core.Context, *core.Task, eucloruntime.TaskClassification, euclotypes.ModeResolution)
type ArtifactPersister func(context.Context, *core.Task, *core.Context, []euclotypes.Artifact) error
type BeforeVerificationHook func(context.Context, *core.Task, *core.Context) error
type MutationCheckpointHook func(context.Context, archaeodomain.MutationCheckpoint, *core.Task, *core.Context) error

type SessionService struct {
	Memory              memory.MemoryStore
	Environment         agentenv.AgentEnvironment
	ProfileCtrl         *orchestrate.ProfileController
	BehaviorService     *orchestrate.Service
	InteractionRegistry *interaction.ModeMachineRegistry
	Emitter             interaction.FrameEmitter
	ResolveEmitter      EmitterResolver
	SeedInteraction     PrepassSeeder
	PersistArtifacts    ArtifactPersister
	BeforeVerification  BeforeVerificationHook
	Checkpoint          MutationCheckpointHook
}

type SessionInput struct {
	Task             *core.Task
	ExecutionTask    *core.Task
	WorkflowExecutor graph.WorkflowExecutor
	State            *core.Context
	Classification   eucloruntime.TaskClassification
	Mode             euclotypes.ModeResolution
	Profile          euclotypes.ExecutionProfileSelection
	Telemetry        core.Telemetry
	Work             eucloruntime.UnitOfWork
}

type SessionOutput struct {
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

func (s SessionService) Execute(ctx context.Context, in SessionInput) SessionOutput {
	var out SessionOutput
	if in.State == nil {
		return out
	}
	if s.BehaviorService == nil {
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
			nil, "", "", in.Telemetry,
		)
		if interactionErr := s.runInteractive(ctx, executionTask, execEnvelope, in.Mode); interactionErr != nil && out.Err == nil {
			out.Err = interactionErr
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
	if s.BehaviorService != nil && in.Work.PrimaryRelurpicCapabilityID != "" {
		result, execErr = s.BehaviorService.Execute(ctx, eucloexec.ExecuteInput{
			Task:             in.Task,
			ExecutionTask:    executionTask,
			State:            in.State,
			Mode:             in.Mode,
			Profile:          in.Profile,
			Work:             in.Work,
			Environment:      s.Environment,
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
	out.MutationCheckpoints = executionMutationCheckpointSummaries(in.State)
	return out
}

func (s SessionService) ShortCircuit(ctx context.Context, in ShortCircuitInput) SessionOutput {
	out := SessionOutput{Result: in.Result}
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
	in.State.Set("euclo.verification_policy", policy)
	artifacts := euclotypes.CollectArtifactsFromState(in.State)
	actionLog := eucloreporting.BuildActionLog(in.State, artifacts)
	in.State.Set("euclo.action_log", actionLog)
	proofSurface := eucloreporting.BuildProofSurface(in.State, artifacts)
	in.State.Set("euclo.proof_surface", proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(in.State)
	in.State.Set("euclo.artifacts", artifacts)
	if s.PersistArtifacts != nil {
		if persistErr := s.PersistArtifacts(ctx, in.Task, in.State, artifacts); persistErr != nil {
			out.Err = persistErr
			out.Result.Success = false
			out.Result.Error = persistErr
		}
	}
	finalReport := euclotypes.AssembleFinalReport(artifacts)
	if raw, ok := in.State.Get("euclo.provider_restore"); ok && raw != nil {
		finalReport["provider_restore"] = raw
	}
	if raw, ok := in.State.Get("euclo.context_runtime"); ok && raw != nil {
		finalReport["context_runtime"] = raw
	}
	if raw, ok := in.State.Get("euclo.security_runtime"); ok && raw != nil {
		finalReport["security_runtime"] = raw
	}
	if raw, ok := in.State.Get("euclo.shared_context_runtime"); ok && raw != nil {
		finalReport["shared_context_runtime"] = raw
	}
	in.State.Set("euclo.final_report", finalReport)
	eucloreporting.EmitObservabilityTelemetry(in.Telemetry, in.Task, actionLog, proofSurface)
	out.Result.Data["final_report"] = finalReport
	out.Result.Data["action_log"] = actionLog
	out.Result.Data["proof_surface"] = proofSurface
	out.Artifacts = artifacts
	out.ActionLog = actionLog
	out.ProofSurface = proofSurface
	out.FinalReport = finalReport
	out.MutationCheckpoints = executionMutationCheckpointSummaries(in.State)
	if !in.SkipSuccessGate {
		if out.Result.Success && out.Err == nil {
			out.Result.Success = true
		}
	}
	return out
}

func (s SessionService) expandContext(ctx context.Context, in SessionInput) (*core.Task, error) {
	executionTask := in.ExecutionTask
	if executionTask == nil {
		executionTask = in.Task
	}
	surfaces := euclorestore.ResolveRuntimeSurfaces(s.Memory)
	if surfaces.Workflow == nil {
		return executionTask, nil
	}
	workflowID := in.State.GetString("euclo.workflow_id")
	if workflowID == "" && in.Task != nil && in.Task.Context != nil {
		if value, ok := in.Task.Context["workflow_id"]; ok {
			if id, ok := value.(string); ok {
				workflowID = id
			}
		}
	}
	policy := euclopolicy.ResolveRetrievalPolicy(in.Mode, in.Profile)
	in.State.Set("euclo.retrieval_policy", policy)
	if expansion, err := eucloruntime.ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, in.State, policy); err != nil {
		return executionTask, err
	} else {
		return eucloruntime.ApplyContextExpansion(in.State, executionTask, expansion), nil
	}
}

func (s SessionService) runInteractive(ctx context.Context, executionTask *core.Task, env euclotypes.ExecutionEnvelope, mode euclotypes.ModeResolution) error {
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

func (s SessionService) resolveEmitter(task *core.Task) (interaction.FrameEmitter, bool, int) {
	if s.ResolveEmitter != nil {
		return s.ResolveEmitter(task, s.Emitter)
	}
	if s.Emitter != nil {
		return s.Emitter, false, 0
	}
	return &interaction.NoopEmitter{}, false, 0
}

func (s SessionService) applyVerificationAndArtifacts(ctx context.Context, in SessionInput, out *SessionOutput) {
	policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
	in.State.Set("euclo.verification_policy", policy)
	if out.Err == nil && in.Profile.MutationAllowed {
		if _, applyErr := eucloruntime.ApplyEditIntentArtifacts(ctx, s.Environment.Registry, in.State); applyErr != nil {
			out.Err = applyErr
		}
	}
	evidence := eucloruntime.NormalizeVerificationEvidence(in.State)
	in.State.Set("euclo.verification", evidence)
	var editRecord *eucloruntime.EditExecutionRecord
	if raw, ok := in.State.Get("euclo.edit_execution"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.EditExecutionRecord); ok {
			editRecord = &typed
		}
	}
	successGate := eucloruntime.EvaluateSuccessGate(policy, evidence, editRecord)
	in.State.Set("euclo.success_gate", successGate)
	if out.Result != nil {
		if out.Result.Data == nil {
			out.Result.Data = map[string]any{}
		}
		out.Result.Data["verification"] = evidence
		out.Result.Data["success_gate"] = successGate
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
	in.State.Set("euclo.action_log", actionLog)
	proofSurface := eucloreporting.BuildProofSurface(in.State, artifacts)
	in.State.Set("euclo.proof_surface", proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(in.State)
	in.State.Set("euclo.artifacts", artifacts)
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
	in.State.Set("euclo.final_report", finalReport)
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
	out.MutationCheckpoints = executionMutationCheckpointSummaries(in.State)
}

func (s SessionService) runCheckpoint(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context) error {
	if s.Checkpoint == nil {
		return nil
	}
	return s.Checkpoint(ctx, checkpoint, task, state)
}
