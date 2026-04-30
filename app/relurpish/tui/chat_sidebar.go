package tui

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UpdateSidebarFromFrame dispatches to UpdateSidebarFromProposalFrame when the
// frame carries a ContextProposalContent.
func (p *ChatPane) UpdateSidebarFromFrame(frame interaction.InteractionFrame) {
	if content, ok := frame.Content.(interaction.ContextProposalContent); ok {
		p.UpdateSidebarFromProposalFrame(content)
	}
}

// UpdateSidebarFromProposalFrame populates the sidebar from the enrichment
// pipeline output, preserving per-file insertion-action classification and
// session-pin status from the proposal.
func (p *ChatPane) UpdateSidebarFromProposalFrame(content interaction.ContextProposalContent) {
	seen := make(map[string]bool)
	entries := make([]ContextSidebarEntry, 0, len(content.AnchoredFiles)+len(content.ExpandedFiles))

	for _, f := range content.AnchoredFiles {
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		action := f.InsertionAction
		if action == "" {
			action = "direct"
		}
		entries = append(entries, ContextSidebarEntry{
			Path:            f.Path,
			InsertionAction: action,
			IsPin:           true,
		})
	}

	for _, f := range content.ExpandedFiles {
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		action := f.InsertionAction
		if action == "" {
			action = "direct"
		}
		entries = append(entries, ContextSidebarEntry{
			Path:            f.Path,
			InsertionAction: action,
			IsPin:           false,
		})
	}

	p.contextEntries = entries
	p.updateSidebarViewport()
}

// updateSidebarContent refreshes sidebar entries from the raw context file list.
func (p *ChatPane) updateSidebarContent() {
	if p.context == nil {
		p.contextEntries = []ContextSidebarEntry{}
		p.updateSidebarViewport()
		return
	}

	entries := make([]ContextSidebarEntry, 0, len(p.context.Files))
	for _, file := range p.context.Files {
		entries = append(entries, ContextSidebarEntry{
			Path:            file,
			InsertionAction: "direct",
			IsPin:           false,
		})
	}
	p.contextEntries = entries
	p.updateSidebarViewport()
}

func (p *ChatPane) updateSidebarViewport() {
	content := p.renderSidebarContent()
	p.sidebarViewport.SetContent(content)
}

func (p *ChatPane) renderSidebarContent() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("context") + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", p.sidebarWidth-2)) + "\n")

	if len(p.contextEntries) == 0 {
		b.WriteString(dimStyle.Render("no files in context") + "\n")
	} else {
		for i, entry := range p.contextEntries {
			b.WriteString(p.renderSidebarEntry(entry, i == p.sidebarCursor) + "\n")
		}
	}

	b.WriteString("\n" + dimStyle.Render(strings.Repeat("─", p.sidebarWidth-2)) + "\n")
	b.WriteString(dimStyle.Render("[a] add  [x] remove"))
	return b.String()
}

func (p *ChatPane) renderSidebarEntry(entry ContextSidebarEntry, selected bool) string {
	maxPath := p.sidebarWidth - 9
	displayPath := entry.Path
	if len(displayPath) > maxPath {
		displayPath = "..." + displayPath[len(displayPath)-maxPath:]
	}

	prefix := "  "
	if entry.IsPin {
		prefix = "· "
	}

	badge := insertionActionBadge(entry.InsertionAction)
	line := fmt.Sprintf("%s%-*s %s", prefix, maxPath, displayPath, dimStyle.Render(badge))

	if selected {
		return panelItemActiveStyle.Render(line)
	}
	return panelItemStyle.Render(line)
}

func insertionActionBadge(action string) string {
	switch action {
	case "direct":
		return "[dir]"
	case "summarized":
		return "[sum]"
	case "metadata-only":
		return "[ref]"
	default:
		return "[dir]"
	}
}

func (p *ChatPane) renderSidebar() string {
	return lipgloss.NewStyle().
		Width(p.sidebarWidth).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(p.sidebarViewport.View())
}

func (p *ChatPane) ToggleSidebar() {
	p.showSidebar = !p.showSidebar
	p.SetSize(p.width, p.height)
	if p.showSidebar && len(p.contextEntries) == 0 {
		p.updateSidebarContent()
	}
}

func (p *ChatPane) AddFileToSidebar(path string) error {
	if p.context == nil {
		return fmt.Errorf("context unavailable")
	}
	if err := p.context.AddFile(path); err != nil {
		return err
	}
	p.updateSidebarContent()
	return nil
}

func (p *ChatPane) RemoveFileFromSidebar(path string) {
	if p.context == nil {
		return
	}
	p.context.RemoveFile(path)
	filtered := p.contextEntries[:0]
	for _, e := range p.contextEntries {
		if e.Path != path {
			filtered = append(filtered, e)
		}
	}
	p.contextEntries = filtered
	p.updateSidebarViewport()
}

func (p *ChatPane) HandleSidebarKey(key string) tea.Cmd {
	switch key {
	case "up", "k":
		if p.sidebarCursor > 0 {
			p.sidebarCursor--
			p.sidebarViewport.LineUp(1)
		}
	case "down", "j":
		if p.sidebarCursor < len(p.contextEntries)-1 {
			p.sidebarCursor++
			p.sidebarViewport.LineDown(1)
		}
	case "a":
		if p.sidebarCursor >= 0 && p.sidebarCursor < len(p.contextEntries) {
			entry := p.contextEntries[p.sidebarCursor]
			return func() tea.Msg {
				p.AddSystemMessage(fmt.Sprintf("add file: %s", entry.Path))
				return chatSystemMsg{Text: fmt.Sprintf("Added: %s", entry.Path)}
			}
		}
	case "x":
		if p.sidebarCursor >= 0 && p.sidebarCursor < len(p.contextEntries) {
			entry := p.contextEntries[p.sidebarCursor]
			p.RemoveFileFromSidebar(entry.Path)
			return func() tea.Msg {
				p.AddSystemMessage(fmt.Sprintf("removed file: %s", entry.Path))
				return chatSystemMsg{Text: fmt.Sprintf("Removed: %s", entry.Path)}
			}
		}
	}
	return nil
}
