package tui

// NotifGuidanceResolveMsg is sent when the notification bar resolves a
// guidance request via freetext input.
type NotifGuidanceResolveMsg struct {
	RequestID string
	ChoiceID  string
	Freetext  string
}
