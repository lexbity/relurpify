package tui

// InteractionState is a detached observation of the host's current interaction.
// It contains no mutable widgets, runtime handles, or permission state.
type InteractionState struct {
	Canvas   TabID
	Agent    string
	Draft    string
	Focus    FocusState
	Overlays int
}

func (m RootModel) InteractionState() InteractionState {
	state := InteractionState{Canvas: m.activeTab, Agent: m.activeAgentName(), Focus: m.focus.State()}
	if m.inputBar != nil {
		state.Draft = m.inputBar.Value()
	}
	if m.overlays != nil {
		state.Overlays = m.overlays.Len()
	}
	return state
}
