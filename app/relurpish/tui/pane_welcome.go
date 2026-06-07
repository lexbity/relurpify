package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

type WelcomePane struct {
	session       *Session
	store         *SessionStore
	factory       SurfaceFactory
	width, height int

	// Logo
	logo Logo

	// Agent selection
	agentDrop Dropdown

	// Session resume
	resumeDrop Dropdown

	// Focus ring index
	focusIdx int

	// Theme is the active semantic style source.
	th *theme.Theme
}

func NewWelcomePane(sess *Session, store *SessionStore, factory SurfaceFactory) *WelcomePane {
	p := &WelcomePane{
		session:    sess,
		store:      store,
		factory:    factory,
		th:         theme.Default(),
		logo:       *NewLogo(55, 20),
		agentDrop:  *NewDropdown("agent", nil),
		resumeDrop: *NewDropdown("resume", nil),
	}
	p.refreshAgents()
	p.refreshSessions()
	return p
}

func (p *WelcomePane) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.logo.SetSize(55, h)
}

func (p *WelcomePane) SetFilter(string) {}

func (p *WelcomePane) Refresh() {
	p.refreshAgents()
	p.refreshSessions()
}

func (p *WelcomePane) refreshAgents() {
	agents := []DropdownItem{}
	if p.factory != nil {
		for _, name := range p.factory.AvailableAgents() {
			agents = append(agents, DropdownItem{ID: name, Label: name})
		}
	}
	if len(agents) == 0 {
		agents = []DropdownItem{{ID: "none", Label: "none"}}
	}
	p.agentDrop = *NewDropdown("agent", agents)
	p.agentDrop.SetTheme(p.th)
}

func (p *WelcomePane) refreshSessions() {
	items := []DropdownItem{}
	if p.store != nil {
		metas, err := p.store.List()
		if err == nil {
			for _, meta := range metas {
				label := fmt.Sprintf("%s — %s", meta.Agent, meta.UpdatedAt.Format("01/02 15:04"))
				items = append(items, DropdownItem{ID: meta.ID, Label: label})
			}
		}
	}
	p.resumeDrop = *NewDropdown("resume", items)
	p.resumeDrop.SetTheme(p.th)
}

func (p *WelcomePane) Update(msg tea.Msg) (*WelcomePane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKey(msg)
	case tea.MouseMsg:
		return p.handleMouse(msg)
	}
	return p, nil
}

func (p *WelcomePane) handleKey(msg tea.KeyMsg) (*WelcomePane, tea.Cmd) {
	// Focus ring navigation via tab/shift+tab.
	if msg.String() == "tab" {
		p.focusIdx = (p.focusIdx + 1) % 6
		p.syncFocus()
		return p, nil
	}
	if msg.String() == "shift+tab" {
		p.focusIdx = (p.focusIdx + 5) % 6
		p.syncFocus()
		return p, nil
	}

	// Route key to focused widget.
	switch p.focusIdx {
	case 0: // agent dropdown
		cmd, handled := p.agentDrop.Update(msg)
		if handled {
			return p, cmd
		}
	case 1: // Start button
		if msg.String() == "enter" || msg.String() == " " {
			return p, p.startCmd()
		}
	case 2: // resume dropdown
		cmd, handled := p.resumeDrop.Update(msg)
		if handled {
			return p, cmd
		}
	case 3: // Resume button
		if msg.String() == "enter" || msg.String() == " " {
			return p, p.resumeCmd()
		}
	case 4: // Doctor
		if msg.String() == "enter" || msg.String() == " " {
			return p, func() tea.Msg { return OpenDoctorMsg{} }
		}
	case 5: // Help
		if msg.String() == "enter" || msg.String() == " " {
			// Help is handled by the host via f1/?.
			return p, nil
		}
	}
	return p, nil
}

func (p *WelcomePane) handleMouse(msg tea.MouseMsg) (*WelcomePane, tea.Cmd) {
	cmds := []tea.Cmd{}

	if cmd, handled := p.agentDrop.HandleClick(msg.X, msg.Y); handled {
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return p, tea.Batch(cmds...)
	}

	if cmd, handled := p.resumeDrop.HandleClick(msg.X, msg.Y); handled {
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return p, tea.Batch(cmds...)
	}

	return p, nil
}

func (p *WelcomePane) startCmd() tea.Cmd {
	agent := p.agentDrop.SelectedID()
	if agent == "" {
		return nil
	}
	return func() tea.Msg {
		return StartSessionMsg{Agent: agent}
	}
}

func (p *WelcomePane) resumeCmd() tea.Cmd {
	sessionID := p.resumeDrop.SelectedID()
	if sessionID == "" {
		return nil
	}
	return func() tea.Msg {
		return ResumeSessionMsg{SessionID: sessionID}
	}
}

func (p *WelcomePane) syncFocus() {
	p.agentDrop.Blur()
	p.resumeDrop.Blur()
	switch p.focusIdx {
	case 0:
		p.agentDrop.Focus()
	case 2:
		p.resumeDrop.Focus()
	}
}

func (p *WelcomePane) View() string {
	if p.width < 60 {
		return p.th.Dim().Render("Terminal too narrow. Minimum 60 columns required.")
	}

	// Left column: logo
	logoView := p.logo.View()

	// Right column: widgets
	agentLabel := p.agentDrop.Selected().Label
	if agentLabel == "" {
		agentLabel = "none"
	}
	agentDropView := p.agentDrop.View()

	startBtn := NewButton("Start")
	startBtn.SetTheme(p.th)
	startBtn.SetWidth(10)
	if p.focusIdx == 1 {
		startBtn.Focus()
	}
	startView := startBtn.View()

	resumeDropView := p.resumeDrop.View()

	resumeBtn := NewButton("Resume")
	resumeBtn.SetTheme(p.th)
	resumeBtn.SetWidth(10)
	if p.focusIdx == 3 {
		resumeBtn.Focus()
	}
	resumeView := resumeBtn.View()

	doctorBtn := NewButton("Doctor")
	doctorBtn.SetTheme(p.th)
	doctorBtn.SetWidth(10)
	if p.focusIdx == 4 {
		doctorBtn.Focus()
	}
	doctorView := doctorBtn.View()

	helpBtn := NewButton("Help f1")
	helpBtn.SetTheme(p.th)
	helpBtn.SetWidth(10)
	if p.focusIdx == 5 {
		helpBtn.Focus()
	}
	helpView := helpBtn.View()

	right := lipgloss.JoinVertical(lipgloss.Left,
		"",
		p.th.Subhead().Render("New Session"),
		"",
		agentDropView,
		"",
		startView,
		"",
		p.th.Subhead().Render("Resume Session"),
		"",
		resumeDropView,
		"",
		resumeView,
		"",
		doctorView,
		"",
		helpView,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, logoView, right)
}
