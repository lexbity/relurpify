package relurpifyenvtui

import (
	"context"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

// Surface owns the base-framework control center for the none agent.
type Surface struct{}

// NewSurface returns the base-framework surface used for the none agent.
func NewSurface() tui.AgentSurface {
	return &Surface{}
}

// NewSurfaceFactory returns a registry with the base-framework surface as the fallback.
func NewSurfaceFactory() tui.SurfaceFactory {
	registry := tui.NewSurfaceRegistry(NewSurface())
	return registry
}

func (s *Surface) Name() string { return "none" }

func (s *Surface) RegisterTabs(reg *tui.TabRegistry) {
	if reg == nil {
		return
	}
	reg.Register(tui.TabDefinition{ID: tui.TabWelcome, Label: "welcome", AgentFilter: []string{"none"}})
	reg.Register(tui.TabDefinition{ID: tui.TabSandbox, Label: "sandbox", AgentFilter: []string{"none"}})
	reg.Register(tui.TabDefinition{ID: tui.TabSecurityGuard, Label: "securityguard", AgentFilter: []string{"none"}})
	reg.Register(tui.TabDefinition{ID: tui.TabAIProvider, Label: "ai provider", AgentFilter: []string{"none"}})
	reg.Register(tui.TabDefinition{ID: tui.TabKeybindings, Label: "keybindings", AgentFilter: []string{"none"}})
	reg.Register(tui.TabDefinition{ID: tui.TabDoctor, Label: "doctor", AgentFilter: []string{"none"}})
}

func (s *Surface) RegisterCommands(reg *tui.CommandRegistry) {
	_ = reg
}

func (s *Surface) NewChat(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session, *tui.NotificationQueue) tui.ChatPaner {
	return nil
}

func (s *Surface) NewInput(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session) tui.InputSurface {
	return nil
}

func (s *Surface) NewNav(tui.RuntimeAdapter, *tui.AgentContext, *tui.Session) tui.NavSurface {
	return nil
}

func (s *Surface) NewRegion1(rt tui.RuntimeAdapter, ctx *tui.AgentContext, sess *tui.Session, store *tui.SessionStore, notifQ *tui.NotificationQueue) tui.Region1Surface {
	return newControlCenterPane(rt, ctx, sess, store, notifQ)
}

func (s *Surface) InitialTab() tui.TabID { return tui.TabWelcome }

func (s *Surface) InitialSubTab(tab tui.TabID) tui.SubTabID { return "" }

func (s *Surface) RenderNotification(item tui.NotificationItem) string {
	return item.Msg
}

func (s *Surface) HandleFrame(_ context.Context, _ *tui.RootModel, _ tui.SurfaceFrameMsg) {}
