package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// noneSurfaceFactory returns a SurfaceFactory that always resolves to "none".
func noneSurfaceFactory() SurfaceFactory {
	return NewSurfaceRegistry(newGenericSurface())
}

func newBaseFrameworkModel() RootModel {
	reg := NewSurfaceRegistry(newGenericSurface())
	return newRootModel(nil, reg)
}

func TestBaseFrameworkBootsWithNoneAgent(t *testing.T) {
	m := newBaseFrameworkModel()
	if m.activeAgent != "none" {
		t.Fatalf("expected activeAgent = 'none', got %q", m.activeAgent)
	}
	if m.activeTab != TabWelcome {
		t.Fatalf("expected initial tab = 'welcome', got %q", m.activeTab)
	}
	if m.chat != nil {
		t.Fatal("expected chat to be nil in base framework mode")
	}
	if !m.focus.State().InInput() {
		t.Fatal("expected input focus at startup")
	}
}

func TestBaseFrameworkRegistersSixPanels(t *testing.T) {
	m := newBaseFrameworkModel()
	want := []TabID{TabWelcome, TabSandbox, TabSecurityGuard, TabAIProvider, TabKeybindings, TabDoctor}
	got := m.tabs.All()
	if len(got) != len(want) {
		t.Fatalf("expected %d tabs, got %d: %v", len(want), len(got), tabIDs(got))
	}
	for i, tab := range got {
		if tab.ID != want[i] {
			t.Fatalf("tab[%d] = %q, want %q", i, tab.ID, want[i])
		}
	}
}

func TestBaseFrameworkSwitchToSandbox(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabSandbox)
	if m.activeTab != TabSandbox {
		t.Fatalf("expected activeTab = 'sandbox', got %q", m.activeTab)
	}
	if m.sandbox == nil {
		t.Fatal("expected sandbox pane to exist")
	}
}

func TestBaseFrameworkSwitchToSecurityGuard(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabSecurityGuard)
	if m.activeTab != TabSecurityGuard {
		t.Fatalf("expected activeTab = 'securityguard', got %q", m.activeTab)
	}
	if m.securityguard == nil {
		t.Fatal("expected securityguard pane to exist")
	}
}

func TestBaseFrameworkSwitchToAIProvider(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabAIProvider)
	if m.activeTab != TabAIProvider {
		t.Fatalf("expected activeTab = 'ai-provider', got %q", m.activeTab)
	}
	if m.aiprovider == nil {
		t.Fatal("expected ai-provider pane to exist")
	}
}

func TestBaseFrameworkSwitchToKeybindings(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabKeybindings)
	if m.activeTab != TabKeybindings {
		t.Fatalf("expected activeTab = 'keybindings', got %q", m.activeTab)
	}
	if m.keybindings == nil {
		t.Fatal("expected keybindings pane to exist")
	}
}

func TestBaseFrameworkSwitchToDoctor(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabDoctor)
	if m.activeTab != TabDoctor {
		t.Fatalf("expected activeTab = 'doctor', got %q", m.activeTab)
	}
	if m.doctor == nil {
		t.Fatal("expected doctor pane to exist")
	}
}

func TestBaseFrameworkWelcomeFilterReducesItems(t *testing.T) {
	store := NewSessionStore(t.TempDir())
	now := time.Now()
	records := []SessionRecord{
		{SessionMeta: SessionMeta{ID: "a", Workspace: "/work/alpha", Agent: "none", UpdatedAt: now.Add(-2 * time.Hour)}},
		{SessionMeta: SessionMeta{ID: "b", Workspace: "/work/beta", Agent: "none", UpdatedAt: now.Add(-1 * time.Hour)}},
	}
	for _, rec := range records {
		if err := store.Save(rec); err != nil {
			t.Fatalf("save session %q: %v", rec.ID, err)
		}
	}

	m := newBaseFrameworkModel()
	m.store = store
	m.welcome = NewWelcomePane(m.sharedSess, store)
	m.setActiveTab(TabWelcome)

	before := len(m.welcome.filteredWorkspaces())
	m.welcome.SetFilter("bet")
	after := len(m.welcome.filteredWorkspaces())
	if after >= before {
		t.Fatalf("expected filter to reduce items (before=%d, after=%d)", before, after)
	}
}

func TestBaseFrameworkTabSwitchRoundTrip(t *testing.T) {
	m := newBaseFrameworkModel()
	tabs := []TabID{TabWelcome, TabSandbox, TabSecurityGuard, TabAIProvider, TabKeybindings, TabDoctor}
	for _, tab := range tabs {
		m.setActiveTab(tab)
		if m.activeTab != tab {
			t.Fatalf("after setActiveTab(%q), activeTab = %q", tab, m.activeTab)
		}
	}
	// Switch back to welcome
	m.setActiveTab(TabWelcome)
	if m.activeTab != TabWelcome {
		t.Fatalf("after returning to welcome, activeTab = %q", m.activeTab)
	}
}

func TestBaseFrameworkSanityCheckUpdates(t *testing.T) {
	m := newBaseFrameworkModel()

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := updated.(RootModel)
	if rm.layout.Region1Rows() <= 0 {
		t.Fatal("expected positive region 1 rows after resize")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd from resize, got %v", cmd)
	}
}

func TestBaseFrameworkStartupNotLocked(t *testing.T) {
	m := newBaseFrameworkModel()
	if m.startupLocked {
		t.Fatal("expected base framework to not be startup-locked when no runtime is present")
	}
}

// tabIDs extracts the ID field from a slice of TabDefinition.
func tabIDs(defs []TabDefinition) []TabID {
	ids := make([]TabID, len(defs))
	for i, d := range defs {
		ids[i] = d.ID
	}
	return ids
}
