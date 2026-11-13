package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type settingsEntry struct{ label, command, detail string }

func (m RootModel) settingsEntries() []settingsEntry {
	entries := []settingsEntry{
		{label: "Doctor", command: "doctor", detail: "Readiness and setup findings"},
		{label: "Keybindings", command: "keybindings", detail: "Host shortcuts and explicit capture"},
	}
	if m.configurationReader != nil {
		entries = append(entries, settingsEntry{label: "Effective configuration", command: "config", detail: "Read-only policies, capabilities, prompts, tools and contract"})
	}
	if m.services.port != nil {
		entries = append(entries, settingsEntry{label: "Managed services", command: "services", detail: "Review a scoped stop or restart and retain partial failures"})
	}
	if m.provider != nil || m.baseSurface != nil {
		entries = append(entries, settingsEntry{label: "Provider and model", command: "model-provider", detail: "Separate local draft, saved preferences and active configuration"})
	}
	if m.baseSurface != nil {
		entries = append(entries,
			settingsEntry{label: "Sandbox and permissions", command: "permissions", detail: "Workspace, commands, tools and network"},
			settingsEntry{label: "Security guard", command: "securityguard", detail: "Patterns and ingestion controls"})
	}
	if m.workshop.port != nil {
		entries = append(entries, settingsEntry{label: "Recipe workshop", command: "workshop", detail: "Author one saved source artifact"})
	}
	available := make([]settingsEntry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := m.cmdReg.Lookup(entry.command); ok {
			available = append(available, entry)
		}
	}
	return available
}

func (m RootModel) settingsView() string {
	entries := m.settingsEntries()
	lines := []string{"Settings", ""}
	for i, entry := range entries {
		marker := " "
		if i == m.settingsSelection {
			marker = ">"
		}
		lines = append(lines, marker+" "+entry.label)
	}
	if len(entries) > 0 {
		lines = append(lines, "", entries[min(m.settingsSelection, len(entries)-1)].detail)
	}
	lines = append(lines, "", "↑/↓ choose · Enter opens one surface · Escape returns to the bar")
	return strings.Join(lines, "\n")
}

func (m *RootModel) settingsKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	entries := m.settingsEntries()
	switch msg.String() {
	case "up":
		m.settingsSelection = max(0, m.settingsSelection-1)
		return nil, true
	case "down":
		m.settingsSelection = min(max(0, len(entries)-1), m.settingsSelection+1)
		return nil, true
	case "enter":
		if len(entries) == 0 {
			return nil, true
		}
		_, cmd := executeCommand(m, entries[min(m.settingsSelection, len(entries)-1)].command, nil)
		m.setFocus(FocusRegionRegion1)
		return cmd, true
	}
	return nil, false
}

func rootHandleSettings(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		m.addSystemMessage("Usage: /settings")
		return m, nil
	}
	m.settingsSelection = min(m.settingsSelection, max(0, len(m.settingsEntries())-1))
	m.setActiveTab(TabSettings)
	m.setFocus(FocusRegionRegion1)
	return m, nil
}
