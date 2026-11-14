package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type guardResultMsg struct {
	owner    *SecurityGuardPane
	epoch    uint64
	section  config.GuardSection
	snapshot GuardPolicySnapshot
	err      error
}

// SecurityGuardPane inspects disk rules and tests a selected shell pattern.
// It has no write, activation, execution, or permission-granting capability.
// Policy editing is deferred until G1 grammar and G2 ingestion enforcement exist.
type SecurityGuardPane struct {
	parent        context.Context
	port          GuardPolicyPort
	section       config.GuardSection
	snapshots     map[config.GuardSection]GuardPolicySnapshot
	views         map[config.GuardSection]*documentInspector
	rules         config.GuardPolicyRules
	testField     textinput.Model
	testMode      bool
	working       bool
	epoch         uint64
	status        string
	width, height int
	demo          bool
}

// NewSecurityGuardPane constructs the shared read-only inspector without I/O.
func NewSecurityGuardPane(parent context.Context, port GuardPolicyPort, demo bool) *SecurityGuardPane {
	if parent == nil {
		panic("guard inspector requires a parent context")
	}
	field := textinput.New()
	field.CharLimit = 65536
	field.Prompt = "> "
	return &SecurityGuardPane{parent: parent, port: port, demo: demo, section: config.GuardShell, snapshots: make(map[config.GuardSection]GuardPolicySnapshot), views: make(map[config.GuardSection]*documentInspector), testField: field}
}

// SetSize assigns the bounded inspector and explicit sample-input field.
func (p *SecurityGuardPane) SetSize(width, height int) {
	p.width, p.height = width, height
	p.testField.Width = max(1, width-4)
}

// OwnsEscape preserves an open detail or explicit matcher field before bar focus.
func (p *SecurityGuardPane) OwnsEscape() bool {
	view := p.views[p.section]
	return p.testMode || (view != nil && view.detail)
}

func (p *SecurityGuardPane) open() tea.Cmd {
	if p.working || p.views[p.section] != nil {
		return nil
	}
	return p.query(p.section)
}
func (p *SecurityGuardPane) query(section config.GuardSection) tea.Cmd {
	if p.working {
		return nil
	}
	if p.port == nil {
		p.status = "Guard inspection unavailable: no policy reader is connected"
		return nil
	}
	p.working = true
	p.status = "Reading " + string(section) + " policy source…"
	p.epoch++
	epoch, parent, port := p.epoch, p.parent, p.port
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		snapshot, err := port.GuardPolicy(ctx, section)
		return guardResultMsg{owner: p, epoch: epoch, section: section, snapshot: snapshot, err: err}
	}
}
func (p *SecurityGuardPane) apply(result guardResultMsg) {
	if result.owner != p || result.epoch != p.epoch || !p.working {
		return
	}
	p.working = false
	if result.err != nil {
		p.status = "Inspection failed: " + result.err.Error()
		if len(p.snapshots) > 0 {
			p.status += " · prior observation retained"
		}
		return
	}
	if result.snapshot.Document.Section != result.section {
		p.status = "Policy response section does not match the request; prior observation retained"
		return
	}
	p.section = result.section
	p.snapshots[p.section] = result.snapshot
	p.rules, _ = config.ParseGuardPolicy(p.section, result.snapshot.Document.Source)
	document := guardInspection(result.snapshot)
	view := p.views[p.section]
	if view == nil {
		view = &documentInspector{}
		p.views[p.section] = view
	}
	view.apply(document)
	p.status = "Read-only source observation. Live enforcement is not established (G1/G2)."
}
func guardInspection(snapshot GuardPolicySnapshot) InspectionDocument {
	section := snapshot.Document.Section
	document := InspectionDocument{Title: "Settings / Guard / " + string(section), Notice: "Read-only inspection · G1/G2 enforcement unresolved. Tab changes section; r refreshes; t tests a shell pattern only."}
	if snapshot.Notice != "" {
		document.Notice += " " + snapshot.Notice
	}
	rules, err := config.ParseGuardPolicy(section, snapshot.Document.Source)
	if err != nil {
		document.Entries = append(document.Entries, InspectionEntry{ID: "diagnostics", Label: "Policy source diagnostics", Origin: snapshot.Workspace, Body: err.Error()})
	}
	for _, rule := range rules.Shell {
		document.Entries = append(document.Entries, InspectionEntry{ID: rule.ID, Label: rule.ID + " · " + rule.Action, Origin: snapshot.Workspace, Summary: "Configured rule; observed enforcement is not established", Fields: []InspectionField{{Name: "Pattern", Value: rule.Pattern}, {Name: "Action", Value: rule.Action}, {Name: "Reason", Value: rule.Reason}, {Name: "Source revision", Value: snapshot.Document.Revision}}})
	}
	for _, rule := range rules.Ingestion {
		var actors []string
		for _, actor := range rule.Conditions.Actors {
			actors = append(actors, actor.ID)
		}
		document.Entries = append(document.Entries, InspectionEntry{ID: rule.ID, Label: fmt.Sprintf("%s · enabled=%t · %s", rule.ID, rule.Enabled, rule.Effect.Action), Origin: snapshot.Workspace, Summary: "Configured ingestion declaration; G2 evaluator is unresolved", Fields: []InspectionField{{Name: "Name", Value: rule.Name}, {Name: "Priority", Value: fmt.Sprint(rule.Priority)}, {Name: "Actor IDs", Value: strings.Join(actors, ", ")}, {Name: "Approvers", Value: strings.Join(rule.Effect.Approvers, ", ")}, {Name: "Approval TTL", Value: rule.Effect.ApprovalTTL}, {Name: "Reason", Value: rule.Effect.Reason}, {Name: "Source revision", Value: snapshot.Document.Revision}}})
	}
	if err == nil && len(document.Entries) == 0 {
		document.Notice = "No rules in the inspected file. This does not establish authorization or protection. Tab changes section; r refreshes."
	}
	return document
}

func (p *SecurityGuardPane) beginTest() {
	if p.section != config.GuardShell {
		p.status = "Ingestion sample evaluation is unavailable until G2 is implemented"
		return
	}
	view := p.views[p.section]
	if view == nil {
		return
	}
	for _, rule := range p.rules.Shell {
		if rule.ID == view.selected {
			p.testMode = true
			p.testField.SetValue("")
			p.testField.Focus()
			return
		}
	}
	p.status = "Select a decoded shell rule before testing its pattern"
}
func (p *SecurityGuardPane) runTest() {
	view := p.views[p.section]
	if view == nil {
		p.testMode = false
		return
	}
	for _, rule := range p.rules.Shell {
		if rule.ID != view.selected {
			continue
		}
		matcher, err := sandbox.NewShellBlacklist([]sandbox.BlacklistRule{{ID: rule.ID, Raw: rule.Pattern, Action: sandbox.BlacklistAction(rule.Action), Reason: rule.Reason}})
		switch {
		case err != nil:
			p.status = "Pattern test error: " + err.Error()
		case matcher.Check(p.testField.Value()) != nil:
			p.status = "Matches selected pattern: " + rule.Action + " · command not executed"
		default:
			p.status = "No match · matcher only; this does not establish authorization"
		}
		p.testMode = false
		return
	}
	p.testMode = false
	p.status = "Selected rule is no longer present; no test was performed"
}

// Update consumes only inspection replies and deliberate local interaction.
func (p *SecurityGuardPane) Update(message tea.Msg) (*SecurityGuardPane, tea.Cmd) {
	switch msg := message.(type) {
	case guardResultMsg:
		p.apply(msg)
	case tea.KeyMsg:
		if p.testMode {
			switch msg.Type {
			case tea.KeyEsc:
				p.testMode = false
				return p, nil
			case tea.KeyEnter:
				p.runTest()
				return p, nil
			}
			if msg.Type == tea.KeyRunes && (len(string(msg.Runes))+len(p.testField.Value()) > 65536 || strings.ContainsAny(string(msg.Runes), "\x00\x1b\r\n")) {
				p.status = "Sample input rejected; previous input retained"
				return p, nil
			}
			var cmd tea.Cmd
			p.testField, cmd = p.testField.Update(msg)
			return p, cmd
		}
		if msg.Paste {
			return p, nil
		}
		if msg.Type == tea.KeyTab || msg.Type == tea.KeyShiftTab {
			if p.working {
				return p, nil
			}
			next := config.GuardIngestion
			if p.section == config.GuardIngestion {
				next = config.GuardShell
			}
			if snapshot, ok := p.snapshots[next]; ok {
				p.section = next
				p.rules, _ = config.ParseGuardPolicy(next, snapshot.Document.Source)
				p.status = "Cached " + string(next) + " source observation; live enforcement is not established"
				return p, nil
			}
			return p, p.query(next)
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 't':
				p.beginTest()
				return p, nil
			case 'r':
				return p, p.query(p.section)
			}
		}
		if view := p.views[p.section]; view != nil {
			view.key(msg)
		}
	}
	return p, nil
}

// View never queries, writes, activates, or executes. Missing data is explicit.
func (p *SecurityGuardPane) View() string {
	width, height := max(1, p.width), max(1, p.height)
	if p.testMode {
		return boundedScreen("Guard / selected shell pattern\n\nMatcher test only; command not executed.\nLive shell enforcement is not established (G1).\n\n"+p.testField.View()+"\n\nEnter tests · Escape cancels · F6 returns to the bar\n\n"+terminalText(p.status), width, height)
	}
	if view := p.views[p.section]; view != nil {
		// Keep operation feedback outside the scrolling observation document.
		rows := max(1, height-2)
		body := view.view(width, rows)
		return boundedScreen(body+"\n"+terminalText(p.status), width, height)
	}
	lines := []string{"Settings / Guard / " + string(p.section), "", "Read-only policy inspection and shell pattern test.", "Rule editing is deferred until G1/G2 enforcement is real.", "", "Open /securityguard or press r to inspect policy source."}
	if p.working {
		lines = append(lines, "Reading policy source…")
	}
	if p.status != "" {
		lines = append(lines, "", terminalText(p.status))
	}
	if p.demo {
		lines = append(lines, "", "DEMO: inspection fixture only; no file or command effects")
	}
	return boundedScreen(strings.Join(lines, "\n"), width, height)
}
