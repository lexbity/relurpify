package assurance

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/orchestrate"
)

// InteractionRunner handles interaction seeding and mode-machine execution.
type InteractionRunner struct {
	ProfileCtrl         *orchestrate.ProfileController
	InteractionRegistry *interaction.ModeMachineRegistry
	ResolveEmitter      EmitterResolver
	Emitter             interaction.FrameEmitter
	SeedInteraction     PrepassSeeder
	ResetDoomLoop       func()
	Environment         agentenv.AgentEnvironment
}

// Run seeds the interaction and executes the mode-machine if configured.
// It handles: interaction seeding, mode-machine execution, and doom-loop reset.
func (r InteractionRunner) Run(ctx context.Context, executionTask *core.Task, in Input) error {
	if r.ProfileCtrl == nil || r.InteractionRegistry == nil {
		return nil
	}

	// Seed interaction before running
	if r.SeedInteraction != nil {
		r.SeedInteraction(in.State, executionTask, in.Classification, in.Mode)
	}

	// Build execution envelope
	var planStore interface {
		UpsertWorkflowArtifact(context.Context, interface{}) error
	}
	if in.ServiceBundle.PlanStore != nil {
		planStore = nil // Will be handled by the envelope function
	}
	_ = planStore // Avoid unused variable error if needed

	execEnvelope := eucloruntime.BuildExecutionEnvelope(
		executionTask, in.State, in.Mode, in.Profile, r.Environment,
		in.ServiceBundle.PlanStore, nil, "", "", in.Telemetry,
	)

	// Resolve emitter
	emitter, withTransitions, maxTransitions := r.resolveEmitter(executionTask)

	// Run interactive mode-machine
	var err error
	if withTransitions {
		_, _, err = r.ProfileCtrl.ExecuteInteractiveWithTransitions(ctx, r.InteractionRegistry, in.Mode, execEnvelope, emitter, maxTransitions)
	} else {
		_, _, err = r.ProfileCtrl.ExecuteInteractive(ctx, r.InteractionRegistry, in.Mode, execEnvelope, emitter)
	}

	// Reset doom loop after interaction completes
	if r.ResetDoomLoop != nil {
		r.ResetDoomLoop()
	}

	return err
}

func (r InteractionRunner) resolveEmitter(task *core.Task) (interaction.FrameEmitter, bool, int) {
	if r.ResolveEmitter != nil {
		return r.ResolveEmitter(task, r.Emitter)
	}
	if r.Emitter != nil {
		return r.Emitter, false, 0
	}
	return &interaction.NoopEmitter{}, false, 0
}
