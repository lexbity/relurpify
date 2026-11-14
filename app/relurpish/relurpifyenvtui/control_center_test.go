package relurpifyenvtui

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

type navigationProviderReader struct {
	workspace string
	reads     int
}

func (r *navigationProviderReader) SessionInfo() tui.SessionInfo {
	r.reads++
	return tui.SessionInfo{Workspace: r.workspace}
}
func (*navigationProviderReader) ReloadWorkspace(context.Context, string) error {
	panic("navigation must not reload execution runtime")
}

func TestControlCenterNavigationRetainsProviderDraftAndDoesNotReload(t *testing.T) {
	reader := &navigationProviderReader{workspace: t.TempDir()}
	pane := &controlCenterPane{aiprovider: tui.NewAIProviderPane(reader)}
	pane.SetSize(100, 24)
	pane.SetActiveTab(tui.TabAIProvider)
	// Focus configuration, select model, edit and stage a local value.
	for _, key := range []tea.KeyMsg{{Type: tea.KeyTab}, {Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("unsaved-marker")}, {Type: tea.KeyEnter}} {
		pane.Update(key)
	}
	if !strings.Contains(pane.View(), "unsaved-marker") {
		t.Fatalf("no staged local model: %s", pane.View())
	}
	reads := reader.reads
	pane.SetActiveTab(tui.TabWelcome)
	pane.SetActiveTab(tui.TabAIProvider)
	if reader.reads != reads || !strings.Contains(pane.View(), "unsaved-marker") {
		t.Fatal("navigation reloaded storage and discarded local provider state")
	}
}
