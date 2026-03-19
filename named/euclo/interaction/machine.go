package interaction

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// ArtifactBundle holds artifacts produced during machine execution.
type ArtifactBundle struct {
	artifacts []euclotypes.Artifact
}

// NewArtifactBundle creates a new empty bundle.
func NewArtifactBundle() *ArtifactBundle {
	return &ArtifactBundle{}
}

// Add appends an artifact to the bundle.
func (b *ArtifactBundle) Add(a euclotypes.Artifact) {
	b.artifacts = append(b.artifacts, a)
}

// All returns all artifacts in the bundle.
func (b *ArtifactBundle) All() []euclotypes.Artifact {
	return b.artifacts
}

// OfKind returns artifacts matching the given kind.
func (b *ArtifactBundle) OfKind(kind euclotypes.ArtifactKind) []euclotypes.Artifact {
	var out []euclotypes.Artifact
	for _, a := range b.artifacts {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out
}

// Has returns true if the bundle contains at least one artifact of the given kind.
func (b *ArtifactBundle) Has(kind euclotypes.ArtifactKind) bool {
	for _, a := range b.artifacts {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// PhaseHandler executes a single phase of the interaction machine.
type PhaseHandler interface {
	Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error)
}

// PhaseMachineContext provides the execution context for a phase handler.
type PhaseMachineContext struct {
	Emitter    FrameEmitter
	State      map[string]any
	Artifacts  *ArtifactBundle
	Mode       string
	Phase      string
	PhaseIndex int
	PhaseCount int
}

// PhaseOutcome is the result of executing a single phase.
type PhaseOutcome struct {
	Advance      bool
	JumpTo       string
	Transition   string
	Artifacts    []euclotypes.Artifact
	StateUpdates map[string]any
}

// PhaseDefinition describes a phase in the machine.
type PhaseDefinition struct {
	ID         string
	Label      string
	Handler    PhaseHandler
	Skippable  bool
	SkipWhen   func(state map[string]any, artifacts *ArtifactBundle) bool
	EnterGuard func(state map[string]any, artifacts *ArtifactBundle) error
}

// PhaseMachine is the generic state machine that drives mode interactions.
type PhaseMachine struct {
	mode           string
	phases         []PhaseDefinition
	current        int
	emitter        FrameEmitter
	artifacts      *ArtifactBundle
	state          map[string]any
	resolver       *AgencyResolver
	executedPhases []string
	skippedPhases  []string
}

// PhaseMachineConfig configures a new PhaseMachine.
type PhaseMachineConfig struct {
	Mode     string
	Phases   []PhaseDefinition
	Emitter  FrameEmitter
	Resolver *AgencyResolver
}

// NewPhaseMachine creates a new phase machine with the given configuration.
func NewPhaseMachine(cfg PhaseMachineConfig) *PhaseMachine {
	return &PhaseMachine{
		mode:      cfg.Mode,
		phases:    cfg.Phases,
		emitter:   cfg.Emitter,
		artifacts: NewArtifactBundle(),
		state:     make(map[string]any),
		resolver:  cfg.Resolver,
	}
}

// State returns the machine's state map.
func (m *PhaseMachine) State() map[string]any {
	return m.state
}

// Artifacts returns the machine's artifact bundle.
func (m *PhaseMachine) Artifacts() *ArtifactBundle {
	return m.artifacts
}

// Emitter returns the machine's frame emitter.
func (m *PhaseMachine) Emitter() FrameEmitter {
	return m.emitter
}

// CurrentPhase returns the ID of the current phase, or empty if done.
func (m *PhaseMachine) CurrentPhase() string {
	if m.current >= len(m.phases) {
		return ""
	}
	return m.phases[m.current].ID
}

// ExecutedPhases returns the phases actually executed during this machine run.
func (m *PhaseMachine) ExecutedPhases() []string {
	if m == nil || len(m.executedPhases) == 0 {
		return nil
	}
	out := make([]string, len(m.executedPhases))
	copy(out, m.executedPhases)
	return out
}

// SkippedPhases returns the phases actually skipped during this machine run.
func (m *PhaseMachine) SkippedPhases() []string {
	if m == nil || len(m.skippedPhases) == 0 {
		return nil
	}
	out := make([]string, len(m.skippedPhases))
	copy(out, m.skippedPhases)
	return out
}

// JumpToPhase moves the machine cursor to the named phase.
func (m *PhaseMachine) JumpToPhase(id string) bool {
	idx := m.phaseIndex(id)
	if idx < 0 {
		return false
	}
	m.current = idx
	return true
}

// Run executes the phase machine from the current phase to completion.
func (m *PhaseMachine) Run(ctx context.Context) error {
	for m.current < len(m.phases) {
		if err := ctx.Err(); err != nil {
			return err
		}

		phase := m.phases[m.current]

		// Auto-skip check.
		if phase.SkipWhen != nil && phase.SkipWhen(m.state, m.artifacts) {
			m.skippedPhases = append(m.skippedPhases, phase.ID)
			m.current++
			continue
		}

		// Enter guard.
		if phase.EnterGuard != nil {
			if err := phase.EnterGuard(m.state, m.artifacts); err != nil {
				return fmt.Errorf("phase %q guard: %w", phase.ID, err)
			}
		}

		mc := PhaseMachineContext{
			Emitter:    m.emitter,
			State:      m.state,
			Artifacts:  m.artifacts,
			Mode:       m.mode,
			Phase:      phase.ID,
			PhaseIndex: m.current,
			PhaseCount: len(m.phases),
		}

		consumedKinds := m.artifactKinds()

		outcome, err := phase.Handler.Execute(ctx, mc)
		if err != nil {
			return fmt.Errorf("phase %q: %w", phase.ID, err)
		}
		m.executedPhases = append(m.executedPhases, phase.ID)
		if recording, ok := m.emitter.(*RecordingEmitter); ok && recording != nil && recording.Recording != nil {
			recording.Recording.RecordPhaseArtifacts(phase.ID, m.mode, outcome.Artifacts, consumedKinds)
		}

		// Merge artifacts.
		for _, a := range outcome.Artifacts {
			m.artifacts.Add(a)
		}

		// Merge state updates.
		for k, v := range outcome.StateUpdates {
			m.state[k] = v
		}

		// Handle jump.
		if outcome.JumpTo != "" {
			idx := m.phaseIndex(outcome.JumpTo)
			if idx < 0 {
				return fmt.Errorf("phase %q: jump target %q not found", phase.ID, outcome.JumpTo)
			}
			m.current = idx
			continue
		}

		// Handle transition proposal.
		if outcome.Transition != "" {
			if err := m.proposeTransition(ctx, phase.ID, outcome.Transition); err != nil {
				return err
			}
		}

		// Advance or stop.
		if !outcome.Advance {
			break
		}
		m.current++
	}
	return nil
}

func (m *PhaseMachine) artifactKinds() []euclotypes.ArtifactKind {
	if m == nil || m.artifacts == nil {
		return nil
	}
	all := m.artifacts.All()
	if len(all) == 0 {
		return nil
	}
	out := make([]euclotypes.ArtifactKind, 0, len(all))
	seen := make(map[euclotypes.ArtifactKind]struct{}, len(all))
	for _, artifact := range all {
		if artifact.Kind == "" {
			continue
		}
		if _, ok := seen[artifact.Kind]; ok {
			continue
		}
		seen[artifact.Kind] = struct{}{}
		out = append(out, artifact.Kind)
	}
	return out
}

// proposeTransition emits a transition frame and waits for user response.
func (m *PhaseMachine) proposeTransition(ctx context.Context, fromPhase, toMode string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	frame := InteractionFrame{
		Kind:  FrameTransition,
		Mode:  m.mode,
		Phase: fromPhase,
		Content: TransitionContent{
			FromMode: m.mode,
			ToMode:   toMode,
			Reason:   fmt.Sprintf("Transition proposed from %s to %s", m.mode, toMode),
		},
		Actions: []ActionSlot{
			{ID: "accept", Label: "Switch to " + toMode, Kind: ActionConfirm, Default: true},
			{ID: "reject", Label: "Stay in " + m.mode, Kind: ActionConfirm},
		},
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: m.current,
			PhaseCount: len(m.phases),
		},
	}

	if err := m.emitter.Emit(ctx, frame); err != nil {
		return fmt.Errorf("emit transition: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	resp, err := m.emitter.AwaitResponse(ctx)
	if err != nil {
		return fmt.Errorf("await transition response: %w", err)
	}
	if recording, ok := m.emitter.(*RecordingEmitter); ok && recording != nil && recording.Recording != nil {
		recording.Recording.RecordTransition(m.mode, toMode, fromPhase)
	}

	// Store the transition decision in state for the controller to act on.
	if resp.ActionID == "accept" {
		m.state["transition.accepted"] = toMode
	} else {
		m.state["transition.rejected"] = toMode
	}
	return nil
}

// phaseIndex returns the index of the phase with the given ID, or -1.
func (m *PhaseMachine) phaseIndex(id string) int {
	for i, p := range m.phases {
		if p.ID == id {
			return i
		}
	}
	return -1
}
