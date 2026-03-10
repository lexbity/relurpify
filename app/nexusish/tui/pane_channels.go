package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ChannelsPane struct {
	subtabPaneState
}

func NewChannelsPane() *ChannelsPane {
	p := &ChannelsPane{}
	p.initTabs([]SubTab{{Label: "Adapters"}, {Label: "Config"}})
	return p
}

func (p *ChannelsPane) HandleCommand(cmd string, args []string) tea.Cmd {
	if p.activeSubtab() != 0 || len(args) == 0 {
		return nil
	}
	switch cmd {
	case "restart":
		channel := strings.TrimSpace(args[0])
		return func() tea.Msg { return channelActionRequestMsg{action: "restart", channel: channel} }
	default:
		return nil
	}
}

func (p *ChannelsPane) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "[":
		p.stepSubtab(-1, 1)
	case "]":
		p.stepSubtab(1, 1)
	}
	return nil
}

func (p *ChannelsPane) View() string {
	if p.activeSubtab() == 1 {
		lines := p.configLines()
		return panelStyle.Width(p.panelWidth()).Render(sectionTitle("Config") + "\n" + strings.Join(lines, "\n"))
	}
	lines := make([]string, 0, len(p.state.Channels))
	for _, channel := range p.state.Channels {
		status := "down"
		if channel.Connected {
			status = "up"
		}
		line := fmt.Sprintf("%-16s %-4s configured=%-3s in=%-4d out=%-4d retries=%-2d", channel.Name, status, yesNo(channel.Configured), channel.Inbound, channel.Outbound, channel.Reconnects)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
		if channel.LastError != "" && matchesFilter(p.filter, channel.LastError) {
			lines = append(lines, mutedStyle.Render("  error: "+channel.LastError))
		}
	}
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("No channel data available.")}
	}
	return panelStyle.Width(p.panelWidth()).Render(sectionTitle("Adapters") + "\n" + strings.Join(lines, "\n"))
}

func (p *ChannelsPane) configLines() []string {
	lines := []string{
		mutedStyle.Render("Configuration editing is not wired to the admin MCP backend yet."),
		"",
		"Expected fields:",
		"  - bot_token / adapter credential material",
		"  - default session scope policy",
		"  - per-channel enablement and restart controls",
	}
	if p.filter == "" {
		return lines
	}
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if matchesFilter(p.filter, line) {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return []string{mutedStyle.Render("No matching channel config items.")}
	}
	return filtered
}
