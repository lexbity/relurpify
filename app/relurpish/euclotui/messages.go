package euclotui

import (
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

// PatchHunk describes one causal code change attached to an execution event.
type PatchHunk struct {
	File         string
	Summary      string
	Body         string
	StepID       string
	Origin       string
	LinesAdded   int
	LinesRemoved int
}

// ExecutionEvent is the normalized event envelope used by the Euclo router.
type ExecutionEvent struct {
	Header      reporting.EventHeader
	Type        reporting.EventType
	TaskID      string
	SessionID   string
	NodeID      string
	RecipeID    string
	StepID      string
	Surface     string
	Summary     string
	Milestone   string
	Output      string
	RouteScores map[string]float64
	PatchHunks  []PatchHunk
	Frame       *interaction.InteractionFrame
	Payload     map[string]any
}

// RecipeRunMsg groups execution events under one recipe run identifier.
type RecipeRunMsg struct {
	RunID    string
	RecipeID string
	TaskID   string
	Events   []ExecutionEvent
	Complete bool
	Outcome  string
}

// NewFrameMsg packages an interaction frame into the generic surface message
// format used by the relurpish host.
func NewFrameMsg(frame interaction.InteractionFrame) tui.SurfaceFrameMsg {
	return tui.SurfaceFrameMsg{
		Surface:      "euclo",
		Message:      RenderInteractionFrame(frame),
		Frame:        frame,
		Notification: notificationItemFromFrame(tui.GenerateID(), NotifKindInteraction, frame, nil),
	}
}
