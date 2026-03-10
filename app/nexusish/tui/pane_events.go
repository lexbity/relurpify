package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type EventsPane struct {
	paneState
	follow bool
}

func NewEventsPane() *EventsPane {
	return &EventsPane{follow: true}
}

func (p *EventsPane) HandleCommand(cmd string, args []string) tea.Cmd {
	switch cmd {
	case "follow":
		p.follow = !p.follow
	}
	return nil
}

func (p *EventsPane) View() string {
	lines := make([]string, 0, len(p.state.EventCounts)+1)
	for eventType, count := range p.state.EventCounts {
		line := fmt.Sprintf("%-36s %d", eventType, count)
		if matchesFilter(p.filter, line) {
			lines = append(lines, line)
		}
	}
	if p.follow {
		lines = append([]string{mutedStyle.Render("Follow mode increases refresh cadence while this pane is active.")}, lines...)
	} else {
		lines = append([]string{mutedStyle.Render("Follow mode is off; refresh stays on the normal polling interval.")}, lines...)
	}
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("No events available.")}
	}
	title := "Events"
	if p.follow {
		title += "  " + accentStyle.Render("follow")
	}
	return panelStyle.Width(p.panelWidth()).Render(sectionTitle(title) + "\n" + strings.Join(lines, "\n"))
}

func (p *EventsPane) FollowEnabled() bool { return p.follow }
