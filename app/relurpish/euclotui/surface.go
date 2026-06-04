package euclotui

import (
	"context"

	"codeburg.org/lexbit/relurpify/app/relurpish/relurpifyenvtui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// EucloSurface is the default Euclo interaction surface.
type EucloSurface struct {
	base   tui.AgentSurface
	router *EucloEventRouter
}

// NewSurface returns the Euclo interaction surface.
func NewSurface() tui.AgentSurface {
	return &EucloSurface{
		base:   tui.NewDefaultSurfaceFactory().Resolve("none"),
		router: NewEucloEventRouter(),
	}
}

// NewSurfaceFactory returns a surface registry with Euclo registered and the
// generic surface as the fallback.
func NewSurfaceFactory() tui.SurfaceFactory {
	registry := tui.NewSurfaceRegistry(relurpifyenvtui.NewSurface())
	registry.Register("euclo", NewSurface())
	return registry
}

func (s *EucloSurface) Name() string { return "euclo" }

func (s *EucloSurface) RegisterTabs(reg *tui.TabRegistry) {
	RegisterEucloTabs(reg)
}

func (s *EucloSurface) RegisterCommands(reg *tui.CommandRegistry) {
	tui.RegisterEucloCommands(reg)
}

func (s *EucloSurface) NewChat(rt tui.RuntimeAdapter, ctx *tui.AgentContext, sess *tui.Session, notifQ *tui.NotificationQueue) tui.ChatPaner {
	return NewChatPane(rt, ctx, sess, notifQ, s.router)
}

func (s *EucloSurface) NewRegion1(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session, *tui.SessionStore, *tui.NotificationQueue) tui.Region1Surface {
	return nil
}

func (s *EucloSurface) NewInput(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session) tui.InputSurface {
	return nil
}

func (s *EucloSurface) NewNav(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session) tui.NavSurface {
	return nil
}

func (s *EucloSurface) InitialTab() tui.TabID {
	return tui.TabChat
}

func (s *EucloSurface) InitialSubTab(tab tui.TabID) tui.SubTabID {
	if tab == tui.TabChat {
		return tui.SubTabChatLocalEdit
	}
	return ""
}

func (s *EucloSurface) RenderNotification(item tui.NotificationItem) string {
	return RenderInteractionNotification(item)
}

func (s *EucloSurface) HandleFrame(ctx context.Context, m *tui.RootModel, msg tui.SurfaceFrameMsg) {
	if m == nil {
		return
	}
	frame, ok := msg.Frame.(interaction.InteractionFrame)
	if !ok {
		if s.base != nil {
			s.base.HandleFrame(ctx, m, msg)
		}
		return
	}
	if s.router != nil {
		s.router.ApplyInteractionFrame(frame)
	}
	if msg.Notification.Kind == "" {
		msg.Notification = notificationItemFromFrame(tui.GenerateID(), NotifKindInteraction, frame, nil)
	}
	m.TrackInteractionFrame(msg.Notification.ID, frame)
	m.PushNotification(msg.Notification)
	m.AppendSurfaceMessage(msg.Message)
	m.ApplyInteractionFrame(frame)
	if notificationAllowsFreetext(msg.Notification) {
		m.OpenInteractionGuidance(msg.Notification.ID, frame)
	}
}
