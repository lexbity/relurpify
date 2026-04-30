package tui

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// AgentSurface owns the agent-specific interaction surface for a given agent.
// The host keeps shell chrome, lifecycle, and persistence, while the surface
// owns its tabs, commands, notification rendering, and frame handling.
type AgentSurface interface {
	Name() string
	RegisterTabs(reg *TabRegistry)
	RegisterCommands(reg *CommandRegistry)
	NewChat(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner
	InitialTab() TabID
	InitialSubTab(tab TabID) SubTabID
	RenderNotification(item NotificationItem) string
	HandleFrame(ctx context.Context, m *RootModel, msg SurfaceFrameMsg)
}

// SurfaceFactory resolves the active surface for a given agent name.
type SurfaceFactory interface {
	Resolve(agentName string) AgentSurface
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

type surfaceRegistry struct {
	defaultSurface AgentSurface
	surfaces       map[string]AgentSurface
}

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
		if surface, ok := r.surfaces[agentName]; ok && surface != nil {
			return surface
		}
	}
	return r.defaultSurface
}

func normalizeSurfaceKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

type genericSurface struct{}

func newGenericSurface() AgentSurface {
	return genericSurface{}
}

func (genericSurface) Name() string { return "generic" }

func (genericSurface) RegisterTabs(reg *TabRegistry) {
	if reg == nil {
		return
	}
	reg.Register(TabDefinition{
		ID:    TabChat,
		Label: "chat",
		SubTabs: []SubTabDefinition{
			{ID: SubTabChatLocalRead, Label: "local-read-only"},
			{ID: SubTabChatLocalEdit, Label: "local-edit-on"},
			{ID: SubTabChatOnlineRead, Label: "online-read-on"},
			{ID: SubTabChatOnlineEdit, Label: "online-edit-on"},
		},
	})
}

func (genericSurface) RegisterCommands(reg *CommandRegistry) {
	registerSurfaceCommands(reg)
}

func (genericSurface) NewChat(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) ChatPaner {
	return NewChatPane(rt, ctx, sess, notifQ)
}

func (genericSurface) InitialTab() TabID { return TabChat }

func (genericSurface) InitialSubTab(tab TabID) SubTabID {
	if tab == TabChat {
		return SubTabChatLocalEdit
	}
	return ""
}

func (genericSurface) RenderNotification(item NotificationItem) string {
	return renderGenericNotification(item)
}

func (genericSurface) HandleFrame(_ context.Context, m *RootModel, msg SurfaceFrameMsg) {
	if m == nil {
		return
	}
	m.PushNotification(msg.Notification)
	m.AppendSurfaceMessage(msg.Message)
	if frame, ok := msg.Frame.(interaction.InteractionFrame); ok {
		m.ApplyInteractionFrame(frame)
	}
}
