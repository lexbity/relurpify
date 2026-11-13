package tui

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	providerLoad     = "load"
	providerSave     = "save"
	providerDiscard  = "discard"
	providerProbe    = "probe"
	providerActivate = "activate"
	providerChoice   = "provider"
)

// ProviderEditorPort stores editor preferences and probes an explicit draft.
// Saving preferences and activating execution configuration are separate ports.
type ProviderEditorPort interface {
	ProviderSettings(context.Context) (ProviderEditorState, error)
	SaveProviderSettings(context.Context, ProviderSaveRequest) (ProviderEditorState, error)
	ProbeProviderSettings(context.Context, config.RuntimeProviderConfig) (ProviderProbeResult, error)
}

// ProviderActivationPort activates an exact saved revision independently of saving.
type ProviderActivationPort interface {
	ActivateProviderSettings(context.Context, string) (ProviderEditorState, error)
}

// ProviderOption is a selectable provider declaration, not a health observation.
type ProviderOption struct{ Name, Kind, Endpoint string }

// ProviderEditorState separates saved preferences from active runtime facts.
type ProviderEditorState struct {
	Workspace                                       string
	Saved                                           config.RuntimeProviderConfig
	Revision, ActiveRevision, ActiveSummary, Notice string
	Catalog                                         []ProviderOption
}

// ProviderSaveRequest captures the workspace and expected saved revision.
type ProviderSaveRequest struct {
	Workspace, ExpectedRevision string
	Profile                     config.RuntimeProviderConfig
}

// ProviderProbeResult contains the observations of an explicit diagnostic probe.
type ProviderProbeResult struct {
	State, Detail string
	Models        []string
}
type providerResultMsg struct {
	owner         *ProviderPane
	Request, Edit uint64
	Operation     string
	State         ProviderEditorState
	Probe         ProviderProbeResult
	Err           error
}

// ProviderPane keeps a local draft, saved preferences and active-runtime facts
// distinct. Constructors and navigation never contact a model provider.
type ProviderPane struct {
	parent        context.Context
	port          ProviderEditorPort
	activator     ProviderActivationPort
	state         ProviderEditorState
	draft         config.RuntimeProviderConfig
	probe         *ProviderProbeResult
	selected      int
	field         textinput.Model
	editing       bool
	fieldOriginal string
	chooser       string
	choice        int
	review        string
	operation     string
	request, edit uint64
	working       bool
	status        string
	width, height int
	demo          bool
}

// NewProviderPane builds the host-owned editor without I/O or probes.
func NewProviderPane(parent context.Context, port ProviderEditorPort, activator ProviderActivationPort, demo bool) *ProviderPane {
	field := textinput.New()
	field.CharLimit = 8192
	field.Prompt = "> "
	return &ProviderPane{parent: parent, port: port, activator: activator, field: field, demo: demo}
}
func (p *ProviderPane) resize(width, height int) {
	p.width, p.height = width, height
	p.field.Width = max(1, width-4)
}
func (p *ProviderPane) ownsEscape() bool { return p.editing || p.chooser != "" || p.review != "" }
func (p *ProviderPane) dirty() bool {
	a, b := p.draft, p.state.Saved
	a.LastUpdated, b.LastUpdated = 0, 0
	return a != b || (p.editing && p.field.Value() != p.fieldOriginal)
}
func (p *ProviderPane) open() tea.Cmd {
	if p.working {
		p.status = "Wait for the pending provider operation; the current draft is retained"
		return nil
	}
	if p.port == nil {
		p.status = "Provider editor service unavailable in this session"
		return nil
	}
	if p.dirty() {
		p.status = "Local provider draft retained; save or discard it before refreshing"
		return nil
	}
	p.request++
	request, port, parent := p.request, p.port, p.parent
	p.working = true
	p.operation = providerLoad
	return func() tea.Msg {
		state, err := port.ProviderSettings(parent)
		return providerResultMsg{owner: p, Request: request, Operation: providerLoad, State: state, Err: err}
	}
}
func (p *ProviderPane) apply(msg providerResultMsg) {
	if msg.owner != p || msg.Request != p.request {
		return
	}
	p.working = false
	if msg.Operation == providerProbe && msg.Edit != p.edit {
		p.status = "Stale provider probe discarded; the newer draft was retained"
		return
	}
	if msg.Err != nil {
		p.status = msg.Operation + " failed: " + msg.Err.Error()
		return
	}
	switch msg.Operation {
	case providerLoad, providerSave, providerActivate:
		p.state = msg.State
		p.draft = msg.State.Saved
		p.edit++
		p.probe = nil
		p.status = msg.State.Notice
	case providerProbe:
		p.probe = &msg.Probe
		p.status = "Observed probe: " + msg.Probe.State + " · " + msg.Probe.Detail
	}
	if p.demo {
		p.status = "DEMO: " + p.status + " (scenario state only)"
	}
}
func (p *ProviderPane) values() []string {
	return []string{p.draft.Provider, p.draft.Endpoint, p.draft.Model, p.draft.Timeout, fmt.Sprint(p.draft.NativeToolCalling)}
}
func providerFieldLabels() []string {
	return []string{"Provider", "Endpoint", "Model", "Timeout", "Native tool calling"}
}

// ValidateProviderDraft rejects invalid, oversized, credential-bearing or terminal-control fields.
func ValidateProviderDraft(profile config.RuntimeProviderConfig) error {
	for _, value := range []string{profile.Provider, profile.Endpoint, profile.Model, profile.Timeout} {
		if len(value) > 8192 || !utf8.ValidString(value) {
			return fmt.Errorf("provider fields must contain at most 8192 UTF-8 bytes")
		}
		if strings.ContainsFunc(value, unicode.IsControl) {
			return fmt.Errorf("provider fields cannot contain control characters")
		}
	}
	if strings.TrimSpace(profile.Model) == "" {
		return fmt.Errorf("model name is required")
	}

	if strings.TrimSpace(profile.Provider) == "" {
		return fmt.Errorf("provider name is required")
	}
	if profile.Endpoint != "" {
		endpoint, err := url.Parse(profile.Endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return fmt.Errorf("endpoint must be an HTTP(S) URL without embedded credentials")
		}
	}
	if profile.Timeout != "" {
		timeout, err := time.ParseDuration(profile.Timeout)
		if err != nil || timeout <= 0 || timeout > 10*time.Minute {
			return fmt.Errorf("timeout must be greater than zero and at most 10m")
		}
	}
	return nil
}
func (p *ProviderPane) stage(value string) {
	switch p.selected {
	case 0:
		p.draft.Provider = value
	case 1:
		p.draft.Endpoint = value
	case 2:
		p.draft.Model = value
	case 3:
		p.draft.Timeout = value
	case 4:
		p.draft.NativeToolCalling = value == "true"
	}
	p.edit++
	p.probe = nil
	p.status = "Local provider draft changed; saved/active settings are unchanged"
}
func (p *ProviderPane) options() []string {
	if p.chooser == CmdModel {
		if p.probe != nil {
			return slices.Clone(p.probe.Models)
		}
		return nil
	}
	var choices []string
	for _, provider := range p.state.Catalog {
		choices = append(choices, provider.Name)
	}
	return choices
}
func (p *ProviderPane) key(msg tea.KeyMsg) (tea.Cmd, bool) {
	if p.working && p.operation != providerProbe {
		return nil, msg.String() != keyEsc
	}
	if p.review != "" {
		switch msg.String() {
		case keyEsc:
			p.review = ""
			return nil, true
		case keyEnter:
			operation := p.review
			p.review = ""
			return p.perform(operation), true
		}
		return nil, true
	}
	if p.editing {
		switch msg.String() {
		case keyEsc:
			p.editing = false
			return nil, true
		case keyEnter:
			p.stage(p.field.Value())
			p.editing = false
			return nil, true
		}
		if msg.Paste && strings.ContainsAny(string(msg.Runes), "\x1b\x00\n\r") {
			p.status = "Paste rejected: provider fields accept one line without terminal controls"
			return nil, true
		}
		var cmd tea.Cmd
		p.field, cmd = p.field.Update(msg)
		return cmd, true
	}
	if p.chooser != "" {
		choices := p.options()
		switch msg.String() {
		case keyEsc:
			p.chooser = ""
		case "up":
			p.choice = max(0, p.choice-1)
		case keyDown:
			p.choice = min(max(0, len(choices)-1), p.choice+1)
		case keyEnter:
			if len(choices) > 0 {
				if p.chooser == CmdModel {
					p.selected = 2
				} else {
					p.selected = 0
				}
				p.stage(choices[p.choice])
				if p.selected == 0 {
					for _, provider := range p.state.Catalog {
						if provider.Name == p.draft.Provider {
							p.draft.Endpoint = provider.Endpoint
						}
					}
				}
			}
			p.chooser = ""
		}
		return nil, true
	}
	switch msg.String() {
	case "up":
		p.selected = max(0, p.selected-1)
		return nil, true
	case keyDown:
		p.selected = min(4, p.selected+1)
		return nil, true
	case "e", keyEnter:
		if p.selected == 4 {
			p.stage(fmt.Sprint(!p.draft.NativeToolCalling))
			return nil, true
		}
		if p.selected == 0 && len(p.state.Catalog) > 0 {
			p.chooser = providerChoice
			p.choice = 0
			return nil, true
		}
		p.fieldOriginal = p.values()[p.selected]
		p.field.SetValue(p.fieldOriginal)
		p.field.Focus()
		p.editing = true
		return nil, true
	case "m":
		p.chooser = CmdModel
		p.choice = 0
		return nil, true
	case "p":
		return p.perform(providerProbe), true
	case "s":
		if err := ValidateProviderDraft(p.draft); err != nil {
			p.status = err.Error()
		} else {
			p.review = providerSave
		}
		return nil, true
	case "a":
		switch {
		case p.activator == nil:
			p.status = "Activation is not integrated here; saved editor preferences do not change the active workspace configuration"
		case p.dirty():
			p.status = "Save the local draft before reviewing activation"
		default:
			p.review = providerActivate
		}
		return nil, true
	case "x":
		p.review = providerDiscard
		return nil, true
	}
	return nil, false
}
func (p *ProviderPane) perform(operation string) tea.Cmd {
	if operation == providerDiscard {
		p.draft = p.state.Saved
		p.edit++
		p.editing = false
		p.probe = nil
		p.status = "Local draft discarded"
		return nil
	}
	if p.port == nil {
		p.status = "Provider editor service unavailable"
		return nil
	}
	if err := ValidateProviderDraft(p.draft); err != nil {
		p.status = err.Error()
		return nil
	}
	if p.working {
		p.status = "Wait for the pending provider operation"
		return nil
	}
	p.request++
	p.operation = operation
	p.working = true
	request, edit, parent, port, activator, profile, state := p.request, p.edit, p.parent, p.port, p.activator, p.draft, p.state
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		result := providerResultMsg{owner: p, Request: request, Edit: edit, Operation: operation}
		switch operation {
		case providerProbe:
			result.Probe, result.Err = port.ProbeProviderSettings(ctx, profile)
		case providerSave:
			result.State, result.Err = port.SaveProviderSettings(ctx, ProviderSaveRequest{Workspace: state.Workspace, ExpectedRevision: state.Revision, Profile: profile})
		case providerActivate:
			result.State, result.Err = activator.ActivateProviderSettings(ctx, state.Revision)
		}
		return result
	}
}
func (p *ProviderPane) view() string {
	lines := []string{"Settings / Provider", ""}
	switch {
	case p.review != "":
		lines = append(lines, "Review "+p.review, "", "Workspace: "+terminalText(p.state.Workspace), "Provider: "+terminalText(p.draft.Provider), "Model: "+terminalText(p.draft.Model), "Endpoint: "+terminalText(p.draft.Endpoint), "Timeout: "+terminalText(p.draft.Timeout), fmt.Sprint("Native tool calling: ", p.draft.NativeToolCalling), "Active: "+terminalText(p.state.ActiveSummary), "Saved editor revision: "+terminalText(p.state.Revision), "", "Enter confirms · Escape cancels")
		if p.demo {
			lines = append(lines, "DEMO: changes scenario memory only.")
		}
	case p.editing:
		lines = append(lines, "Edit "+providerFieldLabels()[p.selected], "", p.field.View(), "", "Enter stages this value · Escape cancels")
	case p.chooser != "":
		lines = append(lines, "Choose "+p.chooser, "")
		choices := p.options()
		budget := max(1, p.height-8)
		start := max(0, p.choice-budget+1)
		for i := start; i < min(len(choices), start+budget); i++ {
			marker := " "
			if i == p.choice {
				marker = ">"
			}
			lines = append(lines, marker+" "+terminalText(choices[i]))
		}
		if len(choices) == 0 {
			lines = append(lines, "No observations available. Probe the current draft explicitly.")
		}
		lines = append(lines, "", "↑/↓ select · Enter stages · Escape cancels")
	default:
		for i, label := range providerFieldLabels() {
			marker := " "
			if i == p.selected {
				marker = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %-20s %s", marker, label, terminalText(p.values()[i])))
		}
		state := "saved preferences"
		if p.dirty() {
			state = "local draft"
		}
		lines = append(lines, "", state+" · active: "+terminalText(p.state.ActiveSummary), "", "↑/↓ field · Enter edit · m models · p probe · s review save · x discard")
		if p.activator != nil {
			lines = append(lines, "a reviews activation of the saved revision")
		}
	}
	if p.working {
		lines = append(lines, "", "Waiting for "+p.operation+" result…")
	}
	if p.status != "" {
		lines = append(lines, "", ansi.Hardwrap(ansi.Wordwrap(terminalText(p.status), max(1, p.width), ""), max(1, p.width), true))
	}
	return boundedScreen(strings.Join(lines, "\n"), max(1, p.width), max(1, p.height))
}
