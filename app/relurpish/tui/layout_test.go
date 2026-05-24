package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChromeLayoutReservesNoExtraRowWhenNoHITLOrNotifications(t *testing.T) {
	surface := &fakeSurface{name: "guest", chat: &fakeChatPane{}}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)

	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := updated.(RootModel)

	if got, want := rm.layout.Region1Rows(), 38; got != want {
		t.Fatalf("region1 rows = %d, want %d", got, want)
	}
	if got, want := rm.layout.Region1PaneRows(), 37; got != want {
		t.Fatalf("region1 pane rows = %d, want %d", got, want)
	}
}

func TestChromeLayoutReservesRowForActiveHITLRow(t *testing.T) {
	surface := &fakeSurface{name: "guest", chat: &fakeChatPane{}}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)
	m.hitlRow.Open("frame-1", "Question?", []string{"one"}, []string{"One"})

	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := updated.(RootModel)

	if got, want := rm.layout.Region1Rows(), 37; got != want {
		t.Fatalf("region1 rows = %d, want %d", got, want)
	}
	if got, want := rm.layout.Region1PaneRows(), 36; got != want {
		t.Fatalf("region1 pane rows = %d, want %d", got, want)
	}
}
