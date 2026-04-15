package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	archaeologybehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/archaeology"
	bkccap "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/bkc"
	chatbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/chat"
	debugbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/debug"
	planningbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/planning"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// behaviorRoutineAdapter wraps an execution.Behavior as a SupportingRoutine,
// enabling it to be called via ExecuteRoutine (used by capability_direct_run).
type behaviorRoutineAdapter struct {
	id       string
	behavior execution.Behavior
}

func (a *behaviorRoutineAdapter) ID() string { return a.id }

func (a *behaviorRoutineAdapter) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	execInput := execution.ExecuteInput{
		Task:          in.Task,
		State:         in.State,
		Environment:   in.Environment,
		ServiceBundle: execution.ServiceBundle{},
	}
	// Type assert ServiceBundle from the any field
	if in.ServiceBundle != nil {
		if sb, ok := in.ServiceBundle.(execution.ServiceBundle); ok {
			execInput.ServiceBundle = sb
		}
	}
	result, err := a.behavior.Execute(ctx, execInput)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Success {
		if result != nil && result.Error != nil {
			return nil, result.Error
		}
		return nil, fmt.Errorf("behavior %q returned unsuccessful result", a.id)
	}
	// Extract artifacts from result data if present.
	if result.Data != nil {
		if artifacts, ok := result.Data["artifacts"].([]euclotypes.Artifact); ok {
			return artifacts, nil
		}
	}
	return nil, nil
}

type Dispatcher struct {
	env       agentenv.AgentEnvironment
	behaviors map[string]execution.Behavior
	routines  map[string]euclorelurpic.SupportingRoutine
}

func NewDispatcher(env agentenv.AgentEnvironment) *Dispatcher {
	d := &Dispatcher{
		env:       env,
		behaviors: map[string]execution.Behavior{},
		routines:  map[string]euclorelurpic.SupportingRoutine{},
	}
	for _, behavior := range []execution.Behavior{
		chatbehavior.NewAskBehavior(),
		chatbehavior.NewInspectBehavior(),
		chatbehavior.NewImplementBehavior(),
		debugbehavior.NewInvestigateRepairBehavior(),
		debugbehavior.NewSimpleRepairBehavior(),
		archaeologybehavior.NewExploreBehavior(),
		archaeologybehavior.NewCompilePlanBehavior(),
		archaeologybehavior.NewImplementPlanBehavior(),
		// Phase A: Register BKC capabilities via PlanningBehavior
		planningbehavior.New(euclorelurpic.CapabilityBKCCompile, bkccap.NewCompileCapability(env)),
		planningbehavior.New(euclorelurpic.CapabilityBKCStream, bkccap.NewStreamCapability(env)),
		planningbehavior.New(euclorelurpic.CapabilityBKCCheckpoint, bkccap.NewCheckpointCapability(env)),
		planningbehavior.New(euclorelurpic.CapabilityBKCInvalidate, bkccap.NewInvalidateCapability(env)),
	} {
		d.behaviors[behavior.ID()] = behavior
	}
	for _, routine := range append(append(chatbehavior.NewSupportingRoutines(), debugbehavior.NewSupportingRoutines()...), archaeologybehavior.NewSupportingRoutines()...) {
		d.routines[routine.ID()] = routine
	}
	// Register BKC behaviors as routines so capability_direct_run can reach them.
	bkcBehaviorIDs := []string{
		euclorelurpic.CapabilityBKCCompile,
		euclorelurpic.CapabilityBKCStream,
		euclorelurpic.CapabilityBKCCheckpoint,
		euclorelurpic.CapabilityBKCInvalidate,
	}
	for _, id := range bkcBehaviorIDs {
		if b, ok := d.behaviors[id]; ok {
			d.routines[id] = &behaviorRoutineAdapter{id: id, behavior: b}
		}
	}
	return d
}

func (d *Dispatcher) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}

	// Check if we have a capability sequence
	seqLen := len(in.Work.CapabilityExecutionSequence)
	if seqLen > 1 {
		// Multiple elements: use sequence execution
		return d.ExecuteSequence(ctx, in)
	} else if seqLen == 1 {
		// Single element in sequence: extract it and use regular Execute path
		in.Work.PrimaryRelurpicCapabilityID = in.Work.CapabilityExecutionSequence[0]
	}

	behaviorID := strings.TrimSpace(in.Work.PrimaryRelurpicCapabilityID)
	if behaviorID == "" {
		return nil, fmt.Errorf("relurpic behavior unavailable: no capability ID provided")
	}
	behavior, ok := d.behaviors[behaviorID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable", behaviorID)
	}
	in.RunSupportingRoutine = d.ExecuteRoutine
	return behavior.Execute(ctx, in)
}

// ExecuteSequence executes a sequence of capabilities (AND or OR).
// For AND: executes all capabilities sequentially, accumulating state.
// For OR: executes only the first capability (the "best" one selected by classifier).
func (d *Dispatcher) ExecuteSequence(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}

	sequence := in.Work.CapabilityExecutionSequence
	if len(sequence) == 0 {
		return nil, fmt.Errorf("empty capability execution sequence")
	}

	// Single element: use regular Execute path
	if len(sequence) == 1 {
		behaviorID := strings.TrimSpace(sequence[0])
		behavior, ok := d.behaviors[behaviorID]
		if !ok {
			return nil, fmt.Errorf("relurpic behavior %q unavailable", behaviorID)
		}
		in.RunSupportingRoutine = d.ExecuteRoutine
		return behavior.Execute(ctx, in)
	}

	// Multiple elements: handle according to operator
	operator := in.Work.CapabilitySequenceOperator
	if operator == "" {
		operator = "AND" // default
	}

	switch operator {
	case "AND":
		return d.executeANDSequence(ctx, in, sequence)
	case "OR":
		return d.executeORSequence(ctx, in, sequence)
	default:
		return nil, fmt.Errorf("unknown capability sequence operator: %q", operator)
	}
}

// executeANDSequence executes all capabilities sequentially, accumulating state.
// Stops on first failure.
func (d *Dispatcher) executeANDSequence(ctx context.Context, in execution.ExecuteInput, sequence []string) (*core.Result, error) {
	var lastResult *core.Result
	var lastErr error

	for i, capabilityID := range sequence {
		behaviorID := strings.TrimSpace(capabilityID)
		behavior, ok := d.behaviors[behaviorID]
		if !ok {
			return nil, fmt.Errorf("relurpic behavior %q unavailable (step %d)", behaviorID, i+1)
		}

		// Update work for this step
		stepWork := in.Work
		stepWork.PrimaryRelurpicCapabilityID = behaviorID

		stepInput := execution.ExecuteInput{
			Task:                 in.Task,
			ExecutionTask:        in.ExecutionTask,
			State:                in.State,
			Mode:                 in.Mode,
			Profile:              in.Profile,
			Work:                 stepWork,
			Environment:          in.Environment,
			ServiceBundle:        in.ServiceBundle,
			WorkflowExecutor:     in.WorkflowExecutor,
			Telemetry:            in.Telemetry,
			RunSupportingRoutine: d.ExecuteRoutine,
		}

		lastResult, lastErr = behavior.Execute(ctx, stepInput)
		if lastErr != nil {
			return lastResult, fmt.Errorf("capability sequence step %d (%s) failed: %w", i+1, capabilityID, lastErr)
		}

		// Record step completion in state for observability
		if in.State != nil {
			in.State.Set(fmt.Sprintf("euclo.sequence_step_%d_completed", i+1), capabilityID)
		}
	}

	return lastResult, nil
}

// executeORSequence executes only the first (best) capability in the sequence.
// The OR semantics mean "pick the best one" which was already determined by the classifier.
func (d *Dispatcher) executeORSequence(ctx context.Context, in execution.ExecuteInput, sequence []string) (*core.Result, error) {
	if len(sequence) == 0 {
		return nil, fmt.Errorf("empty capability sequence for OR execution")
	}

	// Execute only the first capability
	capabilityID := sequence[0]
	behaviorID := strings.TrimSpace(capabilityID)
	behavior, ok := d.behaviors[behaviorID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable (OR step)", behaviorID)
	}

	// Update work for this step
	stepWork := in.Work
	stepWork.PrimaryRelurpicCapabilityID = behaviorID

	stepInput := execution.ExecuteInput{
		Task:                 in.Task,
		ExecutionTask:        in.ExecutionTask,
		State:                in.State,
		Mode:                 in.Mode,
		Profile:              in.Profile,
		Work:                 stepWork,
		Environment:          in.Environment,
		ServiceBundle:        in.ServiceBundle,
		WorkflowExecutor:     in.WorkflowExecutor,
		Telemetry:            in.Telemetry,
		RunSupportingRoutine: d.ExecuteRoutine,
	}

	result, err := behavior.Execute(ctx, stepInput)
	if err != nil {
		return result, fmt.Errorf("OR capability execution (%s) failed: %w", capabilityID, err)
	}

	// Record which capability was selected in state for observability
	if in.State != nil {
		in.State.Set("euclo.or_selected_capability", capabilityID)
	}

	return result, nil
}

func (d *Dispatcher) ExecuteRoutine(ctx context.Context, routineID string, task *core.Task, state *core.Context, work runtimepkg.UnitOfWork, env agentenv.AgentEnvironment, bundle execution.ServiceBundle) ([]euclotypes.Artifact, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	routineID = strings.TrimSpace(routineID)
	routine, ok := d.routines[routineID]
	if !ok {
		return nil, fmt.Errorf("routine %q not registered in dispatcher", routineID)
	}
	return routine.Execute(ctx, euclorelurpic.RoutineInput{
		Task:  task,
		State: state,
		Work: euclorelurpic.WorkContext{
			PrimaryCapabilityID:             work.PrimaryRelurpicCapabilityID,
			SupportingRelurpicCapabilityIDs: append([]string(nil), work.SupportingRelurpicCapabilityIDs...),
			PatternRefs:                     append([]string(nil), work.SemanticInputs.PatternRefs...),
			TensionRefs:                     append([]string(nil), work.SemanticInputs.TensionRefs...),
			ProspectiveRefs:                 append([]string(nil), work.SemanticInputs.ProspectiveRefs...),
			ConvergenceRefs:                 append([]string(nil), work.SemanticInputs.ConvergenceRefs...),
			RequestProvenanceRefs:           append([]string(nil), work.SemanticInputs.RequestProvenanceRefs...),
		},
		Environment:   env,
		ServiceBundle: bundle,
	})
}
