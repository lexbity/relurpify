package tui

// InputGate tracks whether the input bar should block prompt submissions
// (> prefix) during active execution. The model activates it when a run
// is in progress and deactivates it when all runs complete.
type InputGate struct {
	active bool
}

// SetActive enables or disables the gate.
func (g *InputGate) SetActive(v bool) {
	if g == nil {
		return
	}
	g.active = v
}

// Active returns true when prompt submissions are blocked.
func (g *InputGate) Active() bool {
	return g != nil && g.active
}
