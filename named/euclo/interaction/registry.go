package interaction

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// ModeMachineFactory builds a PhaseMachine for a given mode.
// The emitter and resolver are provided by the caller (controller).
type ModeMachineFactory func(emitter FrameEmitter, resolver *AgencyResolver) *PhaseMachine

// ModeMachineRegistry maps mode IDs to their machine factories.
type ModeMachineRegistry struct {
	factories map[string]ModeMachineFactory
}

// NewModeMachineRegistry creates an empty registry.
func NewModeMachineRegistry() *ModeMachineRegistry {
	return &ModeMachineRegistry{factories: make(map[string]ModeMachineFactory)}
}

// Register adds a factory for the given mode ID.
func (r *ModeMachineRegistry) Register(modeID string, factory ModeMachineFactory) {
	r.factories[modeID] = factory
}

// Build constructs a PhaseMachine for the given mode, or returns nil if unregistered.
func (r *ModeMachineRegistry) Build(modeID string, emitter FrameEmitter, resolver *AgencyResolver) *PhaseMachine {
	factory, ok := r.factories[modeID]
	if !ok {
		return nil
	}
	return factory(emitter, resolver)
}

// Has returns true if a factory is registered for the mode.
func (r *ModeMachineRegistry) Has(modeID string) bool {
	_, ok := r.factories[modeID]
	return ok
}

// Modes returns all registered mode IDs.
func (r *ModeMachineRegistry) Modes() []string {
	out := make([]string, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	return out
}

// InteractionState captures the interaction progress for persistence/resume.
type InteractionState struct {
	Mode           string            `json:"mode"`
	CurrentPhase   string            `json:"current_phase"`
	PhaseStates    map[string]any    `json:"phase_states"`
	Selections     map[string]string `json:"selections"`
	PhasesExecuted []string          `json:"phases_executed,omitempty"`
	SkippedPhases  []string          `json:"skipped_phases"`
}

// InteractionResult is the output of an interactive execution.
type InteractionResult struct {
	Artifacts      []euclotypes.Artifact
	State          map[string]any
	TransitionTo   string // non-empty if machine proposed and user accepted a transition
	PhasesExecuted []string
}

// TransitionCarryOver defines which artifact kinds carry over between mode transitions.
var TransitionCarryOver = map[string]map[string][]euclotypes.ArtifactKind{
	"code": {
		"debug":    {euclotypes.ArtifactKindExplore, euclotypes.ArtifactKindEditIntent, euclotypes.ArtifactKindVerification},
		"planning": {euclotypes.ArtifactKindExplore},
	},
	"debug": {
		"code": {euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindExplore},
	},
	"planning": {
		"code": {euclotypes.ArtifactKindPlan},
	},
	"tdd": {
		"code": {euclotypes.ArtifactKindVerification},
	},
	"review": {
		"code": {euclotypes.ArtifactKindAnalyze},
	},
}

// CarryOverArtifacts filters artifacts from the source bundle that should
// carry over from fromMode to toMode.
func CarryOverArtifacts(from *ArtifactBundle, fromMode, toMode string) []euclotypes.Artifact {
	if from == nil {
		return nil
	}
	modeMap, ok := TransitionCarryOver[fromMode]
	if !ok {
		return nil
	}
	kinds, ok := modeMap[toMode]
	if !ok {
		return nil
	}
	var out []euclotypes.Artifact
	for _, kind := range kinds {
		out = append(out, from.OfKind(kind)...)
	}
	return out
}

// CarryOverArtifactsFromRules uses the transition rules engine to determine
// which artifacts to carry over. Falls back to the static map if no matching
// rule is found.
func CarryOverArtifactsFromRules(from *ArtifactBundle, fromMode, toMode string, rules *TransitionRuleSet, trigger TransitionTrigger, state map[string]any) []euclotypes.Artifact {
	if from == nil {
		return nil
	}
	if rules != nil {
		rule := rules.Evaluate(fromMode, trigger, state, from)
		if rule != nil && rule.ToMode == toMode {
			var out []euclotypes.Artifact
			for _, kind := range rule.ArtifactCarry {
				out = append(out, from.OfKind(kind)...)
			}
			return out
		}
	}
	// Fallback to static map.
	return CarryOverArtifacts(from, fromMode, toMode)
}

// ExtractInteractionState builds an InteractionState snapshot from a machine.
func ExtractInteractionState(m *PhaseMachine) InteractionState {
	is := InteractionState{
		Mode:         m.mode,
		CurrentPhase: m.CurrentPhase(),
		PhaseStates:  make(map[string]any),
		Selections:   make(map[string]string),
	}
	// Copy state entries namespaced by phase.
	for k, v := range m.state {
		is.PhaseStates[k] = v
	}
	if executed := m.ExecutedPhases(); len(executed) > 0 {
		is.PhasesExecuted = executed
	} else {
		is.PhasesExecuted = make([]string, 0, m.current)
		for i := 0; i < m.current && i < len(m.phases); i++ {
			is.PhasesExecuted = append(is.PhasesExecuted, m.phases[i].ID)
		}
	}
	if skipped := m.SkippedPhases(); len(skipped) > 0 {
		is.SkippedPhases = skipped
	} else {
		// Fallback for machines built before skipped-phase tracking existed.
		for i, p := range m.phases {
			if i < m.current && p.SkipWhen != nil && p.SkipWhen(m.state, m.artifacts) {
				is.SkippedPhases = append(is.SkippedPhases, p.ID)
			}
		}
	}
	return is
}

// ExtractInteractionResult builds an InteractionResult from a completed machine.
func ExtractInteractionResult(m *PhaseMachine) InteractionResult {
	result := InteractionResult{
		Artifacts: m.artifacts.All(),
		State:     m.state,
	}
	if toMode, ok := m.state["transition.accepted"].(string); ok {
		result.TransitionTo = toMode
	}
	if executed := m.ExecutedPhases(); len(executed) > 0 {
		result.PhasesExecuted = executed
	} else {
		for i := 0; i < m.current && i < len(m.phases); i++ {
			result.PhasesExecuted = append(result.PhasesExecuted, m.phases[i].ID)
		}
	}
	return result
}
