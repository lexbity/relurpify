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
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	planningbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/planning"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/named/euclo/thoughtrecipes"
)

type Dispatcher struct {
	env            agentenv.AgentEnvironment
	invocables     map[string]execution.Invocable
	recipeRegistry *thoughtrecipes.PlanRegistry
	recipeExecutor *thoughtrecipes.Executor
}

func NewDispatcher(env agentenv.AgentEnvironment) *Dispatcher {
	d := &Dispatcher{
		env:        env,
		invocables: map[string]execution.Invocable{},
	}
	// Register all primary capabilities as invocables.
	for _, invocable := range []execution.Invocable{
		// Chat capabilities
		chatbehavior.NewAskInvocable(),
		chatbehavior.NewInspectInvocable(),
		chatbehavior.NewImplementInvocable(),
		// Debug capabilities
		debugbehavior.NewInvestigateRepairInvocable(),
		debugbehavior.NewSimpleRepairInvocable(),
		// Archaeology capabilities
		archaeologybehavior.NewExploreInvocable(),
		archaeologybehavior.NewCompilePlanInvocable(),
		archaeologybehavior.NewImplementPlanInvocable(),
		// Planning capabilities (BKC)
		planningbehavior.NewInvocable(euclorelurpic.CapabilityBKCCompile, bkccap.NewCompileCapability(env)),
		planningbehavior.NewInvocable(euclorelurpic.CapabilityBKCStream, bkccap.NewStreamCapability(env)),
		planningbehavior.NewInvocable(euclorelurpic.CapabilityBKCCheckpoint, bkccap.NewCheckpointCapability(env)),
		planningbehavior.NewInvocable(euclorelurpic.CapabilityBKCInvalidate, bkccap.NewInvalidateCapability(env)),
	} {
		d.invocables[invocable.ID()] = invocable
	}
	for _, invocable := range append(append(chatbehavior.NewSupportingRoutines(), debugbehavior.NewSupportingRoutines()...), archaeologybehavior.NewSupportingInvocables()...) {
		d.invocables[invocable.ID()] = invocable
	}
	for _, invocable := range []execution.Invocable{localbehavior.DeferralsSurfaceRoutine{}, localbehavior.LearningPromoteRoutine{}} {
		d.invocables[invocable.ID()] = invocable
	}
	return d
}

func (d *Dispatcher) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	invokeInput := execution.InvokeInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: d.InvokeSupporting,
	}
	return d.Invoke(ctx, invokeInput)
}

func (d *Dispatcher) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
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

	invocableID := strings.TrimSpace(in.Work.PrimaryRelurpicCapabilityID)
	if invocableID == "" {
		return nil, fmt.Errorf("relurpic behavior unavailable: no capability ID provided")
	}

	// Check if this is a thought recipe capability ID
	if strings.HasPrefix(invocableID, "euclo:recipe.") && d.recipeRegistry != nil && d.recipeExecutor != nil {
		plan, ok := d.recipeRegistry.Get(invocableID)
		if ok {
			recipeResult, err := d.recipeExecutor.Execute(ctx, plan, in.Task, in.Environment)
			if err != nil {
				return &core.Result{Success: false, Error: err}, err
			}
			return recipeResultToCoreResult(recipeResult), nil
		}
		return nil, fmt.Errorf("thought recipe %q not found", invocableID)
	}

	invocable, ok := d.invocables[invocableID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable", invocableID)
	}
	return invocable.Invoke(ctx, in)
}

// SetRecipeRegistry sets the recipe registry and executor for thought recipe support.
func (d *Dispatcher) SetRecipeRegistry(registry *thoughtrecipes.PlanRegistry, executor *thoughtrecipes.Executor) {
	if d == nil {
		return
	}
	d.recipeRegistry = registry
	d.recipeExecutor = executor
}

// RegisterSupporting adds or replaces a supporting routine in the dispatcher.
func (d *Dispatcher) RegisterSupporting(invocable execution.Invocable) {
	if d == nil || invocable == nil {
		return
	}
	id := strings.TrimSpace(invocable.ID())
	if id == "" {
		return
	}
	if d.invocables == nil {
		d.invocables = map[string]execution.Invocable{}
	}
	d.invocables[id] = invocable
}

// recipeResultToCoreResult converts a RecipeResult to a core.Result.
func recipeResultToCoreResult(recipeResult *thoughtrecipes.RecipeResult) *core.Result {
	if recipeResult == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("nil recipe result")}
	}

	data := map[string]any{
		"recipe_id": recipeResult.RecipeID,
		"artifacts": recipeResult.Artifacts,
		"warnings":  recipeResult.Warnings,
	}

	// Include final result data if present
	if recipeResult.FinalResult != nil && recipeResult.FinalResult.Data != nil {
		for k, v := range recipeResult.FinalResult.Data {
			if _, exists := data[k]; !exists {
				data[k] = v
			}
		}
	}

	return &core.Result{
		Success: recipeResult.Success,
		Data:    data,
		Error:   nil,
	}
}

// ExecuteSequence executes a sequence of capabilities (AND or OR).
// For AND: executes all capabilities sequentially, accumulating state.
// For OR: executes only the first capability (the "best" one selected by classifier).
func (d *Dispatcher) ExecuteSequence(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}

	sequence := in.Work.CapabilityExecutionSequence
	if len(sequence) == 0 {
		return nil, fmt.Errorf("empty capability execution sequence")
	}

	// Single element: use regular Execute path
	if len(sequence) == 1 {
		invocableID := strings.TrimSpace(sequence[0])
		invocable, ok := d.invocables[invocableID]
		if !ok {
			return nil, fmt.Errorf("relurpic behavior %q unavailable", invocableID)
		}
		return invocable.Invoke(ctx, execution.InvokeInput{
			Task:             in.Task,
			ExecutionTask:    in.ExecutionTask,
			State:            in.State,
			Mode:             in.Mode,
			Profile:          in.Profile,
			Work:             in.Work,
			Environment:      in.Environment,
			ServiceBundle:    in.ServiceBundle,
			WorkflowExecutor: in.WorkflowExecutor,
			Telemetry:        in.Telemetry,
			InvokeSupporting: d.InvokeSupporting,
		})
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
func (d *Dispatcher) executeANDSequence(ctx context.Context, in execution.InvokeInput, sequence []string) (*core.Result, error) {
	var lastResult *core.Result
	var lastErr error

	for i, capabilityID := range sequence {
		invocableID := strings.TrimSpace(capabilityID)
		invocable, ok := d.invocables[invocableID]
		if !ok {
			return nil, fmt.Errorf("relurpic behavior %q unavailable (step %d)", invocableID, i+1)
		}

		// Update work for this step
		stepWork := in.Work
		stepWork.PrimaryRelurpicCapabilityID = invocableID

		stepInput := execution.InvokeInput{
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
			InvokeSupporting:     d.InvokeSupporting,
		}

		lastResult, lastErr = invocable.Invoke(ctx, stepInput)
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
func (d *Dispatcher) executeORSequence(ctx context.Context, in execution.InvokeInput, sequence []string) (*core.Result, error) {
	if len(sequence) == 0 {
		return nil, fmt.Errorf("empty capability sequence for OR execution")
	}

	// Execute only the first capability
	capabilityID := sequence[0]
	invocableID := strings.TrimSpace(capabilityID)
	invocable, ok := d.invocables[invocableID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable (OR step)", invocableID)
	}

	// Update work for this step
	stepWork := in.Work
	stepWork.PrimaryRelurpicCapabilityID = invocableID

	stepInput := execution.InvokeInput{
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
		InvokeSupporting:     d.InvokeSupporting,
	}

	result, err := invocable.Invoke(ctx, stepInput)
	if err != nil {
		return result, fmt.Errorf("OR capability execution (%s) failed: %w", capabilityID, err)
	}

	// Record which capability was selected in state for observability
	if in.State != nil {
		in.State.Set("euclo.or_selected_capability", capabilityID)
	}

	return result, nil
}

func (d *Dispatcher) InvokeSupporting(ctx context.Context, routineID string, in execution.InvokeInput) ([]euclotypes.Artifact, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	routineID = strings.TrimSpace(routineID)
	invocable, ok := d.invocables[routineID]
	if !ok {
		return nil, fmt.Errorf("routine %q not registered in dispatcher", routineID)
	}
	result, err := invocable.Invoke(ctx, in)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Success {
		if result != nil && result.Error != nil {
			return nil, result.Error
		}
		return nil, fmt.Errorf("routine %q returned unsuccessful result", routineID)
	}
	if result.Data != nil {
		if artifacts, ok := result.Data["artifacts"].([]euclotypes.Artifact); ok {
			return artifacts, nil
		}
	}
	return nil, nil
}

func (d *Dispatcher) ExecuteRoutine(ctx context.Context, routineID string, task *core.Task, state *core.Context, work runtimepkg.UnitOfWork, env agentenv.AgentEnvironment, bundle execution.ServiceBundle) ([]euclotypes.Artifact, error) {
	return d.InvokeSupporting(ctx, routineID, execution.InvokeInput{
		Task:          task,
		State:         state,
		Work:          work,
		Environment:   env,
		ServiceBundle: bundle,
	})
}
