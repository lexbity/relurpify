package orchestrate

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// InteractiveControllerResult captures the output of interactive execution.
type InteractiveControllerResult struct {
	Result           *core.Result
	Artifacts        []euclotypes.Artifact
	PhasesExecuted   []string
	TransitionTo     string
	InteractionState interaction.InteractionState
}

// ExecuteInteractive runs a mode's phase machine with the given emitter.
// For non-interactive (batch) execution, pass interaction.NoopEmitter{} —
// phases auto-advance with defaults, preserving backward compatibility.
func (pc *ProfileController) ExecuteInteractive(
	ctx context.Context,
	registry *interaction.ModeMachineRegistry,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
	emitter interaction.FrameEmitter,
) (*core.Result, *InteractiveControllerResult, error) {
	if registry == nil || !registry.Has(mode.ModeID) {
		return nil, nil, fmt.Errorf("no interactive mode registered for %q", mode.ModeID)
	}

	recordingEmitter, ok := emitter.(*interaction.RecordingEmitter)
	if !ok {
		recordingEmitter = interaction.NewRecordingEmitter(emitter)
	}

	resolver := interaction.NewAgencyResolver()
	interaction.RegisterHelpTriggers(resolver)

	machine := registry.Build(mode.ModeID, recordingEmitter, resolver)
	if machine == nil {
		return nil, nil, fmt.Errorf("failed to build machine for mode %q", mode.ModeID)
	}

	// Seed state from execution envelope.
	if env.Task != nil {
		machine.State()["task.instruction"] = env.Task.Instruction
	}
	machine.State()["mode.id"] = mode.ModeID
	machine.State()["profile.id"] = env.Profile.ProfileID

	// Seed any existing artifacts from the execution context.
	if env.State != nil {
		snapshot := env.State.Snapshot()
		for key, value := range snapshot.State {
			machine.State()[key] = value
		}
		existingArtifacts := euclotypes.ArtifactStateFromContext(env.State)
		for _, art := range existingArtifacts.All() {
			machine.Artifacts().Add(art)
		}
	}

	if env.State != nil {
		if err := maybeResumeInteractiveSession(ctx, machine, env.State, mode.ModeID); err != nil {
			return nil, nil, fmt.Errorf("interactive resume for mode %q: %w", mode.ModeID, err)
		}
	}

	// Run the machine.
	if err := machine.Run(ctx); err != nil {
		return nil, nil, fmt.Errorf("interactive execution for mode %q: %w", mode.ModeID, err)
	}

	// Extract results.
	iResult := interaction.ExtractInteractionResult(machine)

	// Merge interaction artifacts back into the execution state.
	if env.State != nil {
		mergeCapabilityArtifactsToState(env.State, iResult.Artifacts)
	}

	// Persist interaction state.
	iState := interaction.ExtractInteractionState(machine)
	iState.PhasesExecuted = append([]string{}, iResult.PhasesExecuted...)
	if env.State != nil {
		env.State.Set("euclo.interaction_state", iState)
		env.State.Set("euclo.interaction_recording", recordingEmitter.Recording.ToStateMap())
		env.State.Set("euclo.interaction_records", recordingEmitter.Recording.Records())
		if raw, ok := machine.State()["propose.items"]; ok && raw != nil {
			env.State.Set("pipeline.plan", map[string]any{
				"source": "interaction.propose",
				"items":  raw,
			})
		}
	}

	icResult := &InteractiveControllerResult{
		Artifacts:        iResult.Artifacts,
		PhasesExecuted:   iResult.PhasesExecuted,
		TransitionTo:     iResult.TransitionTo,
		InteractionState: iState,
	}

	result := &core.Result{
		Success: true,
		NodeID:  taskNodeID(env.Task),
		Data: map[string]any{
			"status":          "completed",
			"mode":            mode.ModeID,
			"phases_executed": iResult.PhasesExecuted,
		},
	}

	// If a transition was accepted, include it in the result.
	if iResult.TransitionTo != "" {
		result.Data["transition_to"] = iResult.TransitionTo
	}

	icResult.Result = result
	return result, icResult, nil
}

func maybeResumeInteractiveSession(ctx context.Context, machine *interaction.PhaseMachine, state *core.Context, modeID string) error {
	if machine == nil || state == nil {
		return nil
	}
	if resumed, _ := state.Get("euclo.session_resume_consumed"); resumed == true {
		return nil
	}
	resume := interaction.ExtractSessionResume(state)
	if resume == nil || resume.Mode == "" || resume.Mode != modeID || resume.LastPhase == "" {
		return nil
	}
	frame := interaction.BuildResumeFrame(resume)
	if err := machine.Emitter().Emit(ctx, frame); err != nil {
		return fmt.Errorf("emit resume frame: %w", err)
	}
	resp, err := machine.Emitter().AwaitResponse(ctx)
	if err != nil {
		return fmt.Errorf("await resume response: %w", err)
	}
	if interaction.HandleResumeResponse(resp) == "resume" {
		interaction.ApplySessionResume(machine, resume)
		state.Set("euclo.session_resume_consumed", true)
	}
	return nil
}

// ExecuteInteractiveWithTransitions runs interactive execution and handles
// mode transitions by building new machines for the target mode.
func (pc *ProfileController) ExecuteInteractiveWithTransitions(
	ctx context.Context,
	registry *interaction.ModeMachineRegistry,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
	emitter interaction.FrameEmitter,
	maxTransitions int,
) (*core.Result, *InteractiveControllerResult, error) {
	if maxTransitions <= 0 {
		maxTransitions = 5
	}

	currentMode := mode
	var allArtifacts []euclotypes.Artifact
	var allPhases []string
	var allSkipped []string
	var carryOver []euclotypes.Artifact
	recordingEmitter, ok := emitter.(*interaction.RecordingEmitter)
	if !ok {
		recordingEmitter = interaction.NewRecordingEmitter(emitter)
	}

	for i := 0; i <= maxTransitions; i++ {
		result, icResult, err := pc.ExecuteInteractive(ctx, registry, currentMode, env, recordingEmitter)
		if err != nil {
			return nil, nil, err
		}

		allArtifacts = append(allArtifacts, icResult.Artifacts...)
		allPhases = append(allPhases, icResult.PhasesExecuted...)
		allSkipped = append(allSkipped, icResult.InteractionState.SkippedPhases...)

		// Seed carry-over artifacts into the new machine's state.
		if len(carryOver) > 0 && env.State != nil {
			mergeCapabilityArtifactsToState(env.State, carryOver)
		}

		if icResult.TransitionTo == "" {
			// No transition — we're done.
			icResult.Artifacts = allArtifacts
			icResult.PhasesExecuted = allPhases
			icResult.InteractionState.PhasesExecuted = append([]string{}, allPhases...)
			icResult.InteractionState.SkippedPhases = uniqueStrings(allSkipped)
			if env.State != nil {
				env.State.Set("euclo.interaction_state", icResult.InteractionState)
			}
			result.Data["phases_executed"] = allPhases
			return result, icResult, nil
		}

		// Prepare carry-over artifacts for the transition.
		bundle := interaction.NewArtifactBundle()
		for _, a := range icResult.Artifacts {
			bundle.Add(a)
		}
		carryOver = interaction.CarryOverArtifacts(bundle, currentMode.ModeID, icResult.TransitionTo)

		currentMode = euclotypes.ModeResolution{
			ModeID: icResult.TransitionTo,
			Source: "transition",
		}
	}

	return nil, nil, fmt.Errorf("exceeded max transitions (%d)", maxTransitions)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
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
