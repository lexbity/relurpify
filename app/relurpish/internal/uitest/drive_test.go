package uitest

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

// newModel builds a root model on the real surface factory, as the QA sweep
// does, so drive helpers are exercised against production components.
func newModel(t *testing.T) tui.RootModel {
	t.Helper()
	m := tui.NewRootModel(context.Background(), tui.HostOptions{SurfaceFactory: euclotui.NewSurfaceFactory()})
	return Resize(m, 120, 40)
}

func TestResizeProducesRenderedModel(t *testing.T) {
	m := newModel(t)
	for _, size := range [][2]int{{1, 1}, {80, 24}, {500, 200}, {3, 3}} {
		m = Resize(m, size[0], size[1])
		if view := strings.TrimSpace(m.View()); view == "" {
			t.Fatalf("empty view after resize to %v", size)
		}
	}
}

func TestNavigateReachesEveryTab(t *testing.T) {
	m := newModel(t)
	for _, tab := range []tui.TabID{
		tui.TabWelcome,
		tui.TabSandbox,
		tui.TabSecurityGuard,
		tui.TabAIProvider,
		tui.TabKeybindings,
		tui.TabDoctor,
		tui.TabChat,
		tui.TabRun,
		tui.TabSettings,
	} {
		m = Navigate(m, tab)
		if view := strings.TrimSpace(m.View()); view == "" {
			t.Fatalf("empty view after navigating to %q", tab)
		}
	}
}

func TestNavigateUnknownCanvasPanics(t *testing.T) {
	m := newModel(t)
	defer func() {
		if recover() == nil {
			t.Fatal("Navigate with unknown canvas did not panic")
		}
	}()
	Navigate(m, tui.TabID("no-such-canvas"))
}

func TestFocusTogglesBetweenRegions(t *testing.T) {
	m := newModel(t)
	if !m.InteractionState().Focus.InInput() {
		t.Fatal("fresh model does not start in the composer")
	}
	m = FocusCanvas(m)
	if !m.InteractionState().Focus.InRegion1() {
		t.Fatal("FocusCanvas did not move focus to the canvas region")
	}
	m = FocusComposer(m)
	if !m.InteractionState().Focus.InInput() {
		t.Fatal("FocusComposer did not restore focus to the composer")
	}
	// Idempotent: already focused regions are left alone.
	m = FocusComposer(m)
	if !m.InteractionState().Focus.InInput() {
		t.Fatal("FocusComposer changed focus while already in the input")
	}
}

func TestSelectAgentSetsActiveAgent(t *testing.T) {
	m := newModel(t)
	if err := SelectAgent(&m, "euclo"); err != nil {
		t.Fatalf("SelectAgent(euclo): %v", err)
	}
	if m.InteractionState().Agent != "euclo" {
		t.Fatalf("agent = %q, want euclo", m.InteractionState().Agent)
	}
}

func TestSelectAgentRejectsUnknownAgent(t *testing.T) {
	m := newModel(t)
	if err := SelectAgent(&m, ""); err == nil {
		t.Fatal("SelectAgent(empty) succeeded; want error")
	}
}
