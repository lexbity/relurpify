package tui

import tea "github.com/charmbracelet/bubbletea"

type pairingActionRequestMsg struct {
	action string
	code   string
}

func requestPairingAction(action, code string) tea.Cmd {
	return func() tea.Msg {
		return pairingActionRequestMsg{action: action, code: code}
	}
}
