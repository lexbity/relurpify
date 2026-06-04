package tui

import (
	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// HITLRowAnswerMsg is emitted when the user resolves an interaction frame
// by pressing a slot shortcut key on the HITL row.
type HITLRowAnswerMsg struct {
	FrameID  string
	SlotID   string
	SlotName string
}

// HITLRowDismissMsg is emitted when the user dismisses the HITL row (esc).
type HITLRowDismissMsg struct{ FrameID string }

// HITLRow is the agent-triggered HITL Notification Row.
// It appears as a single row between Region 1 and the bottom bar when the
// agent emits an interaction frame requiring user selection.
type HITLRow struct {
	open bool

	frameID   string
	question  string
	slotIDs   []string
	slotNames []string

	width int
	// Theme is the active semantic style source.
	th *theme.Theme
}

// Open activates the row with the given frame information.
func (h *HITLRow) Open(frameID, question string, slotIDs, slotNames []string) {
	if h == nil {
		return
	}
	h.open = true
	h.frameID = frameID
	h.question = question
	h.slotIDs = slotIDs
	h.slotNames = slotNames
}

// Close deactivates the row.
func (h *HITLRow) Close() {
	if h == nil {
		return
	}
	h.open = false
	h.frameID = ""
	h.question = ""
	h.slotIDs = nil
	h.slotNames = nil
}

// Active returns true when the row is visible.
func (h *HITLRow) Active() bool {
	return h != nil && h.open
}

// SetWidth updates the row width (called on resize).
func (h *HITLRow) SetWidth(w int) {
	if h == nil {
		return
	}
	h.width = w
}

// FrameID returns the currently displayed frame ID.
func (h *HITLRow) FrameID() string {
	if h == nil {
		return ""
	}
	return h.frameID
}

// HandleKey processes a key event when the row is active.
// Returns a command and whether the key was handled.
func (h *HITLRow) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if h == nil || !h.open {
		return nil, false
	}
	key := msg.String()
	switch {
	case key == "esc":
		fid := h.frameID
		h.Close()
		return func() tea.Msg {
			return HITLRowDismissMsg{FrameID: fid}
		}, true

	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		idx := int(key[0] - '1')
		if idx >= len(h.slotIDs) {
			return nil, false
		}
		fid := h.frameID
		sid := h.slotIDs[idx]
		sname := ""
		if idx < len(h.slotNames) {
			sname = h.slotNames[idx]
		}
		h.Close()
		return func() tea.Msg {
			return HITLRowAnswerMsg{FrameID: fid, SlotID: sid, SlotName: sname}
		}, true
	}
	return nil, false
}

// View renders the HITL row. Returns empty string when inactive.
func (h *HITLRow) View() string {
	if h == nil || !h.open {
		return ""
	}
	if h.th == nil {
		h.th = theme.Default()
	}
	w := h.width
	if w < 1 {
		w = 1
	}

	parts := []string{"● " + h.question}
	for i, name := range h.slotNames {
		if name == "" && i < len(h.slotIDs) {
			name = h.slotIDs[i]
		}
		parts = append(parts, fmt.Sprintf("[%d] %s", i+1, name))
	}
	parts = append(parts, h.th.Dim().Render("[esc] dismiss"))

	content := strings.Join(parts, "  ")
	return h.th.Notif(theme.NotifInfo).Width(w).Render(content)
}

// SetTheme sets the active semantic style source.
func (h *HITLRow) SetTheme(th *theme.Theme) {
	if th != nil {
		h.th = th
	}
}
