package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View composes the scrollable feed, prompt bar, and status bar.
func (m Model) View() string {
	if !m.ready || m.feed == nil {
		return "Initializing..."
	}

	feed := m.feed.View()
	panel := ""
	switch m.mode {
	case ModeCommand:
		panel = m.renderCommandPalette()
	case ModeFilePicker:
		panel = m.renderFilePicker()
	}
	prompt := m.renderPromptBar()
	status := m.statusBar.View(m.width)

	if panel != "" {
		return lipgloss.JoinVertical(lipgloss.Left, feed, panel, prompt, status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, feed, prompt, status)
}

func (m Model) renderMessages() string {
	if len(m.messages) == 0 {
		return welcomeStyle.Render("Welcome! Type a message or use /help for commands.")
	}
	rendered := make([]string, 0, len(m.messages))
	spinnerView := m.spinner.View()
	for _, msg := range m.messages {
		rendered = append(rendered, RenderMessage(msg, m.width, spinnerView))
	}
	return strings.Join(rendered, "\n\n")
}

func (m Model) renderPromptBar() string {
	prefix := "> "
	hint := dimStyle.Render(" / for commands | @ for context | tab toggles " + m.expandTarget + " | shift+tab cycles | ctrl+l clears")
	promptText := ""

	switch m.mode {
	case ModeCommand:
		prefix = "/ "
		hint = dimStyle.Render(" Enter to run | Esc to cancel | Tab to autocomplete | ↑/↓ select")
	case ModeFilePicker:
		prefix = "@ "
		hint = dimStyle.Render(" Enter to add file | Esc to cancel | ↑/↓ select")
	case ModeHITL:
		prefix = "! "
		hint = dimStyle.Render(" y approve | n deny | Esc cancel")
		if m.hitlRequest != nil {
			promptText = fmt.Sprintf("Approve %s: %s (%s)?", m.hitlRequest.ID, m.hitlRequest.Permission.Action, m.hitlRequest.Justification)
		} else {
			promptText = "Approve pending permission?"
		}
	}
	if m.hasActiveRuns() && m.mode == ModeNormal {
		hint = dimStyle.Render(" streaming... pgup/down to scroll | ctrl+c to quit")
	}

	content := prefix
	if m.mode == ModeHITL {
		content += promptText
	} else {
		content += m.input.View()
	}
	if hint != "" {
		content += " " + hint
	}
	return promptBarStyle.Width(m.width).Render(content)
}

func (m Model) renderFilePicker() string {
	if m.filePicker.loading {
		return panelStyle.Width(m.width).Render("Indexing files...")
	}
	if m.filePicker.err != nil {
		return panelStyle.Width(m.width).Render(fmt.Sprintf("File index error: %v", m.filePicker.err))
	}
	if len(m.filePicker.filtered) == 0 {
		return panelStyle.Width(m.width).Render("No matching files")
	}
	lines := make([]string, 0, len(m.filePicker.filtered)+1)
	lines = append(lines, panelHeaderStyle.Render("Context Files"))
	for i, entry := range m.filePicker.filtered {
		line := renderFileEntry(entry)
		if i == m.filePicker.selected {
			line = panelItemActiveStyle.Render(line)
		} else {
			line = panelItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return panelStyle.Width(m.width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderCommandPalette() string {
	if len(m.commandPalette.items) == 0 {
		return panelStyle.Width(m.width).Render("No matching commands")
	}
	lines := make([]string, 0, len(m.commandPalette.items)+1)
	lines = append(lines, panelHeaderStyle.Render("Command Palette"))
	for i, item := range m.commandPalette.items {
		line := renderCommandItem(item)
		if i == m.commandPalette.selected {
			line = panelItemActiveStyle.Render(line)
		} else {
			line = panelItemStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return panelStyle.Width(m.width).Render(strings.Join(lines, "\n"))
}
