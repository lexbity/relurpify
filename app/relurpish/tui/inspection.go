package tui

import (
	"context"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// InspectionDocument is a detached, read-only document of observed/catalog
// facts. A document's notice states what those facts do and do not establish.
type InspectionDocument struct {
	Title, Notice string
	Entries       []InspectionEntry
}
type InspectionEntry struct {
	ID, Label, Summary, Origin, Body string
	Fields                           []InspectionField
}
type InspectionField struct{ Name, Value string }
type ConfigurationReader interface {
	InspectConfiguration(context.Context) (InspectionDocument, error)
}
type configurationLoadedMsg struct {
	Epoch    uint64
	Document InspectionDocument
	Err      error
}

type documentInspector struct {
	document InspectionDocument
	selected string
	detail   bool
	offset   int
	notice   string
}

func (p *documentInspector) apply(document InspectionDocument) {
	document.Entries = slices.Clone(document.Entries)
	for i := range document.Entries {
		document.Entries[i].Fields = slices.Clone(document.Entries[i].Fields)
	}
	p.document = document
	if p.index() < 0 {
		if p.selected != "" {
			p.detail = false
			p.notice = "The selected entry is no longer present; choose another entry."
		}
		p.selected = ""
		p.offset = 0
		if len(document.Entries) > 0 {
			p.selected = document.Entries[0].ID
		}
	}
}
func (p documentInspector) index() int {
	return slices.IndexFunc(p.document.Entries, func(entry InspectionEntry) bool { return entry.ID == p.selected })
}
func (p *documentInspector) key(message tea.KeyMsg) bool {
	switch message.String() {
	case "esc":
		if p.detail {
			p.detail = false
			p.offset = 0
			return true
		}
		return false
	case "enter":
		p.detail = p.index() >= 0
		p.offset = 0
		return true
	case "up", "down", "pgup", "pgdown", "home", "end":
		delta := 1
		if message.String() == "up" || message.String() == "pgup" {
			delta = -1
		}
		if p.detail {
			p.offset = max(0, p.offset+delta)
			if message.String() == "home" {
				p.offset = 0
			}
			return true
		}
		if len(p.document.Entries) > 0 {
			index := min(max(0, p.index()+delta), len(p.document.Entries)-1)
			if message.String() == "home" {
				index = 0
			}
			if message.String() == "end" {
				index = len(p.document.Entries) - 1
			}
			p.selected = p.document.Entries[index].ID
		}
		return true
	}
	return false
}
func (p documentInspector) view(width, height int) string {
	lines := []string{terminalText(p.document.Title), ""}
	if p.detail && p.index() >= 0 {
		entry := p.document.Entries[p.index()]
		lines = append(lines, terminalText(entry.Label), terminalText(entry.Summary), "Origin: "+terminalText(entry.Origin), "")
		for _, field := range entry.Fields {
			lines = append(lines, terminalText(field.Name)+": "+terminalText(field.Value))
		}
		if entry.Body != "" {
			lines = append(lines, "", terminalText(entry.Body))
		}
		lines = append(lines, "", terminalText(p.document.Notice), "", "Escape returns · ↑/↓ scroll")
		wrapped := strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, width), true), "\n")
		start := min(p.offset, max(0, len(wrapped)-height))
		return boundedScreen(strings.Join(wrapped[start:], "\n"), width, height)
	}
	budget := max(1, height-7)
	start := max(0, p.index()-budget+1)
	for i := start; i < min(len(p.document.Entries), start+budget); i++ {
		entry := p.document.Entries[i]
		marker := " "
		if entry.ID == p.selected {
			marker = ">"
		}
		lines = append(lines, marker+" "+terminalText(entry.Label))
	}
	if len(p.document.Entries) == 0 {
		lines = append(lines, "No observations available.")
	}
	lines = append(lines, "", terminalText(p.document.Notice), "", "↑/↓ select · Enter inspects · Escape returns to the bar")
	if p.notice != "" {
		lines = append(lines, terminalText(p.notice))
	}
	return boundedScreen(strings.Join(lines, "\n"), width, height)
}

func rootHandleConfiguration(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		m.addSystemMessage("Usage: /config")
		return m, nil
	}
	m.setActiveTab(TabConfiguration)
	m.setFocus(FocusRegionRegion1)
	if m.configurationReader == nil {
		m.configuration.apply(InspectionDocument{Title: "Configuration", Notice: "Runtime configuration inspection is unavailable before an execution runtime is composed."})
		return m, nil
	}
	reader, parent := m.configurationReader, m.parentCtx
	m.configurationEpoch++
	epoch := m.configurationEpoch
	return m, func() tea.Msg {
		document, err := reader.InspectConfiguration(parent)
		return configurationLoadedMsg{Epoch: epoch, Document: document, Err: err}
	}
}
