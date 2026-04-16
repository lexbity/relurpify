package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime/session"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
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

	resolver := interaction.NewAgencyResolver()
	interaction.RegisterHelpTriggers(resolver)
	resolver.RegisterTrigger("", session.SessionResumeTrigger())

	recordingEmitter := wrapInteractiveEmitter(emitter)
	machine := registry.Build(mode.ModeID, recordingEmitter, resolver)
	if machine == nil {
		return nil, nil, fmt.Errorf("failed to build machine for mode %q", mode.ModeID)
	}

	seedInteractiveMachine(machine, env, mode)

	// Check for initial phase jump trigger (e.g., "resume session" phrase)
	if jumpPhase := resolveInitialPhaseJump(resolver, mode.ModeID, env.Task); jumpPhase != "" {
		machine.State()["euclo.session_select.triggered"] = true
		machine.JumpToPhase(jumpPhase)
	}

	if err := maybeResumeInteractiveSession(ctx, machine, env.State, mode.ModeID); err != nil {
		return nil, nil, fmt.Errorf("interactive resume for mode %q: %w", mode.ModeID, err)
	}

	if err := machine.Run(ctx); err != nil {
		return nil, nil, fmt.Errorf("interactive execution for mode %q: %w", mode.ModeID, err)
	}

	iResult := extractInteractiveResult(machine)
	persistInteractiveState(env, machine, recordingEmitter, iResult)
	icResult := buildInteractiveControllerResult(machine, iResult)
	result := buildInteractiveResult(env.Task, mode, iResult)
	icResult.Result = result
	return result, icResult, nil
}

func maybeResumeInteractiveSession(ctx context.Context, machine *interaction.PhaseMachine, state *core.Context, modeID string) error {
	if machine == nil || state == nil {
		return nil
	}
	if resumed, _ := euclostate.GetSessionResumeConsumed(state); resumed {
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
		euclostate.SetSessionResumeConsumed(state, true)
	}
	return nil
}

func wrapInteractiveEmitter(emitter interaction.FrameEmitter) *interaction.RecordingEmitter {
	if emitter == nil {
		emitter = &interaction.NoopEmitter{}
	}
	if recordingEmitter, ok := emitter.(*interaction.RecordingEmitter); ok {
		return recordingEmitter
	}
	return interaction.NewRecordingEmitter(emitter)
}

func seedInteractiveMachine(machine *interaction.PhaseMachine, env euclotypes.ExecutionEnvelope, mode euclotypes.ModeResolution) {
	if machine == nil {
		return
	}
	if env.Task != nil {
		machine.State()["task.instruction"] = env.Task.Instruction
	}
	machine.State()["mode.id"] = mode.ModeID
	machine.State()["profile.id"] = env.Profile.ProfileID
	if env.State == nil {
		return
	}
	snapshot := env.State.Snapshot()
	for key, value := range snapshot.State {
		machine.State()[key] = value
	}
	existingArtifacts := euclotypes.ArtifactStateFromContext(env.State)
	for _, art := range existingArtifacts.All() {
		machine.Artifacts().Add(art)
	}
}

func extractInteractiveResult(machine *interaction.PhaseMachine) interaction.InteractionResult {
	if machine == nil {
		return interaction.InteractionResult{}
	}
	return interaction.ExtractInteractionResult(machine)
}

func persistInteractiveState(
	env euclotypes.ExecutionEnvelope,
	machine *interaction.PhaseMachine,
	recordingEmitter *interaction.RecordingEmitter,
	iResult interaction.InteractionResult,
) {
	defaultOrchestrateRecorder.persistInteractiveState(env, machine, recordingEmitter, iResult)
}

func buildInteractiveControllerResult(machine *interaction.PhaseMachine, iResult interaction.InteractionResult) *InteractiveControllerResult {
	iState := interaction.InteractionState{}
	if machine != nil {
		iState = interaction.ExtractInteractionState(machine)
	}
	iState.PhasesExecuted = append([]string{}, iResult.PhasesExecuted...)
	return &InteractiveControllerResult{
		Artifacts:        iResult.Artifacts,
		PhasesExecuted:   iResult.PhasesExecuted,
		TransitionTo:     iResult.TransitionTo,
		InteractionState: iState,
	}
}

func buildInteractiveResult(task *core.Task, mode euclotypes.ModeResolution, iResult interaction.InteractionResult) *core.Result {
	result := &core.Result{
		Success: true,
		NodeID:  taskNodeID(task),
		Data: map[string]any{
			"status":          "completed",
			"mode":            mode.ModeID,
			"phases_executed": iResult.PhasesExecuted,
		},
	}
	if iResult.TransitionTo != "" {
		result.Data["transition_to"] = iResult.TransitionTo
	}
	return result
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
	recordingEmitter := wrapInteractiveEmitter(emitter)

	for i := 0; i <= maxTransitions; i++ {
		result, icResult, err := pc.ExecuteInteractive(ctx, registry, currentMode, env, recordingEmitter)
		if err != nil {
			return nil, nil, err
		}

		allArtifacts = append(allArtifacts, icResult.Artifacts...)
		allPhases = append(allPhases, icResult.PhasesExecuted...)
		allSkipped = append(allSkipped, icResult.InteractionState.SkippedPhases...)

		seedTransitionCarryOver(env, carryOver)

		if icResult.TransitionTo == "" {
			return finalizeTransitionResult(result, icResult, allArtifacts, allPhases, allSkipped, env), icResult, nil
		}

		carryOver = carryOverTransitionArtifacts(icResult.Artifacts, currentMode.ModeID, icResult.TransitionTo)
		currentMode = nextInteractiveMode(icResult.TransitionTo)
	}

	return nil, nil, fmt.Errorf("exceeded max transitions (%d)", maxTransitions)
}

func seedTransitionCarryOver(env euclotypes.ExecutionEnvelope, carryOver []euclotypes.Artifact) {
	if env.State == nil || len(carryOver) == 0 {
		return
	}
	mergeCapabilityArtifactsToState(env.State, carryOver)
}

func carryOverTransitionArtifacts(artifacts []euclotypes.Artifact, fromMode, toMode string) []euclotypes.Artifact {
	bundle := interaction.NewArtifactBundle()
	for _, a := range artifacts {
		bundle.Add(a)
	}
	return interaction.CarryOverArtifacts(bundle, fromMode, toMode)
}

func nextInteractiveMode(modeID string) euclotypes.ModeResolution {
	return euclotypes.ModeResolution{
		ModeID: modeID,
		Source: "transition",
	}
}

func finalizeTransitionResult(
	result *core.Result,
	icResult *InteractiveControllerResult,
	allArtifacts []euclotypes.Artifact,
	allPhases []string,
	allSkipped []string,
	env euclotypes.ExecutionEnvelope,
) *core.Result {
	icResult.Artifacts = allArtifacts
	icResult.PhasesExecuted = allPhases
	icResult.InteractionState.PhasesExecuted = append([]string{}, allPhases...)
	icResult.InteractionState.SkippedPhases = uniqueStrings(allSkipped)
	if env.State != nil {
		env.State.Set("euclo.interaction_state", icResult.InteractionState)
	}
	result.Data["phases_executed"] = allPhases
	return result
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

// resolveInitialPhaseJump checks if the task instruction triggers a phase jump.
// Returns the target phase ID if a trigger fires, or empty string otherwise.
func resolveInitialPhaseJump(resolver *interaction.AgencyResolver, modeID string, task *core.Task) string {
	if resolver == nil || task == nil || strings.TrimSpace(task.Instruction) == "" {
		return ""
	}
	trigger, ok := resolver.Resolve(modeID, strings.TrimSpace(task.Instruction))
	if !ok || trigger == nil || trigger.PhaseJump == "" {
		return ""
	}
	return trigger.PhaseJump
}
