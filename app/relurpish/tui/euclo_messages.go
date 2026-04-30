package tui

import "codeburg.org/lexbit/relurpify/named/euclo/interaction"

// EucloFrameMsg is sent to the TUI when an interaction frame should be rendered.
type EucloFrameMsg struct {
	Msg   Message
	Frame interaction.InteractionFrame
}

// EucloPhaseProgressMsg updates the phase progress indicator.
type EucloPhaseProgressMsg struct {
	Mode       string
	PhaseIndex int
	PhaseCount int
	Labels     []interaction.PhaseInfo
}
