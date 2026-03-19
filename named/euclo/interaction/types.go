package interaction

import "time"

// FrameKind identifies the type of interaction frame.
type FrameKind string

const (
	FrameProposal      FrameKind = "proposal"       // system proposes scope/interpretation for confirmation
	FrameQuestion      FrameKind = "question"       // system asks a targeted question with options
	FrameCandidates    FrameKind = "candidates"     // system presents multiple candidates for selection
	FrameComparison    FrameKind = "comparison"     // side-by-side comparison of candidates
	FrameDraft         FrameKind = "draft"          // editable draft (plan, test list, edit proposal)
	FrameResult        FrameKind = "result"         // execution result (verification, reproduction, findings)
	FrameStatus        FrameKind = "status"         // progress/streaming status during execution
	FrameSummary       FrameKind = "summary"        // final summary with produced artifacts
	FrameTransition    FrameKind = "transition"     // proposed mode transition
	FrameHelp          FrameKind = "help"           // mode help surface
	FrameSessionResume FrameKind = "session_resume" // resume previously persisted interaction state
)

// ActionKind identifies the type of user action available in a frame.
type ActionKind string

const (
	ActionConfirm    ActionKind = "confirm"    // yes/proceed
	ActionSelect     ActionKind = "select"     // pick from numbered options
	ActionFreetext   ActionKind = "freetext"   // open text input
	ActionToggle     ActionKind = "toggle"     // on/off per-item
	ActionBatch      ActionKind = "batch"      // apply to category
	ActionTransition ActionKind = "transition" // propose mode switch
)

// InteractionFrame is the core unit of the interaction protocol.
// Euclo emits frames; UX layers consume them.
type InteractionFrame struct {
	Kind        FrameKind     `json:"kind"`
	Mode        string        `json:"mode"`
	Phase       string        `json:"phase"`
	Content     any           `json:"content"`
	Actions     []ActionSlot  `json:"actions"`
	Continuable bool          `json:"continuable"`
	Metadata    FrameMetadata `json:"metadata"`
}

// ActionSlot describes a single user action available in a frame.
type ActionSlot struct {
	ID                string     `json:"id"`
	Label             string     `json:"label"`
	Shortcut          string     `json:"shortcut,omitempty"`
	Kind              ActionKind `json:"kind"`
	Default           bool       `json:"default,omitempty"`
	TargetPhase       string     `json:"target_phase,omitempty"`
	CapabilityTrigger string     `json:"capability_trigger,omitempty"`
}

// FrameMetadata carries contextual information about where a frame sits
// in the overall interaction flow.
type FrameMetadata struct {
	Timestamp    time.Time `json:"timestamp"`
	PhaseIndex   int       `json:"phase_index"`
	PhaseCount   int       `json:"phase_count"`
	ArtifactRefs []string  `json:"artifact_refs,omitempty"`
}

// DefaultAction returns the default action slot, or nil if none is marked default.
func (f *InteractionFrame) DefaultAction() *ActionSlot {
	for i := range f.Actions {
		if f.Actions[i].Default {
			return &f.Actions[i]
		}
	}
	return nil
}

// ActionByID returns the action slot with the given ID, or nil if not found.
func (f *InteractionFrame) ActionByID(id string) *ActionSlot {
	for i := range f.Actions {
		if f.Actions[i].ID == id {
			return &f.Actions[i]
		}
	}
	return nil
}
