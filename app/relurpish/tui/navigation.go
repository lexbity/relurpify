package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type navigationFrame struct {
	canvas TabID
	focus  FocusRegion
}

func rootHandleBack(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if len(m.navigation) == 0 {
		m.setFocus(FocusRegionInput)
		return m, nil
	}
	frame := m.navigation[len(m.navigation)-1]
	m.navigation = m.navigation[:len(m.navigation)-1]
	m.selectCanvas(frame.canvas, false)
	m.setFocus(frame.focus)
	return m, nil
}

func rootHandleQuit(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	m.quitReview, m.quitChoice = true, 0
	return m, nil
}

func (m RootModel) quitKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Paste {
		return m, nil
	}
	switch msg.String() {
	case keyEsc:
		m.quitReview = false
	case "up", keyLeft, keyHome:
		m.quitChoice = 0
	case keyDown, keyRight, keyEnd:
		m.quitChoice = 1
	case keyTab, keyShiftTab:
		m.quitChoice = 1 - m.quitChoice
	case keyEnter:
		if m.quitChoice == 0 {
			m.quitReview = false
			return m, nil
		}
		return m, func() tea.Msg { m.cleanup(); return tea.Quit() }
	}
	return m, nil
}

func (m RootModel) quitView() string {
	lines := []string{"Leave relurpish?", ""}
	if m.frontendState.Running || (m.chat != nil && m.chat.HasActiveRuns()) {
		lines = append(lines, "Active work will receive a cancellation request during shutdown.")
	}
	if m.workshop != nil && m.workshop.Dirty() {
		lines = append(lines, "Recipe: unsaved local edits will be lost.")
	}
	if m.provider != nil {
		if m.provider.dirty() {
			lines = append(lines, "Provider: unsaved local edits will be lost.")
		}
		if m.provider.state.Revision != m.provider.state.ActiveRevision {
			lines = append(lines, "Saved provider preferences differ from the active configuration.")
		}
		if m.provider.working {
			lines = append(lines, "A provider operation is awaiting its result.")
		}
	}
	if m.inputBar != nil && m.inputBar.Value() != "" {
		lines = append(lines, "Your unsent request is still a local draft.")
	}
	if (m.notifQ != nil && m.notifQ.Len() > 0) || m.frontendState.Decision != nil {
		lines = append(lines, "Unresolved attention remains; quitting grants no permission.")
	}
	if m.demoScenario != "" {
		lines = append(lines, "DEMO: all scenario changes reset on exit.")
	}
	lines = append(lines, "")
	for i, label := range []string{"Keep working", "Quit"} {
		marker := "  "
		if i == m.quitChoice {
			marker = "> "
		}
		lines = append(lines, marker+label)
	}
	return strings.Join(lines, "\n")
}

// emergencyView preserves the escape routes and a usable composer without
// allocating a full form on terminals smaller than the interaction minimum.
func (m RootModel) emergencyView() string {
	prefix := ""
	if m.demoScenario != "" {
		prefix = "DEMO · "
	}
	if m.height <= 2 {
		return boundedScreen(prefix+"> "+terminalText(m.inputBar.Value()), m.width, m.height)
	}
	lines := []string{prefix + "relurpish · compact", "F1 help · Ctrl+Q quit"}
	switch {
	case m.quitReview:
		lines = append(lines, "Quit review · Esc returns")
		for i, label := range []string{"Keep working", "Quit"} {
			marker := "  "
			if i == m.quitChoice {
				marker = "> "
			}
			lines = append(lines, marker+label)
		}
	case m.showHelp:
		lines = append(lines, "Enter submits · Alt+Enter newline", "/ commands · Esc closes help")
	default:
		lines = append(lines, "Resize for the full canvas")
	}
	if len(lines) >= m.height {
		lines = lines[:m.height-1]
	}
	lines = append(lines, "> "+strings.ReplaceAll(terminalText(m.inputBar.Value()), "\n", " ↵ "))
	return boundedScreen(strings.Join(lines, "\n"), m.width, m.height)
}
