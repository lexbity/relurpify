package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// AnimationID identifies a registered animation within the manager.
type AnimationID int

// AnimationFrame is produced by each registered animation on every tick.
type AnimationFrame struct {
	Text string
	Done bool
}

// AnimationTickMsg is emitted by the manager on each tick when at least one
// animation is active.
type AnimationTickMsg struct{}

// AnimationManager owns the global tick budget. When no animations are
// registered the manager emits nothing, keeping the idle message cost at zero.
type AnimationManager struct {
	nextID AnimationID
	active map[AnimationID]func() AnimationFrame
}

// NewAnimationManager creates an empty manager.
func NewAnimationManager() *AnimationManager {
	return &AnimationManager{active: make(map[AnimationID]func() AnimationFrame)}
}

// Register adds a frame-producing function and returns a unique ID that can
// be used to deregister the animation.
func (m *AnimationManager) Register(fn func() AnimationFrame) AnimationID {
	if m == nil || fn == nil {
		return 0
	}
	m.nextID++
	id := m.nextID
	m.active[id] = fn
	return id
}

// Deregister removes a previously registered animation by ID. It is safe to
// call with an ID that was already removed.
func (m *AnimationManager) Deregister(id AnimationID) {
	if m == nil || m.active == nil {
		return
	}
	delete(m.active, id)
}

// Active reports whether any animations are currently registered.
func (m *AnimationManager) Active() bool {
	if m == nil {
		return false
	}
	return len(m.active) > 0
}

// TickCmd returns a tea.Cmd that fires an AnimationTickMsg if at least one
// animation is active. When idle it returns nil, keeping the tick budget at
// zero.
func (m *AnimationManager) TickCmd() tea.Cmd {
	if m == nil || !m.Active() {
		return nil
	}
	return func() tea.Msg {
		return AnimationTickMsg{}
	}
}

// Advance calls every registered animation function and removes those that
// report Done. Callers use the returned frames to update their state.
func (m *AnimationManager) Advance() []AnimationFrame {
	if m == nil || !m.Active() {
		return nil
	}
	frames := make([]AnimationFrame, 0, len(m.active))
	for id, fn := range m.active {
		fr := fn()
		frames = append(frames, fr)
		if fr.Done {
			delete(m.active, id)
		}
	}
	return frames
}
