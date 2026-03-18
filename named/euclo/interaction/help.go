package interaction

import "time"

// ModePhaseMap holds registered phase maps for each mode.
// Used by BuildHelpFrame to populate the help surface.
type ModePhaseMap struct {
	phases map[string][]PhaseInfo
}

// NewModePhaseMap creates a new empty phase map registry.
func NewModePhaseMap() *ModePhaseMap {
	return &ModePhaseMap{phases: make(map[string][]PhaseInfo)}
}

// Register adds a phase map for a mode.
func (m *ModePhaseMap) Register(mode string, phases []PhaseInfo) {
	m.phases[mode] = phases
}

// Get returns the phase map for a mode, or nil.
func (m *ModePhaseMap) Get(mode string) []PhaseInfo {
	return m.phases[mode]
}

// ModeTransitions holds the set of valid transitions from each mode.
type ModeTransitions struct {
	transitions map[string][]TransitionInfo
}

// NewModeTransitions creates a new empty transition registry.
func NewModeTransitions() *ModeTransitions {
	return &ModeTransitions{transitions: make(map[string][]TransitionInfo)}
}

// Register adds valid transitions from a mode.
func (t *ModeTransitions) Register(fromMode string, transitions []TransitionInfo) {
	t.transitions[fromMode] = transitions
}

// Get returns valid transitions from a mode, or nil.
func (t *ModeTransitions) Get(fromMode string) []TransitionInfo {
	return t.transitions[fromMode]
}

// BuildHelpFrame produces a FrameHelp with the current mode's interaction surface.
// The help frame is non-advancing — it doesn't change the current phase.
func BuildHelpFrame(
	mode string,
	currentPhase string,
	resolver *AgencyResolver,
	phaseMap *ModePhaseMap,
	transitions *ModeTransitions,
) InteractionFrame {
	content := HelpContent{
		Mode:         mode,
		CurrentPhase: currentPhase,
	}

	// Phase map with current marker.
	if phaseMap != nil {
		phases := phaseMap.Get(mode)
		marked := make([]PhaseInfo, len(phases))
		for i, p := range phases {
			marked[i] = PhaseInfo{
				ID:      p.ID,
				Label:   p.Label,
				Current: p.ID == currentPhase,
			}
		}
		content.PhaseMap = marked
	}

	// Available actions from agency resolver.
	if resolver != nil {
		triggers := resolver.TriggersForMode(mode)
		actions := make([]ActionInfo, 0, len(triggers))
		for _, t := range triggers {
			phrase := ""
			if len(t.Phrases) > 0 {
				phrase = t.Phrases[0]
			}
			actions = append(actions, ActionInfo{
				Phrase:      phrase,
				Description: t.Description,
			})
		}
		content.AvailableActions = actions
	}

	// Available transitions.
	if transitions != nil {
		content.AvailableTransitions = transitions.Get(mode)
	}

	return InteractionFrame{
		Kind:    FrameHelp,
		Mode:    mode,
		Phase:   currentPhase,
		Content: content,
		Metadata: FrameMetadata{
			Timestamp: time.Now(),
		},
	}
}

// DefaultModeTransitions returns the canonical set of mode transitions.
func DefaultModeTransitions() *ModeTransitions {
	t := NewModeTransitions()
	t.Register("code", []TransitionInfo{
		{Phrase: "plan first", TargetMode: "planning"},
		{Phrase: "debug this", TargetMode: "debug"},
		{Phrase: "review", TargetMode: "review"},
	})
	t.Register("debug", []TransitionInfo{
		{Phrase: "fix it", TargetMode: "code"},
		{Phrase: "plan first", TargetMode: "planning"},
	})
	t.Register("planning", []TransitionInfo{
		{Phrase: "execute plan", TargetMode: "code"},
	})
	t.Register("tdd", []TransitionInfo{
		{Phrase: "refactor", TargetMode: "code"},
		{Phrase: "plan first", TargetMode: "planning"},
	})
	t.Register("review", []TransitionInfo{
		{Phrase: "fix findings", TargetMode: "code"},
		{Phrase: "plan first", TargetMode: "planning"},
	})
	return t
}

// HelpTriggerPhrases are the phrases that trigger the help surface.
var HelpTriggerPhrases = []string{
	"help",
	"what can I do",
	"what are my options",
}

// RegisterHelpTriggers registers the cross-cutting help triggers.
func RegisterHelpTriggers(resolver *AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("", AgencyTrigger{
		Phrases:     HelpTriggerPhrases,
		Description: "Show available actions and mode information",
	})
}
