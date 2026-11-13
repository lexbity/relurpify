package demo_test

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestV2ServicesReviewCancelAndPartialFailure(t *testing.T) {
	m, session := model(t, "services-partial")
	m = command(m, "services")
	if !strings.Contains(m.View(), "knowledge.index") {
		t.Fatal("service observations did not reach the shared canvas")
	}
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if !strings.Contains(m.View(), "Review restart-all") {
		t.Fatal("lifecycle action bypassed review")
	}
	m = press(m, tea.KeyEsc)
	items, err := session.Services(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Status != tui.ServiceStatusRunning {
			t.Fatal("canceled review changed a service")
		}
	}
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "1 of 3 service operations failed") || !strings.Contains(m.View(), "DEMO") {
		t.Fatalf("partial failure was flattened or mislabeled:\n%s", m.View())
	}
	items, err = session.Services(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == "knowledge.index" && item.Status != tui.ServiceStatusError {
			t.Fatal("failed restart lost its observed error state")
		}
	}
	// A service shortcut typed in the Euclo bar is still ordinary request text.
	m = press(m, tea.KeyF6)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r is a request")})
	if m.InteractionState().Draft != "r is a request" {
		t.Fatal("service canvas stole composer typing")
	}
}

func TestV2ConfigurationInspectorIsReadOnlyAndDraftPreserving(t *testing.T) {
	m, _ := model(t, "quiet")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("keep this draft")})
	m = command(m, "config")
	m = press(m, tea.KeyEnter)
	if !strings.Contains(m.View(), "Declared level: ask") || !strings.Contains(m.View(), "DEMO") {
		t.Fatal("selected configuration entry did not open with origin/evidence")
	}
	m = press(m, tea.KeyEsc, tea.KeyEsc)
	if m.InteractionState().Draft != "keep this draft" || !m.InteractionState().Focus.InInput() {
		t.Fatal("inspecting configuration erased or captured the request draft")
	}
}

func TestV2ReboundHelpUsesHostMapWithoutOldAlias(t *testing.T) {
	m, _ := model(t, "quiet")
	m = command(m, "keybindings")
	m = press(m, tea.KeyF6)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = press(m, tea.KeyF2, tea.KeyEnter, tea.KeyF6, tea.KeyF2)
	if !strings.Contains(m.View(), "Commands · Escape") {
		t.Fatal("rebound help key did not dispatch through the host map")
	}
	m = press(m, tea.KeyEsc, tea.KeyF1)
	if strings.Contains(m.View(), "Commands · Escape") {
		t.Fatal("old help chord remained as an undocumented alias")
	}
}
