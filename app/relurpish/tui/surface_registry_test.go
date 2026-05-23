package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type registrySurface struct {
	name string
	tabs []TabDefinition
	chat *fakeChatPane
}

func (s *registrySurface) Name() string { return s.name }

func (s *registrySurface) RegisterTabs(reg *TabRegistry) {
	if reg == nil {
		return
	}
	for _, tab := range s.tabs {
		reg.Register(tab)
	}
}

func (s *registrySurface) RegisterCommands(*CommandRegistry) {}

func (s *registrySurface) NewChat(RuntimeAdapter, *AgentContext, *Session, *NotificationQueue) ChatPaner {
	if s.chat != nil {
		return s.chat
	}
	return &fakeChatPane{}
}

func (s *registrySurface) InitialTab() TabID {
	if len(s.tabs) > 0 {
		return s.tabs[0].ID
	}
	return ""
}

func (s *registrySurface) InitialSubTab(tab TabID) SubTabID {
	_ = tab
	return ""
}

func (s *registrySurface) RenderNotification(item NotificationItem) string { return item.Msg }

func (s *registrySurface) HandleFrame(context.Context, *RootModel, SurfaceFrameMsg) {}

type registryFactory struct {
	defaultSurface AgentSurface
	surfaces       map[string]AgentSurface
	resolves       map[string]int
}

func (f *registryFactory) Resolve(agentName string) AgentSurface {
	if f.resolves == nil {
		f.resolves = make(map[string]int)
	}
	key := normalizeSurfaceKey(agentName)
	f.resolves[key]++
	if key == "none" {
		return f.defaultSurface
	}
	if surface, ok := f.surfaces[key]; ok && surface != nil {
		return surface
	}
	return f.defaultSurface
}

func TestSurfaceRegistryResolveFallsBack(t *testing.T) {
	defaultSurface := &registrySurface{name: "none"}
	registry := NewSurfaceRegistry(defaultSurface)
	custom := &registrySurface{name: "euclo"}
	registry.Register("euclo", custom)

	if got := registry.Resolve("euclo"); got != custom {
		t.Fatalf("expected euclo surface, got %#v", got)
	}
	if got := registry.Resolve("unknown"); got != defaultSurface {
		t.Fatalf("expected default surface, got %#v", got)
	}
	if got := registry.Resolve("none"); got != defaultSurface {
		t.Fatalf("expected none to resolve to default surface, got %#v", got)
	}
}

func TestActivateSurfaceCachesPerAgent(t *testing.T) {
	noneSurface := &registrySurface{
		name: "none",
		tabs: []TabDefinition{
			{ID: TabWelcome, Label: "welcome"},
			{ID: TabSandbox, Label: "sandbox"},
			{ID: TabSecurityGuard, Label: "securityguard"},
			{ID: TabAIProvider, Label: "ai provider"},
			{ID: TabKeybindings, Label: "keybindings"},
			{ID: TabDoctor, Label: "doctor"},
		},
		chat: &fakeChatPane{},
	}
	eucloChat := &fakeChatPane{}
	eucloSurface := &registrySurface{
		name: "euclo",
		tabs: []TabDefinition{
			{ID: TabChat, Label: "chat"},
			{ID: TabGraph, Label: "graph"},
			{ID: TabDiff, Label: "diff"},
			{ID: TabLibrary, Label: "library"},
		},
		chat: eucloChat,
	}
	factory := &registryFactory{
		defaultSurface: noneSurface,
		surfaces: map[string]AgentSurface{
			"euclo": eucloSurface,
		},
	}

	m := newRootModel(nil, factory)
	if got := m.activeAgentName(); got != "none" {
		t.Fatalf("initial agent = %q, want none", got)
	}
	if got := len(m.tabs.All()); got != 6 {
		t.Fatalf("initial tab count = %d, want 6", got)
	}

	if err := m.switchActiveAgent("euclo"); err != nil {
		t.Fatalf("switch to euclo failed: %v", err)
	}
	if got := m.activeAgentName(); got != "euclo" {
		t.Fatalf("agent after switch = %q, want euclo", got)
	}
	if got := len(m.tabs.All()); got != 4 {
		t.Fatalf("euclo tab count = %d, want 4", got)
	}
	if got := m.tabs.All()[0].Label; got != "chat" {
		t.Fatalf("euclo first tab label = %q, want chat", got)
	}

	eucloChat.width = 77
	m.setActiveTab(TabDiff)
	m.setActiveSubTab("")

	if err := m.switchActiveAgent("none"); err != nil {
		t.Fatalf("switch to none failed: %v", err)
	}
	if got := m.activeAgentName(); got != "none" {
		t.Fatalf("agent after switch back = %q, want none", got)
	}
	if got := len(m.tabs.All()); got != 6 {
		t.Fatalf("none tab count = %d, want 6", got)
	}
	if got := m.tabs.All()[0].Label; got != "welcome" {
		t.Fatalf("none first tab label = %q, want welcome", got)
	}

	if err := m.switchActiveAgent("euclo"); err != nil {
		t.Fatalf("switch back to euclo failed: %v", err)
	}
	if got := m.activeAgentName(); got != "euclo" {
		t.Fatalf("agent after restoring = %q, want euclo", got)
	}
	if got := m.tabs.ActiveTab().ID; got != TabDiff {
		t.Fatalf("restored active tab = %q, want %q", got, TabDiff)
	}
	if got := m.chat.(*fakeChatPane).width; got != 77 {
		t.Fatalf("restored chat width = %d, want 77", got)
	}
	if got := m.tabs.All()[3].Label; got != "library" {
		t.Fatalf("euclo tab labels not restored, got %q", got)
	}
}

func TestAgentPickerOpensFromRegion2Click(t *testing.T) {
	noneSurface := &registrySurface{name: "none", tabs: []TabDefinition{{ID: TabWelcome, Label: "welcome"}}}
	factory := &registryFactory{
		defaultSurface: noneSurface,
		surfaces: map[string]AgentSurface{
			"euclo": &registrySurface{name: "euclo", tabs: []TabDefinition{{ID: TabChat, Label: "chat"}}},
		},
	}
	m := newRootModel(nil, factory)
	m.width = 120
	m.height = 40
	m.ready = true
	m.layout.Recalculate(120, 40, false)
	m.overlays.Clear()

	handled, _ := m.handleMouse(tea.MouseMsg{
		X:      1,
		Y:      38,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if !handled {
		t.Fatal("expected region 2 click to open picker")
	}
	if m.agentPicker == nil || !m.agentPicker.IsOpen() {
		t.Fatal("expected agent picker to open")
	}
}
