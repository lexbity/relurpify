package tui

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ──────────────────────────────────────────────────────────────
// Chat context sidebar — euclo-specific
//
// The sidebar surfaces confirmed files and session pins produced by
// ContextProposalPhase.  Its state lives in ChatPane but all methods
// are defined here to keep euclo-specific TUI behaviour out of
// pane_chat.go.
// ──────────────────────────────────────────────────────────────

// UpdateSidebarFromFrame implements ChatPaner. It dispatches to
// UpdateSidebarFromProposalFrame when the frame carries a ContextProposalContent.
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
	entries := make([]ContextSidebarEntry, 0,
		len(content.AnchoredFiles)+len(content.ExpandedFiles))

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
			IsPin:           true, // anchored files are session pins
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
// This is the fallback path used before any ContextProposalContent frame arrives.
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

// updateSidebarViewport regenerates the viewport content string from
// the current contextEntries slice.
func (p *ChatPane) updateSidebarViewport() {
	content := p.renderSidebarContent()
	p.sidebarViewport.SetContent(content)
}

// renderSidebarContent generates the full sidebar content string.
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

// renderSidebarEntry renders a single sidebar entry line.
func (p *ChatPane) renderSidebarEntry(entry ContextSidebarEntry, selected bool) string {
	maxPath := p.sidebarWidth - 9 // room for prefix + badge
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

// insertionActionBadge returns the display badge for an insertion action.
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

// renderSidebar renders the full sidebar widget (border + viewport).
func (p *ChatPane) renderSidebar() string {
	return lipgloss.NewStyle().
		Width(p.sidebarWidth).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(p.sidebarViewport.View())
}

// ToggleSidebar shows or hides the context sidebar and recalculates pane sizes.
func (p *ChatPane) ToggleSidebar() {
	p.showSidebar = !p.showSidebar
	p.SetSize(p.width, p.height)
	if p.showSidebar && len(p.contextEntries) == 0 {
		p.updateSidebarContent()
	}
}

// AddFileToSidebar adds a file to the context and refreshes the sidebar.
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

// RemoveFileFromSidebar removes a file from context and refreshes the sidebar.
func (p *ChatPane) RemoveFileFromSidebar(path string) {
	if p.context == nil {
		return
	}
	p.context.RemoveFile(path)
	// Also remove from contextEntries directly so sidebar updates immediately
	// without waiting for the next proposal frame.
	filtered := p.contextEntries[:0]
	for _, e := range p.contextEntries {
		if e.Path != path {
			filtered = append(filtered, e)
		}
	}
	p.contextEntries = filtered
	p.updateSidebarViewport()
}

// HandleSidebarKey routes a key event to sidebar navigation and actions.
// Returns a tea.Cmd when an action produces a side-effect message.
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
	case "x", "d":
		if p.sidebarCursor >= 0 && p.sidebarCursor < len(p.contextEntries) {
			entry := p.contextEntries[p.sidebarCursor]
			if p.runtime != nil {
				_ = p.runtime.DropFileFromContext(entry.Path)
			}
			p.RemoveFileFromSidebar(entry.Path)
			if p.sidebarCursor >= len(p.contextEntries) {
				p.sidebarCursor = max(0, len(p.contextEntries)-1)
			}
			return func() tea.Msg {
				return chatSystemMsg{Text: fmt.Sprintf("removed from context: %s", entry.Path)}
			}
		}
	case "a":
		// Delegate to the /add command flow via input bar.
		return func() tea.Msg {
			return chatSystemMsg{Text: "use /add <path> or @<path> to add a file"}
		}
	}
	return nil
}
