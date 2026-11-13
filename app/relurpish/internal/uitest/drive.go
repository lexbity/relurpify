// Package uitest drives the public host interface using real input messages.
package uitest

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func Resize(m tui.RootModel, width, height int) tui.RootModel {
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(tui.RootModel)
}

func Navigate(m tui.RootModel, canvas tui.TabID) tui.RootModel {
	commands := map[tui.TabID]string{tui.TabChat: "chat", tui.TabRun: "run", tui.TabSettings: "settings", tui.TabWelcome: "session", tui.TabSandbox: "permissions", tui.TabAIProvider: "model-provider", tui.TabSecurityGuard: "securityguard", tui.TabKeybindings: "keybindings", tui.TabDoctor: "doctor"}
	command, ok := commands[canvas]
	if !ok {
		panic(fmt.Sprintf("no command for canvas %q", canvas))
	}
	next, _ := m.Update(tui.CommandInvokedMsg{Name: command})
	return next.(tui.RootModel)
}

func FocusCanvas(m tui.RootModel) tui.RootModel {
	if m.InteractionState().Focus.InRegion1() {
		return m
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF6})
	return next.(tui.RootModel)
}

func FocusComposer(m tui.RootModel) tui.RootModel {
	if m.InteractionState().Focus.InInput() {
		return m
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF6})
	return next.(tui.RootModel)
}

func SelectAgent(m *tui.RootModel, agent string) error {
	next, _ := m.Update(tui.CommandInvokedMsg{Name: "agent", Args: []string{agent}})
	*m = next.(tui.RootModel)
	if m.InteractionState().Agent != agent {
		return fmt.Errorf("agent selection failed: %s", agent)
	}
	return nil
}
