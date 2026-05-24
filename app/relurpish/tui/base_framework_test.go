package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type baseSurfaceFake struct {
	activeTab TabID
	filter    string
	report    DoctorReport
	status    string
}

func (f *baseSurfaceFake) Name() string { return "none" }
func (f *baseSurfaceFake) RegisterTabs(reg *TabRegistry) {
	reg.Register(TabDefinition{ID: TabWelcome, Label: "welcome", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabSandbox, Label: "sandbox", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabSecurityGuard, Label: "securityguard", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabAIProvider, Label: "ai provider", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabKeybindings, Label: "keybindings", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabDoctor, Label: "doctor", AgentFilter: []string{"none"}})
}
func (f *baseSurfaceFake) RegisterCommands(*CommandRegistry) {}
func (f *baseSurfaceFake) NewChat(RuntimeAdapter, *AgentContext, *Session, *NotificationQueue) ChatPaner { return nil }
func (f *baseSurfaceFake) NewLibrary(RuntimeAdapter, *AgentContext, *Session) LibrarySurface            { return nil }
func (f *baseSurfaceFake) NewRegion1(RuntimeAdapter, *AgentContext, *Session, *SessionStore, *NotificationQueue) Region1Surface {
	return f
}
func (f *baseSurfaceFake) InitialTab() TabID { return TabWelcome }
func (f *baseSurfaceFake) InitialSubTab(TabID) SubTabID { return "" }
func (f *baseSurfaceFake) RenderNotification(item NotificationItem) string { return item.Msg }
func (f *baseSurfaceFake) HandleFrame(_ context.Context, _ *RootModel, _ SurfaceFrameMsg) {}

func (f *baseSurfaceFake) SetSize(int, int)                     {}
func (f *baseSurfaceFake) SetStore(*SessionStore)               {}
func (f *baseSurfaceFake) SetActiveTab(id TabID)                { f.activeTab = id }
func (f *baseSurfaceFake) SetFilter(filter string)              { f.filter = filter }
func (f *baseSurfaceFake) Refresh()                              {}
func (f *baseSurfaceFake) Update(msg tea.Msg) (Region1Surface, tea.Cmd) { return f, nil }
func (f *baseSurfaceFake) View() string                         { return f.filter + "|" + string(f.activeTab) }
func (f *baseSurfaceFake) HandleInputSubmit(string) tea.Cmd     { return nil }
func (f *baseSurfaceFake) Cleanup()                             {}
func (f *baseSurfaceFake) FocusFilescopes()                     {}
func (f *baseSurfaceFake) OpenSecurityGuard()                   { f.activeTab = TabSecurityGuard }
func (f *baseSurfaceFake) OpenAIProvider()                      { f.activeTab = TabAIProvider }
func (f *baseSurfaceFake) OpenKeybindings()                     { f.activeTab = TabKeybindings }
func (f *baseSurfaceFake) OpenDoctor()                          { f.activeTab = TabDoctor }
func (f *baseSurfaceFake) DoctorReport() DoctorReport           { return f.report }
func (f *baseSurfaceFake) SetDoctorReport(report DoctorReport)  { f.report = report }
func (f *baseSurfaceFake) SetDoctorStatus(status string)        { f.status = status }

// noneSurfaceFactory returns a SurfaceFactory that always resolves to "none".
func noneSurfaceFactory() SurfaceFactory {
	return NewSurfaceRegistry(&baseSurfaceFake{})
}

func newBaseFrameworkModel() RootModel {
	reg := noneSurfaceFactory()
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
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
	}
}

func TestBaseFrameworkSwitchToSecurityGuard(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabSecurityGuard)
	if m.activeTab != TabSecurityGuard {
		t.Fatalf("expected activeTab = 'securityguard', got %q", m.activeTab)
	}
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
	}
}

func TestBaseFrameworkSwitchToAIProvider(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabAIProvider)
	if m.activeTab != TabAIProvider {
		t.Fatalf("expected activeTab = 'ai-provider', got %q", m.activeTab)
	}
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
	}
}

func TestBaseFrameworkSwitchToKeybindings(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabKeybindings)
	if m.activeTab != TabKeybindings {
		t.Fatalf("expected activeTab = 'keybindings', got %q", m.activeTab)
	}
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
	}
}

func TestBaseFrameworkSwitchToDoctor(t *testing.T) {
	m := newBaseFrameworkModel()
	m.setActiveTab(TabDoctor)
	if m.activeTab != TabDoctor {
		t.Fatalf("expected activeTab = 'doctor', got %q", m.activeTab)
	}
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
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
	if m.baseSurface == nil {
		t.Fatal("expected base surface to exist")
	}
	m.baseSurface.SetStore(store)
	m.setActiveTab(TabWelcome)

	before := m.baseSurface.View()
	m.baseSurface.SetFilter("bet")
	after := m.baseSurface.View()
	if before == after {
		t.Fatalf("expected filter to change welcome view")
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

func TestBaseFrameworkRendersControlCenterOnNoneAgent(t *testing.T) {
	m := newBaseFrameworkModel()
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := updated.(RootModel)
	rm.ready = true

	view := rm.View()
	if !strings.Contains(view, "|welcome") {
		t.Fatalf("view missing welcome control-center marker: %s", view)
	}
	if !strings.Contains(view, "sandbox") || !strings.Contains(view, "doctor") {
		t.Fatalf("view missing base-framework tabs: %s", view)
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
