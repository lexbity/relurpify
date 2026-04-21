package assurance

import (
	"context"
	"fmt"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloexec "codeburg.org/lexbit/relurpify/named/euclo/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclodispatch "codeburg.org/lexbit/relurpify/named/euclo/runtime/dispatch"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/runtime/policy"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

// EmitterResolver resolves the emitter for a given task.
type EmitterResolver func(*core.Task, interaction.FrameEmitter) (interaction.FrameEmitter, bool, int)

// PrepassSeeder seeds interaction prepass data.
type PrepassSeeder func(*core.Context, *core.Task, eucloruntime.TaskClassification, euclotypes.ModeResolution)

// ArtifactPersister persists artifacts to storage.
type ArtifactPersister func(context.Context, *core.Task, *core.Context, []euclotypes.Artifact) error

// BeforeVerificationHook runs before verification.
type BeforeVerificationHook func(context.Context, *core.Task, *core.Context) error

// MutationCheckpointHook runs at mutation checkpoints.
type MutationCheckpointHook func(context.Context, archaeodomain.MutationCheckpoint, *core.Task, *core.Context) error

// Runtime holds the services needed for assurance execution.
// It is the coordination shell that wires together the four focused services.
type Runtime struct {
	Memory             memory.MemoryStore
	Environment        agentenv.AgentEnvironment
	BehaviorDispatcher *euclodispatch.Dispatcher
	Checkpoint         MutationCheckpointHook

	// Services decomposed from the monolithic assurance layer
	Expander    ContextExpander
	Interaction InteractionRunner
	Gate        VerificationGate
	Recorder    ExecutionRecorder
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

// Execute runs the full assurance execution pipeline using the decomposed services.
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

	// 1. Expand context
	expansion := s.Expander.Expand(ctx, in)
	executionTask := expansion.ExecutionTask
	if expansion.Err != nil {
		out.Err = expansion.Err
	}

	// 2. Run interaction
	if interactionErr := s.Interaction.Run(ctx, executionTask, in); interactionErr != nil && out.Err == nil {
		out.Err = interactionErr
	}

	// 3. Pre-dispatch checkpoint
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreDispatch, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}

	// 4. Dispatch
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

	// 5. Post-execution checkpoint
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPostExecution, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}

	// 6. Pre-verification checkpoint
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreVerification, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}

	// 7. Verify
	gateResult := s.Gate.Evaluate(ctx, in, in.Profile.MutationAllowed)
	if gateResult.Err != nil && out.Err == nil {
		out.Err = gateResult.Err
	}

	// Gate check: block if not allowed
	if out.Err == nil && !gateResult.SuccessGate.Allowed {
		out.Err = fmt.Errorf("euclo success gate blocked completion: %s", gateResult.SuccessGate.Reason)
	}

	// 8. Pre-finalization checkpoint
	if out.Err == nil {
		if checkpointErr := s.runCheckpoint(ctx, archaeodomain.MutationCheckpointPreFinalization, in.Task, in.State); checkpointErr != nil {
			out.Err = checkpointErr
		}
	}

	// Update result success status
	if out.Result != nil {
		out.Result.Success = out.Err == nil && gateResult.SuccessGate.Allowed && out.Result.Success
		if out.Err != nil {
			out.Result.Error = out.Err
		}
		// Add verification data to result
		if out.Result.Data == nil {
			out.Result.Data = map[string]any{}
		}
		out.Result.Data["verification"] = gateResult.Evidence
		out.Result.Data["success_gate"] = gateResult.SuccessGate
		out.Result.Data["assurance_class"] = gateResult.SuccessGate.AssuranceClass
	}

	// 9. Record
	recorded := s.Recorder.Record(ctx, in.Task, in.State, gateResult, result)
	if recorded.Err != nil && out.Err == nil {
		out.Err = recorded.Err
		if out.Result != nil {
			out.Result.Success = false
			out.Result.Error = out.Err
		}
	}

	// Populate output
	out.Artifacts = recorded.Artifacts
	out.ActionLog = recorded.ActionLog
	out.ProofSurface = recorded.ProofSurface
	out.FinalReport = recorded.FinalReport
	out.MutationCheckpoints = recorded.MutationCheckpoints

	// Populate result data
	if out.Result != nil {
		if out.Result.Data == nil {
			out.Result.Data = map[string]any{}
		}
		out.Result.Data["final_report"] = recorded.FinalReport
		out.Result.Data["action_log"] = recorded.ActionLog
		out.Result.Data["proof_surface"] = recorded.ProofSurface
	}

	return out
}

// ShortCircuit runs a minimal execution path using only the recorder.
// It skips expansion, interaction, dispatch, checkpoints, and verification.
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

	// Set verification policy (minimal path still needs policy in state)
	policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
	euclostate.SetVerificationPolicy(in.State, policy)

	// Record with a permissive gate (short circuit assumes success)
	gateResult := GateResult{
		SuccessGate: eucloruntime.SuccessGateResult{Allowed: true},
	}
	recorded := s.Recorder.Record(ctx, in.Task, in.State, gateResult, in.Result)

	// Populate output
	out.Artifacts = recorded.Artifacts
	out.ActionLog = recorded.ActionLog
	out.ProofSurface = recorded.ProofSurface
	out.FinalReport = recorded.FinalReport
	out.MutationCheckpoints = recorded.MutationCheckpoints
	out.Err = recorded.Err

	// Populate result data
	out.Result.Data["final_report"] = recorded.FinalReport
	out.Result.Data["action_log"] = recorded.ActionLog
	out.Result.Data["proof_surface"] = recorded.ProofSurface

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

func (s Runtime) runCheckpoint(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context) error {
	if s.Checkpoint == nil {
		return nil
	}
	return s.Checkpoint(ctx, checkpoint, task, state)
}
