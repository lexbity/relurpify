package euclotui

import (
	"context"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// EucloSurface is the default Euclo interaction surface.
type EucloSurface struct {
	base tui.AgentSurface
}

// NewSurface returns the Euclo interaction surface.
func NewSurface() tui.AgentSurface {
	return &EucloSurface{base: tui.NewDefaultSurfaceFactory().Resolve("generic")}
}

// NewSurfaceFactory returns a surface registry with Euclo registered and the
// generic surface as the fallback.
func NewSurfaceFactory() tui.SurfaceFactory {
	registry := tui.NewSurfaceRegistry(tui.NewDefaultSurfaceFactory().Resolve("generic"))
	registry.Register("euclo", NewSurface())
	return registry
}

func (s *EucloSurface) Name() string { return "euclo" }

func (s *EucloSurface) RegisterTabs(reg *tui.TabRegistry) {
	if s.base != nil {
		s.base.RegisterTabs(reg)
	}
}

func (s *EucloSurface) RegisterCommands(reg *tui.CommandRegistry) {
	if s.base != nil {
		s.base.RegisterCommands(reg)
	}
}

func (s *EucloSurface) NewChat(rt tui.RuntimeAdapter, ctx *tui.AgentContext, sess *tui.Session, notifQ *tui.NotificationQueue) tui.ChatPaner {
	if s.base != nil {
		return s.base.NewChat(rt, ctx, sess, notifQ)
	}
	return tui.NewChatPane(rt, ctx, sess, notifQ)
}

func (s *EucloSurface) InitialTab() tui.TabID {
	if s.base != nil {
		return s.base.InitialTab()
	}
	return tui.TabChat
}

func (s *EucloSurface) InitialSubTab(tab tui.TabID) tui.SubTabID {
	if s.base != nil {
		return s.base.InitialSubTab(tab)
	}
	if tab == tui.TabChat {
		return tui.SubTabChatLocalEdit
	}
	return ""
}

func (s *EucloSurface) RenderNotification(item tui.NotificationItem) string {
	return RenderInteractionNotification(item)
}

func (s *EucloSurface) HandleFrame(_ context.Context, m *tui.RootModel, msg tui.SurfaceFrameMsg) {
	if m == nil {
		return
	}
	frame, ok := msg.Frame.(interaction.InteractionFrame)
	if !ok {
		if s.base != nil {
			s.base.HandleFrame(context.Background(), m, msg)
		}
		return
	}
	if msg.Notification.Kind == "" {
		msg.Notification = notificationItemFromFrame(tui.GenerateID(), NotifKindInteraction, frame, nil)
	}
	m.PushNotification(msg.Notification)
	m.AppendSurfaceMessage(msg.Message)
	m.ApplyInteractionFrame(frame)
}
