package relurpifyenvtui

import (
	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

type controlCenterPane struct {
	runtime tui.RuntimeAdapter
	context *tui.AgentContext
	session *tui.Session
	store   *tui.SessionStore

	welcome       *tui.WelcomePane
	sandbox       *tui.SandboxPane
	securityguard *tui.SecurityGuardPane
	aiprovider    *tui.AIProviderPane

	activeTab tui.TabID
	width     int
	height    int
}

func (p *controlCenterPane) OwnsEscape() bool {
	switch p.activeTab {
	case tui.TabSandbox:
		return p.sandbox.OwnsEscape()
	case tui.TabAIProvider:
		return p.aiprovider.OwnsEscape()
	case tui.TabSecurityGuard:
		return p.securityguard.OwnsEscape()
	default:
		return false
	}
}

func newControlCenterPane(rt tui.RuntimeAdapter, ctx *tui.AgentContext, sess *tui.Session, store *tui.SessionStore, _ *tui.NotificationQueue) tui.Region1Surface {
	p := &controlCenterPane{
		runtime: rt,
		context: ctx,
		session: sess,
		store:   store,
	}
	p.welcome = tui.NewWelcomePane(sess, store, nil)
	p.sandbox = tui.NewSandboxPane(rt)
	p.securityguard = tui.NewSecurityGuardPane(rt)
	p.aiprovider = tui.NewAIProviderPane(rt)
	p.activeTab = tui.TabWelcome
	return p
}

func (p *controlCenterPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.applySize()
}

func (p *controlCenterPane) SetStore(store *tui.SessionStore) {
	p.store = store
	if p.welcome != nil {
		p.welcome = tui.NewWelcomePane(p.session, store, nil)
	}
	if p.sandbox != nil {
		p.sandbox.Refresh()
	}
	if p.securityguard != nil {
		p.securityguard.Refresh()
	}
	if p.aiprovider != nil {
		p.aiprovider.Refresh()
	}
	p.Refresh()
}

func (p *controlCenterPane) SetActiveTab(id tui.TabID) {
	p.activeTab = id
	p.Refresh()
}

func (p *controlCenterPane) SetFilter(filter string) {
	switch p.activeTab {
	case tui.TabWelcome:
		if p.welcome != nil {
			p.welcome.SetFilter(filter)
		}
	case tui.TabSandbox:
		if p.sandbox != nil {
			p.sandbox.SetFilter(filter)
		}
	case tui.TabSecurityGuard:
		if p.securityguard != nil {
			p.securityguard.SetFilter(filter)
		}
	case tui.TabAIProvider:
		if p.aiprovider != nil {
			p.aiprovider.SetFilter(filter)
		}
	}
}

func (p *controlCenterPane) Refresh() {
	switch p.activeTab {
	case tui.TabWelcome:
		if p.welcome != nil {
			p.welcome.Refresh()
		}
	case tui.TabSandbox:
		if p.sandbox != nil {
			p.sandbox.Refresh()
		}
	case tui.TabSecurityGuard:
		if p.securityguard != nil {
			p.securityguard.Refresh()
		}
	case tui.TabAIProvider:
		if p.aiprovider != nil {
			p.aiprovider.Refresh()
		}
	}
	p.applySize()
}

func (p *controlCenterPane) Update(msg tea.Msg) (tui.Region1Surface, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.WorkspaceSelectedMsg:
		if msg.Workspace != "" {
			p.SetStore(tui.NewSessionStore(msg.Workspace))
		}
		return p, nil
	case tui.ProviderModelsLoadedMsg:
		if p.aiprovider != nil {
			_, cmd := p.aiprovider.Update(msg)
			return p, cmd
		}
		return p, nil
	case tui.ConfigRefreshMsg:
		p.Refresh()
		return p, nil
	case tui.SandboxPersistedMsg:
		if p.sandbox != nil {
			p.sandbox.Refresh()
		}
		return p, nil
	case tea.KeyMsg:
		// ? opens help when Region 1 has focus (control center).
		if msg.String() == "?" {
			return p, func() tea.Msg {
				return tui.CommandInvokedMsg{Name: "help"}
			}
		}
		switch p.activeTab {
		case tui.TabWelcome:
			if p.welcome != nil {
				wp, cmd := p.welcome.Update(msg)
				p.welcome = wp
				return p, cmd
			}
		case tui.TabSandbox:
			if p.sandbox != nil {
				sp, cmd := p.sandbox.Update(msg)
				p.sandbox = sp
				return p, cmd
			}
		case tui.TabSecurityGuard:
			if p.securityguard != nil {
				cp, cmd := p.securityguard.Update(msg)
				p.securityguard = cp
				return p, cmd
			}
		case tui.TabAIProvider:
			if p.aiprovider != nil {
				pa, cmd := p.aiprovider.Update(msg)
				p.aiprovider = pa
				return p, cmd
			}
		}
	}
	return p, nil
}

func (p *controlCenterPane) View() string {
	switch p.activeTab {
	case tui.TabWelcome:
		if p.welcome != nil {
			return p.welcome.View()
		}
	case tui.TabSandbox:
		if p.sandbox != nil {
			return p.sandbox.View()
		}
	case tui.TabSecurityGuard:
		if p.securityguard != nil {
			return p.securityguard.View()
		}
	case tui.TabAIProvider:
		if p.aiprovider != nil {
			return p.aiprovider.View()
		}
	}
	return ""
}

func (p *controlCenterPane) HandleInputSubmit(value string) tea.Cmd {
	_ = value
	return nil
}

func (p *controlCenterPane) Cleanup() {}

func (p *controlCenterPane) FocusFilescopes() {
	p.activeTab = tui.TabSandbox
	if p.sandbox != nil {
		p.sandbox.FocusFilescopes()
	}
}

func (p *controlCenterPane) OpenSecurityGuard() { p.activeTab = tui.TabSecurityGuard }

func (p *controlCenterPane) OpenAIProvider() { p.activeTab = tui.TabAIProvider }

func (p *controlCenterPane) OpenKeybindings() { p.activeTab = tui.TabKeybindings }

func (p *controlCenterPane) OpenDoctor() { p.activeTab = tui.TabDoctor }

func (p *controlCenterPane) applySize() {
	paneH := p.height
	if paneH < 1 {
		paneH = 1
	}
	if p.welcome != nil {
		p.welcome.SetSize(p.width, paneH)
	}
	if p.sandbox != nil {
		p.sandbox.SetSize(p.width, paneH)
	}
	if p.securityguard != nil {
		p.securityguard.SetSize(p.width, paneH)
	}
	if p.aiprovider != nil {
		p.aiprovider.SetSize(p.width, paneH)
	}
}
