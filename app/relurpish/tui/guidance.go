package tui

// NotifGuidanceResolveMsg is sent when the notification bar resolves a
// guidance request via freetext input.
type NotifGuidanceResolveMsg struct {
	RequestID string
	ChoiceID  string
	Freetext  string
}

// NotifInteractionResolveMsg is sent when the notification bar resolves an
// interaction frame by slot selection or free-text submission.
type NotifInteractionResolveMsg struct {
	NotificationID string
	FrameID        string
	TaskID         string
	ChoiceID       string
	Freetext       string
}
