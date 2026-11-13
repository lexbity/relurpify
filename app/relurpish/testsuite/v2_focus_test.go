package testsuite_test

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	"codeburg.org/lexbit/relurpify/app/relurpish/internal/uitest"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestV2KeyCaptureEscapeReturnsInTwoSteps(t *testing.T) {
	m := tui.NewRootModel(context.Background(), tui.HostOptions{SurfaceFactory: euclotui.NewSurfaceFactory()})
	m = uitest.Resize(m, 120, 40)
	m = uitest.Navigate(m, tui.TabKeybindings)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("keep draft")})
	m = uitest.FocusCanvas(m)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !strings.Contains(m.View(), "Press the new key") {
		t.Fatal("capture did not open through actual host focus routing")
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.InteractionState().Focus.InRegion1() || strings.Contains(m.View(), "Rebind open_help") {
		t.Fatal("first Escape did not close capture while retaining canvas focus")
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.InteractionState().Focus.InInput() || m.InteractionState().Draft != "keep draft" {
		t.Fatal("second Escape did not restore unchanged composer")
	}
}

func TestV2QuitCannotBeCapturedByAdministration(t *testing.T) {
	for _, canvas := range []tui.TabID{tui.TabDoctor, tui.TabKeybindings, tui.TabAIProvider, tui.TabSandbox} {
		m := tui.NewRootModel(context.Background(), tui.HostOptions{SurfaceFactory: euclotui.NewSurfaceFactory()})
		m = uitest.Resize(m, 80, 24)
		m = uitest.FocusCanvas(uitest.Navigate(m, canvas))
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
		if cmd == nil {
			t.Fatalf("%s consumed immutable quit", canvas)
		}
		batch, ok := cmd().(tea.BatchMsg)
		if !ok {
			t.Fatalf("%s did not schedule clean shutdown", canvas)
		}
		quit := false
		for _, action := range batch {
			if _, ok := action().(tea.QuitMsg); ok {
				quit = true
			}
		}
		if !quit {
			t.Fatalf("%s never quit", canvas)
		}
	}
}
