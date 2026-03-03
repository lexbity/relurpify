package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// settingsSessionsMsg carries an async-loaded list of stored sessions.
type settingsSessionsMsg struct {
	sessions []SessionMeta
	err      error
}

// settingsRowKind identifies the type of a settings row.
type settingsRowKind int

const (
	rowKindOption  settingsRowKind = iota // agent / model / recording
	rowKindSession                        // saved session
)

// SettingsPane displays configuration options.
type SettingsPane struct {
	agents        []string
	models        []string
	sessions      []SessionMeta
	store         *SessionStore
	sel           int
	currentAgent  string
	currentModel  string
	recording     string
	runtime       RuntimeAdapter
	width, height int
}

// NewSettingsPane creates a SettingsPane.
func NewSettingsPane(rt RuntimeAdapter, store *SessionStore) *SettingsPane {
	p := &SettingsPane{runtime: rt, store: store, recording: "off"}
	if rt != nil {
		p.agents = rt.AvailableAgents()
		p.recording = rt.RecordingMode()
		info := rt.SessionInfo()
		p.currentAgent = info.Agent
		p.currentModel = info.Model
	}
	return p
}

// Init loads available Ollama models and stored sessions asynchronously.
func (p *SettingsPane) Init() tea.Cmd {
	var cmds []tea.Cmd
	if p.runtime != nil {
		rt := p.runtime
		cmds = append(cmds, func() tea.Msg {
			models, err := rt.OllamaModels(context.Background())
			return settingsModelsMsg{models: models, err: err}
		})
	}
	if p.store != nil {
		store := p.store
		cmds = append(cmds, func() tea.Msg {
			sessions, err := store.List()
			return settingsSessionsMsg{sessions: sessions, err: err}
		})
	}
	return tea.Batch(cmds...)
}

type settingsModelsMsg struct {
	models []string
	err    error
}

// SetSize resizes the pane.
func (p *SettingsPane) SetSize(w, h int) { p.width = w; p.height = h }

// Update handles navigation and actions.
func (p *SettingsPane) Update(msg tea.Msg) (*SettingsPane, tea.Cmd) {
	switch msg := msg.(type) {
	case settingsModelsMsg:
		if msg.err == nil {
			p.models = msg.models
		}
	case settingsSessionsMsg:
		if msg.err == nil {
			p.sessions = msg.sessions
		}
	case tea.KeyMsg:
		rows := p.rows()
		switch msg.String() {
		case "up":
			if p.sel > 0 {
				p.sel--
			}
		case "down":
			if p.sel < len(rows)-1 {
				p.sel++
			}
		case "enter":
			if p.sel < len(rows) {
				return p, rows[p.sel].action()
			}
		case "x":
			if p.sel < len(rows) && rows[p.sel].kind == rowKindSession {
				id := rows[p.sel].id
				if p.store != nil {
					_ = p.store.Delete(id)
					store := p.store
					return p, func() tea.Msg {
						sessions, err := store.List()
						return settingsSessionsMsg{sessions: sessions, err: err}
					}
				}
			}
		}
	}
	return p, nil
}

type settingsRow struct {
	label  string
	kind   settingsRowKind
	id     string // session ID when kind == rowKindSession
	action func() tea.Cmd
}

func (p *SettingsPane) rows() []settingsRow {
	var rows []settingsRow
	for _, a := range p.agents {
		a := a
		marker := " "
		if a == p.currentAgent {
			marker = "●"
		}
		rows = append(rows, settingsRow{
			label: fmt.Sprintf("%s Agent: %s", marker, a),
			kind:  rowKindOption,
			action: func() tea.Cmd {
				if p.runtime == nil {
					return nil
				}
				if err := p.runtime.SwitchAgent(a); err != nil {
					return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Agent switch failed: %v", err)} }
				}
				p.currentAgent = a
				return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Switched to agent: %s", a)} }
			},
		})
	}
	for _, m := range p.models {
		m := m
		marker := " "
		if m == p.currentModel {
			marker = "●"
		}
		rows = append(rows, settingsRow{
			label: fmt.Sprintf("%s Model: %s", marker, m),
			kind:  rowKindOption,
			action: func() tea.Cmd {
				p.currentModel = m
				if p.runtime != nil {
					if err := p.runtime.SaveModel(m); err != nil {
						return func() tea.Msg {
							return chatSystemMsg{text: fmt.Sprintf("Model save failed: %v", err)}
						}
					}
				}
				return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Model set to: %s (restart required)", m)} }
			},
		})
	}
	for _, mode := range []string{"off", "capture", "replay"} {
		mode := mode
		marker := " "
		if mode == p.recording {
			marker = "●"
		}
		rows = append(rows, settingsRow{
			label: fmt.Sprintf("%s Recording: %s", marker, mode),
			kind:  rowKindOption,
			action: func() tea.Cmd {
				if p.runtime != nil {
					_ = p.runtime.SetRecordingMode(mode)
				}
				p.recording = mode
				return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Recording mode: %s", mode)} }
			},
		})
	}
	if len(p.sessions) > 0 {
		for _, s := range p.sessions {
			s := s
			rows = append(rows, settingsRow{
				label:  fmt.Sprintf("  ↺ %s  %s  %s ago", s.ID[:8], s.Agent, formatAge(s.UpdatedAt)),
				kind:   rowKindSession,
				id:     s.ID,
				action: func() tea.Cmd { return func() tea.Msg { return NotifRestoreSessionMsg{ID: s.ID} } },
			})
		}
	}
	return rows
}

// View renders the settings pane.
func (p *SettingsPane) View() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Settings"))
	b.WriteString("\n\n")
	rows := p.rows()
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("No configurable settings available."))
		return b.String()
	}
	inSessionSection := false
	for i, row := range rows {
		if row.kind == rowKindSession && !inSessionSection {
			inSessionSection = true
			b.WriteString("\n" + sectionHeaderStyle.Render("Sessions") + "\n")
		}
		line := row.label
		if i == p.sel {
			line = panelItemActiveStyle.Render(line)
		} else {
			line = panelItemStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	hint := "↑↓ navigate  enter select"
	if p.sel < len(rows) && rows[p.sel].kind == rowKindSession {
		hint += "  x delete"
	}
	b.WriteString("\n" + dimStyle.Render(hint))
	return b.String()
}
