package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/telemetry"

	tea "github.com/charmbracelet/bubbletea"
)

// ConversationSession supplies application outcomes, not views or key handlers.
// The explicit demo provider implements this port without an execution adapter.
type ConversationSession interface {
	Current() FrontendSnapshot
	Next(context.Context) (FrontendSnapshot, error)
	Submit(context.Context, string) (FrontendSnapshot, error)
	Cancel(context.Context) (FrontendSnapshot, error)
	Answer(context.Context, string, string, string) (FrontendSnapshot, error)
}

type ConversationLine struct{ Role, Text string }
type DecisionChoice struct{ ID, Label string }
type FrontendDecision struct {
	ID, Kind, Question string
	Choices            []DecisionChoice
	AllowText          bool
}
type FrontendSnapshot struct {
	Revision uint64
	Messages []ConversationLine
	Circuit  CircuitSnapshot
	Decision *FrontendDecision
	Running  bool
	Status   string
	// Observations are explicit demo fixtures, never actual execution evidence.
	// Live observations arrive exclusively through the runtime's OTel pipeline.
	Observations []telemetry.Event
}

// HostOptions is the sole composition contract for the shared host. Demo ports
// cannot be combined with a live runtime or a factory that creates live panels.
type HostOptions struct {
	Runtime        RuntimeAdapter
	SurfaceFactory SurfaceFactory
	DemoScenario   string
	Conversation   ConversationSession
	Doctor         DoctorReader
	Initializer    DoctorOperations
	Recipes        RecipeWorkspace
	Workspace      string
	Configuration  ConfigurationReader
	Services       ServicePort
	Providers      ProviderEditorPort
}

type frontendSnapshotMsg struct {
	Snapshot FrontendSnapshot
	Err      error
	Observed bool
}
type frontendSubmitMsg struct {
	Draft    string
	Snapshot FrontendSnapshot
	Err      error
}

func (m *RootModel) applyFrontend(snapshot FrontendSnapshot) {
	if snapshot.Revision < m.frontendState.Revision {
		return
	}
	previous := ""
	if m.frontendState.Decision != nil {
		previous = m.frontendState.Decision.ID
	}
	m.frontendState = snapshot
	if m.demoScenario != "" {
		for _, event := range snapshot.Observations {
			if event.Seq > m.demoObserved {
				m.activity.apply([]telemetry.Event{event})
				m.demoObserved = event.Seq
			}
		}
	}
	if snapshot.Decision != nil {
		m.decisionChoice = max(0, min(m.decisionChoice, len(snapshot.Decision.Choices)-1))
	}
	m.circuit.apply(snapshot.Circuit)
	if snapshot.Decision == nil || snapshot.Decision.ID != previous {
		m.decisionChoice, m.decisionAnswer, m.decisionEditing = 0, "", false
	}
	// A new attention item never changes focus. An expired item simply closes
	// its existing detail; it never grants/denies an application decision.
	if snapshot.Decision == nil {
		m.decisionFocused = false
	}
}

func (m RootModel) nextFrontendCmd() tea.Cmd {
	if m.conversation == nil {
		return nil
	}
	port, parent := m.conversation, m.parentCtx
	return func() tea.Msg {
		snapshot, err := port.Next(parent)
		return frontendSnapshotMsg{Snapshot: snapshot, Err: err, Observed: true}
	}
}

func (m RootModel) submitFrontend(text string) (tea.Model, tea.Cmd) {
	if m.frontendPending || m.frontendState.Running || m.startupLocked {
		m.addSystemMessage("Request kept as a draft while execution is unavailable or busy.")
		return m, nil
	}
	m.frontendPending = true
	port, parent := m.conversation, m.parentCtx
	return m, func() tea.Msg {
		snapshot, err := port.Submit(parent, text)
		return frontendSubmitMsg{Draft: text, Snapshot: snapshot, Err: err}
	}
}

func (m RootModel) cancelFrontendCmd() tea.Cmd {
	port, parent := m.conversation, m.parentCtx
	return func() tea.Msg {
		snapshot, err := port.Cancel(parent)
		return frontendSnapshotMsg{Snapshot: snapshot, Err: err}
	}
}

func (m *RootModel) frontendDecisionKey(msg tea.KeyMsg) tea.Cmd {
	decision := m.frontendState.Decision
	if decision == nil {
		m.decisionFocused = false
		return nil
	}
	if m.decisionEditing {
		switch msg.String() {
		case "esc":
			m.decisionEditing = false
			return nil
		case "backspace":
			runes := []rune(m.decisionAnswer)
			if len(runes) > 0 {
				m.decisionAnswer = string(runes[:len(runes)-1])
			}
			return nil
		case "enter":
			if strings.TrimSpace(m.decisionAnswer) == "" {
				return nil
			}
		default:
			if msg.Type == tea.KeyRunes && len(m.decisionAnswer)+len(string(msg.Runes)) <= 65536 {
				m.decisionAnswer += string(msg.Runes)
			}
			if msg.Type == tea.KeySpace {
				m.decisionAnswer += " "
			}
			return nil
		}
	} else {
		switch msg.String() {
		case "esc":
			m.decisionFocused = false
			return nil
		case "up":
			m.decisionChoice = max(0, m.decisionChoice-1)
			return nil
		case "down":
			m.decisionChoice = min(len(decision.Choices)-1, m.decisionChoice+1)
			return nil
		case "tab":
			if decision.AllowText {
				m.decisionEditing = true
			}
			return nil
		case "enter":
			if len(decision.Choices) == 0 {
				m.decisionEditing = decision.AllowText
				return nil
			}
		default:
			return nil
		}
	}
	choice, answer := "", m.decisionAnswer
	if !m.decisionEditing && len(decision.Choices) > 0 {
		choice = decision.Choices[m.decisionChoice].ID
	}
	port, parent, id := m.conversation, m.parentCtx, decision.ID
	m.decisionFocused, m.decisionEditing = false, false
	return func() tea.Msg {
		snapshot, err := port.Answer(parent, id, choice, answer)
		return frontendSnapshotMsg{Snapshot: snapshot, Err: err}
	}
}

func (m RootModel) frontendDecisionView() string {
	decision := m.frontendState.Decision
	if decision == nil {
		return "No pending decision."
	}
	lines := []string{"Attention / " + terminalText(decision.Kind), "", terminalText(decision.Question), ""}
	for i, choice := range decision.Choices {
		marker := " "
		if i == m.decisionChoice && !m.decisionEditing {
			marker = ">"
		}
		lines = append(lines, marker+" "+terminalText(choice.Label))
	}
	if m.decisionEditing {
		lines = append(lines, "", "Your answer (Enter sends):", terminalText(m.decisionAnswer))
	}
	lines = append(lines, "", "↑/↓ choose · Enter responds · Escape returns to request")
	if decision.AllowText {
		lines = append(lines, "Tab opens a separate answer field")
	}
	return strings.Join(lines, "\n")
}

func (m RootModel) handleFrontendSnapshot(msg frontendSnapshotMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if msg.Snapshot.Revision > 0 {
			m.applyFrontend(msg.Snapshot)
		}
		if !errors.Is(msg.Err, context.Canceled) {
			m.notice = fmt.Sprintf("Session operation failed: %v", msg.Err)
		}
		return m, nil
	}
	m.applyFrontend(msg.Snapshot)
	return m, nil
}
