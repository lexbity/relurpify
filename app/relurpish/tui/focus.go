package tui

import tea "github.com/charmbracelet/bubbletea"

// FocusAction describes the host-level routing decision for a key press.
type FocusAction int

const (
	FocusActionIgnore FocusAction = iota
	FocusActionFocusInput
	FocusActionFocusRegion1
	FocusActionRouteInput
	FocusActionRouteRegion1
	FocusActionTypePrintable
)

// FocusRoute is the deterministic result of the host focus router.
type FocusRoute struct {
	Action    FocusAction
	Printable string
}

// FocusRouter owns the current host focus state and translates raw keys into
// focus transitions or routing decisions.
type FocusRouter struct {
	state FocusState
}

// NewFocusRouter returns a router with Region 3 focused.
func NewFocusRouter() FocusRouter {
	return FocusRouter{state: NewFocusState()}
}

// State returns the current focus state.
func (r FocusRouter) State() FocusState {
	return r.state
}

// SetState replaces the router state.
func (r *FocusRouter) SetState(state FocusState) {
	if r == nil {
		return
	}
	r.state = state
}

// FocusInput moves focus back to Region 3.
func (r *FocusRouter) FocusInput() {
	if r == nil {
		return
	}
	r.state.Region = FocusRegionInput
}

// FocusRegion1 moves focus into Region 1.
func (r *FocusRouter) FocusRegion1() {
	if r == nil {
		return
	}
	r.state.Region = FocusRegionRegion1
}

// Route translates a key into a focus action.
func (r FocusRouter) Route(msg tea.KeyMsg) FocusRoute {
	if r.state.InRegion1() {
		switch msg.String() {
		case "esc":
			return FocusRoute{Action: FocusActionFocusInput}
		}
		if printable, ok := printableKey(msg); ok {
			return FocusRoute{Action: FocusActionTypePrintable, Printable: printable}
		}
		return FocusRoute{Action: FocusActionRouteRegion1}
	}
	switch msg.String() {
	case "tab", "ctrl+down":
		return FocusRoute{Action: FocusActionFocusRegion1}
	case "esc":
		return FocusRoute{Action: FocusActionIgnore}
	}
	return FocusRoute{Action: FocusActionRouteInput}
}

func printableKey(msg tea.KeyMsg) (string, bool) {
	if msg.Type == tea.KeySpace {
		return " ", true
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return "", false
	}
	return string(msg.Runes[0]), true
}
