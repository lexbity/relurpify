package tui

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
)

// AgentSurface owns the interaction surface for a given agent.
// The host keeps shell chrome, lifecycle, and persistence, while the surface
// owns its tabs, commands, notification rendering, and frame handling.
// Optional input/nav surfaces let an agent replace Region 3 and Region 4.
type AgentSurface interface {
	Name() string
	RegisterTabs(reg *TabRegistry)
	RegisterCommands(reg *CommandRegistry)
	NewChat(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner
	NewRegion1(rt RuntimeAdapter, ctx *AgentContext, sess *Session, store *SessionStore, notifQ *NotificationQueue) Region1Surface
	NewInput(rt RuntimeAdapter, ctx *AgentContext, sess *Session) InputSurface
	NewNav(rt RuntimeAdapter, ctx *AgentContext, sess *Session) NavSurface
	InitialTab() TabID
	InitialSubTab(tab TabID) SubTabID
	RenderNotification(item NotificationItem) string
	HandleFrame(ctx context.Context, m *RootModel, msg SurfaceFrameMsg)
}

// SurfaceFactory resolves the active surface for a given agent name.
type SurfaceFactory interface {
	Resolve(agentName string) AgentSurface
	AvailableAgents() []string
}

// SurfaceFrameMsg is a surface-local event emitted by agent-specific runtime
// code. The host treats it as opaque and delegates handling to the active
// surface.
type SurfaceFrameMsg struct {
	Surface      string
	Message      Message
	Frame        any
	Notification NotificationItem
}

// InputSurface owns the host's Region 3 input bar when a surface opts out of
// the default host input chrome.
type InputSurface interface {
	SetSize(w, h int)
	View() string
	HandleKey(msg tea.KeyMsg) (tea.Cmd, bool)
}

// NavSurface owns the host's Region 4 navigation bar when a surface opts out
// of the default host tab switcher chrome.
type NavSurface interface {
	SetSize(w, h int)
	View() string
	HandleKey(msg tea.KeyMsg) (tea.Cmd, bool)
}

type surfaceRegistry struct {
	defaultSurface AgentSurface
	surfaces       map[string]AgentSurface
}

// NewDefaultSurfaceFactory returns a registry with the host default surface.
func NewDefaultSurfaceFactory() SurfaceFactory {
	registry := NewSurfaceRegistry(newGenericSurface())
	return registry
}

func NewSurfaceRegistry(defaultSurface AgentSurface) *surfaceRegistry {
	return &surfaceRegistry{
		defaultSurface: defaultSurface,
		surfaces:       make(map[string]AgentSurface),
	}
}

func (r *surfaceRegistry) Register(agentName string, surface AgentSurface) {
	if r == nil || surface == nil {
		return
	}
	if r.surfaces == nil {
		r.surfaces = make(map[string]AgentSurface)
	}
	if agentName = normalizeSurfaceKey(agentName); agentName != "" {
		r.surfaces[agentName] = surface
	}
}

func (r *surfaceRegistry) Resolve(agentName string) AgentSurface {
	if r == nil {
		return nil
	}
	if agentName = normalizeSurfaceKey(agentName); agentName != "" {
		if agentName == "none" {
			return r.defaultSurface
		}
		if surface, ok := r.surfaces[agentName]; ok && surface != nil {
			return surface
		}
	}
	return r.defaultSurface
}

func (r *surfaceRegistry) AvailableAgents() []string {
	if r == nil {
		return nil
	}
	agents := make([]string, 0, len(r.surfaces))
	for name, surface := range r.surfaces {
		if surface == nil {
			continue
		}
		if name = normalizeSurfaceKey(name); name != "" && name != "none" {
			agents = append(agents, name)
		}
	}
	sort.Strings(agents)
	return agents
}

func normalizeSurfaceKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

type genericSurface struct{}

func newGenericSurface() AgentSurface {
	return genericSurface{}
}

func (genericSurface) Name() string { return "none" }

func (genericSurface) RegisterTabs(reg *TabRegistry) {
	if reg == nil {
		return
	}
	reg.Register(TabDefinition{
		ID:          TabWelcome,
		Label:       "welcome",
		AgentFilter: []string{"none"},
	})
	reg.Register(TabDefinition{
		ID:          TabSandbox,
		Label:       "sandbox",
		AgentFilter: []string{"none"},
	})
	reg.Register(TabDefinition{ID: TabSecurityGuard, Label: "securityguard", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabAIProvider, Label: "ai provider", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabKeybindings, Label: "keybindings", AgentFilter: []string{"none"}})
	reg.Register(TabDefinition{ID: TabDoctor, Label: "doctor", AgentFilter: []string{"none"}})
}

func (genericSurface) RegisterCommands(reg *CommandRegistry) {
	_ = reg
}

func (genericSurface) NewChat(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner {
	return nil
}

func (genericSurface) NewRegion1(rt RuntimeAdapter, ctx *AgentContext, sess *Session, store *SessionStore, notifQ *NotificationQueue) Region1Surface {
	return nil
}

func (genericSurface) NewInput(rt RuntimeAdapter, ctx *AgentContext, sess *Session) InputSurface {
	return nil
}

func (genericSurface) NewNav(rt RuntimeAdapter, ctx *AgentContext, sess *Session) NavSurface {
	return nil
}

func (genericSurface) InitialTab() TabID { return TabWelcome }

func (genericSurface) InitialSubTab(tab TabID) SubTabID { return "" }

func (genericSurface) RenderNotification(item NotificationItem) string {
	return renderGenericNotification(item)
}

func (genericSurface) HandleFrame(_ context.Context, m *RootModel, msg SurfaceFrameMsg) {
	if m == nil {
		return
	}
	if frame, ok := msg.Frame.(interaction.InteractionFrame); ok {
		m.TrackInteractionFrame(msg.Notification.ID, frame)
	}
	m.PushNotification(msg.Notification)
	m.AppendSurfaceMessage(msg.Message)
	if frame, ok := msg.Frame.(interaction.InteractionFrame); ok {
		m.ApplyInteractionFrame(frame)
	}
}
