package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type WelcomePane struct {
	session       *Session
	store         *SessionStore
	recent        []SessionMeta
	selected      int
	filter        string
	width, height int
}

type workspaceSelectedMsg struct {
	Workspace string
}

func NewWelcomePane(sess *Session, store *SessionStore) *WelcomePane {
	p := &WelcomePane{session: sess, store: store}
	p.Refresh()
	return p
}

func (p *WelcomePane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *WelcomePane) Refresh() {
	if p.store == nil {
		p.recent = nil
		return
	}
	metas, err := p.store.List()
	if err != nil {
		p.recent = nil
		return
	}
	p.recent = append([]SessionMeta(nil), metas...)
	if p.selected >= len(p.recent) {
		p.selected = max(0, len(p.recent)-1)
	}
}

func (p *WelcomePane) SetFilter(filter string) {
	p.filter = strings.ToLower(strings.TrimSpace(filter))
	p.selected = 0
}

func (p *WelcomePane) Update(msg tea.Msg) (*WelcomePane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.selected > 0 {
				p.selected--
			}
		case "down", "j":
			if p.selected < len(p.filteredWorkspaces())-1 {
				p.selected++
			}
		case "r":
			p.Refresh()
		case "enter":
			if ws := p.selectedWorkspace(); ws != "" {
				return p, func() tea.Msg { return workspaceSelectedMsg{Workspace: ws} }
			}
		}
	}
	return p, nil
}

func (p *WelcomePane) filtered() []SessionMeta {
	if p == nil {
		return nil
	}
	if p.filter == "" {
		return append([]SessionMeta(nil), p.recent...)
	}
	var out []SessionMeta
	for _, meta := range p.recent {
		if strings.Contains(strings.ToLower(meta.ID), p.filter) ||
			strings.Contains(strings.ToLower(meta.Workspace), p.filter) ||
			strings.Contains(strings.ToLower(meta.Agent), p.filter) ||
			strings.Contains(strings.ToLower(meta.Model), p.filter) {
			out = append(out, meta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (p *WelcomePane) View() string {
	widths := splitWidths(p.width, 5, 7)
	current := []string{
		dimStyle.Render("workspace") + "  " + fallback(p.sessionField(func(s *Session) string { return s.Workspace }), "(unset)"),
		dimStyle.Render("branch") + "  " + fallback(p.gitBranch(), "(detached)"),
		dimStyle.Render("agent") + "  " + fallback(p.sessionField(func(s *Session) string { return s.Agent }), "none"),
		dimStyle.Render("model") + "  " + fallback(p.sessionField(func(s *Session) string { return s.Model }), "(unset)"),
		dimStyle.Render("provider") + "  " + fallback(p.sessionField(func(s *Session) string { return s.Provider }), "(unset)"),
		dimStyle.Render("mode") + "  " + fallback(p.sessionField(func(s *Session) string { return s.Mode }), "(unset)"),
	}
	if p.session != nil && p.session.Strategy != "" {
		current = append(current, dimStyle.Render("strategy")+"  "+p.session.Strategy)
	}
	items := p.filteredWorkspaces()
	lines := make([]string, 0, len(items))
	for _, meta := range items {
		lines = append(lines, fmt.Sprintf("%s  %s", meta.UpdatedAt.Format("01/02 15:04"), meta.Workspace))
	}
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("No recent workspaces")}
	}
	left := sectionPanel("Workspace", widths[0], current...)
	right := sectionPanel("Recent Workspaces", widths[1], sectionList(lines, p.selected, p.height-10))
	footer := dimStyle.Render("↑↓ navigate  enter switch  r refresh  type to filter")
	if p.filter != "" {
		footer = dimStyle.Render(fmt.Sprintf("filter: %q", p.filter)) + "\n" + footer
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		footer,
	}, "\n\n")
}

func (p *WelcomePane) sessionField(sel func(*Session) string) string {
	if p == nil || p.session == nil {
		return ""
	}
	return sel(p.session)
}

func (p *WelcomePane) filteredWorkspaces() []SessionMeta {
	if p == nil {
		return nil
	}
	seen := make(map[string]SessionMeta)
	for _, meta := range p.filtered() {
		key := strings.TrimSpace(meta.Workspace)
		if key == "" {
			continue
		}
		if existing, ok := seen[key]; !ok || meta.UpdatedAt.After(existing.UpdatedAt) {
			seen[key] = meta
		}
	}
	out := make([]SessionMeta, 0, len(seen))
	for _, meta := range seen {
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (p *WelcomePane) selectedWorkspace() string {
	items := p.filteredWorkspaces()
	if len(items) == 0 {
		return ""
	}
	if p.selected < 0 || p.selected >= len(items) {
		return ""
	}
	return items[p.selected].Workspace
}

func (p *WelcomePane) gitBranch() string {
	workspace := ""
	if p.session != nil {
		workspace = strings.TrimSpace(p.session.Workspace)
	}
	if workspace == "" {
		return ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	cmd := exec.Command("git", "-C", filepath.Clean(workspace), "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
