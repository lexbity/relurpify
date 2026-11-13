package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ServiceOperation struct {
	Action, Workspace string
	IDs               []string
}
type ServiceOperationResult struct{ ID, Error string }
type ServiceOutcome struct {
	Services []ServiceInfo
	Results  []ServiceOperationResult
}
type ServicePort interface {
	Services(context.Context) ([]ServiceInfo, error)
	ControlServices(context.Context, ServiceOperation) (ServiceOutcome, error)
}
type servicesResultMsg struct {
	Epoch    uint64
	Services []ServiceInfo
	Results  []ServiceOperationResult
	Err      error
	Review   ServiceOperation
}

type ServicesPane struct {
	parent       context.Context
	port         ServicePort
	items        []ServiceInfo
	selected     string
	detail       bool
	review       *ServiceOperation
	reviewOffset int
	working      bool
	epoch        uint64
	status       string
	results      []ServiceOperationResult
	demo         bool
	workspace    string
}

func NewServicesPane(parent context.Context, port ServicePort, demo bool) *ServicesPane {
	return &ServicesPane{parent: parent, port: port, demo: demo}
}
func (p *ServicesPane) index() int {
	return slices.IndexFunc(p.items, func(item ServiceInfo) bool { return item.ID == p.selected })
}
func (p *ServicesPane) open(review ServiceOperation) tea.Cmd {
	if p.working {
		p.status = "Wait for the pending service operation"
		return nil
	}
	if p.port == nil {
		p.status = "Managed services are unavailable in this session"
		return nil
	}
	p.epoch++
	epoch, port, parent := p.epoch, p.port, p.parent
	p.working = true
	return func() tea.Msg {
		items, err := port.Services(parent)
		return servicesResultMsg{Epoch: epoch, Services: items, Review: review, Err: err}
	}
}
func (p *ServicesPane) apply(msg servicesResultMsg) {
	if msg.Epoch != p.epoch {
		return
	}
	p.working = false
	if msg.Services != nil {
		p.items = slices.Clone(msg.Services)
		slices.SortFunc(p.items, func(a, b ServiceInfo) int { return strings.Compare(a.ID, b.ID) })
		if p.index() < 0 {
			p.selected = ""
			p.detail = false
			if len(p.items) > 0 {
				p.selected = p.items[0].ID
			}
		}
	}
	p.results = slices.Clone(msg.Results)
	if msg.Err != nil {
		p.status = msg.Err.Error()
		return
	}
	if msg.Review.Action != "" {
		p.prepare(msg.Review)
		return
	}
	failed := 0
	for _, result := range msg.Results {
		if result.Error != "" {
			failed++
		}
	}
	if failed > 0 {
		p.status = fmt.Sprintf("%d of %d service operations failed; inspect retained results", failed, len(msg.Results))
	} else if len(msg.Results) > 0 {
		p.status = "Operation returned; the displayed service states are the latest observation"
	} else {
		p.status = "Managed services refreshed"
	}
	if p.demo {
		p.status = "DEMO: " + p.status + " (memory only)"
	}
}
func (p *ServicesPane) prepare(operation ServiceOperation) {
	if operation.Action != "stop" && operation.Action != "restart" && operation.Action != "restart-all" {
		p.status = "Unknown service operation"
		return
	}
	if operation.Action == "restart-all" {
		operation.IDs = nil
		for _, item := range p.items {
			operation.IDs = append(operation.IDs, item.ID)
		}
	}
	if len(operation.IDs) == 0 && p.index() >= 0 {
		operation.IDs = []string{p.selected}
	}
	if len(operation.IDs) == 0 {
		p.status = "No registered service selected"
		return
	}
	for _, id := range operation.IDs {
		if !slices.ContainsFunc(p.items, func(item ServiceInfo) bool { return item.ID == id }) {
			p.status = "Service no longer registered: " + id
			return
		}
	}
	operation.IDs = slices.Clone(operation.IDs)
	operation.Workspace = p.workspace
	p.review = &operation
	p.reviewOffset = 0
}
func (p *ServicesPane) key(msg tea.KeyMsg) (tea.Cmd, bool) {
	if p.working {
		return nil, msg.String() != "esc"
	}
	if p.review != nil {
		switch msg.String() {
		case "up", "pgup":
			p.reviewOffset = max(0, p.reviewOffset-1)
			return nil, true
		case "down", "pgdown":
			p.reviewOffset = min(max(0, len(p.review.IDs)-1), p.reviewOffset+1)
			return nil, true
		case "esc":
			p.review = nil
			return nil, true
		case "enter":
			operation := *p.review
			operation.IDs = slices.Clone(operation.IDs)
			p.review = nil
			p.working = true
			p.epoch++
			epoch, port, parent := p.epoch, p.port, p.parent
			return func() tea.Msg {
				outcome, err := port.ControlServices(parent, operation)
				return servicesResultMsg{Epoch: epoch, Services: outcome.Services, Results: outcome.Results, Err: err}
			}, true
		}
		return nil, true
	}
	switch msg.String() {
	case "esc":
		if p.detail {
			p.detail = false
			return nil, true
		}
		return nil, false
	case "up", "down":
		if len(p.items) > 0 {
			delta := 1
			if msg.String() == "up" {
				delta = -1
			}
			p.selected = p.items[min(max(0, p.index()+delta), len(p.items)-1)].ID
		}
		return nil, true
	case "enter":
		p.detail = p.index() >= 0
		return nil, true
	case "s":
		p.prepare(ServiceOperation{Action: "stop"})
		return nil, true
	case "r":
		p.prepare(ServiceOperation{Action: "restart"})
		return nil, true
	case "R":
		p.prepare(ServiceOperation{Action: "restart-all"})
		return nil, true
	}
	return nil, false
}
func (p *ServicesPane) view(width, height int) string {
	lines := []string{"Settings / Managed services", ""}
	if p.review != nil {
		lines = append(lines, fmt.Sprintf("Review %s · %d services", p.review.Action, len(p.review.IDs)), "")
		budget := max(1, height-12)
		start := min(p.reviewOffset, max(0, len(p.review.IDs)-budget))
		for _, id := range p.review.IDs[start:min(len(p.review.IDs), start+budget)] {
			lines = append(lines, "  "+terminalText(id))
		}
		lines = append(lines, "", "Only the listed registered service IDs are included.", "↑/↓ scroll the reviewed scope", "", "Enter confirms · Escape cancels")
	} else if p.detail && p.index() >= 0 {
		item := p.items[p.index()]
		lines = append(lines, item.ID, "Observed state: "+string(item.Status), "Owner: "+item.Owner, "Source: "+item.Source, "")
		lines = append(lines, item.Notes...)
		lines = append(lines, "", "Escape returns · s stop · r restart")
	} else {
		budget := max(1, height-8)
		start := max(0, p.index()-budget+1)
		for i := start; i < min(len(p.items), start+budget); i++ {
			item := p.items[i]
			marker := " "
			if item.ID == p.selected {
				marker = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %-26s %s", marker, terminalText(item.ID), terminalText(string(item.Status))))
		}
		if len(p.items) == 0 {
			lines = append(lines, "No managed services observed.")
		}
		lines = append(lines, "", "↑/↓ select · Enter detail · s stop · r restart · R review all")
	}
	if p.working {
		lines = append(lines, "", "Waiting for the application result…")
	}
	if p.status != "" {
		lines = append(lines, "", terminalText(p.status))
	}
	for _, result := range p.results {
		if result.Error != "" {
			lines = append(lines, terminalText(result.ID+": "+result.Error))
		}
	}
	if p.demo && p.review != nil {
		lines = append(lines, "DEMO: this changes scenario memory only.")
	}
	return boundedScreen(terminalText(strings.Join(lines, "\n")), width, height)
}

func rootHandleServices(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		m.addSystemMessage("Usage: /services")
		return m, nil
	}
	m.setActiveTab(TabServices)
	m.setFocus(FocusRegionRegion1)
	return m, m.services.open(ServiceOperation{})
}
