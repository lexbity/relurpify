package tui

import "codeburg.org/lexbit/relurpify/named/euclo/interaction"

// EucloFrameMsg is sent to the TUI when an interaction frame should be rendered.
type EucloFrameMsg struct {
	Msg   Message
	Frame interaction.InteractionFrame
}

// EucloResponseMsg is sent when the user responds to an interaction notification.
type EucloResponseMsg struct {
	Response interaction.UserResponse
}

// EucloPhaseProgressMsg updates the phase progress indicator.
type EucloPhaseProgressMsg struct {
	Mode       string
	PhaseIndex int
	PhaseCount int
	Labels     []interaction.PhaseInfo
}
