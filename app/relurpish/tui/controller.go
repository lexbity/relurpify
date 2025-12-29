package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/runtime"
)

// Run bootstraps the new agentic TUI experience.
func Run(ctx context.Context, rt *runtimesvc.Runtime) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	model := NewModel(newRuntimeAdapter(rt))
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	final, err := program.Run()
	if m, ok := final.(Model); ok {
		m.cleanup()
	}
	return err
}

// Model implements the Bubble Tea Model interface and coordinates the feed,
// prompt bar, and status bar components described in the new UX spec.
type Model struct {
	runtime RuntimeAdapter

	hitl    hitlService
	hitlCh  <-chan runtime.HITLEvent
	hitlOff func()

	feed    *viewport.Model
	input   textinput.Model
	spinner spinner.Model

	statusBar StatusBar

	messages []Message
	context  *AgentContext
	session  *Session

	width  int
	height int
	ready  bool

	mode InputMode

	expandTarget  string
	autoFollow    bool
	allowParallel bool
	lastPrompt    string
	runStates     map[string]*RunState

	filePicker     filePickerState
	commandPalette commandPaletteState

	// HITL prompt state (temporarily replaces normal prompt)
	hitlRequest        *runtime.PermissionRequest
	hitlPreviousMode   InputMode
	hitlPreviousValue  string
	hitlPreviousPrompt string
}

// NewModel initializes the prompt/input/feed model with defaults from runtime.
func NewModel(adapter RuntimeAdapter) Model {
	hitlSvc := hitlService(adapter)
	var hitlCh <-chan runtime.HITLEvent
	var hitlOff func()
	if hitlSvc != nil {
		hitlCh, hitlOff = hitlSvc.SubscribeHITL()
	}
	input := textinput.New()
	input.Placeholder = "Type a message or /help for commands"
	input.Focus()

	v := viewport.New(0, 0)
	vp := &v

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	info := SessionInfo{MaxTokens: 100000}
	if adapter != nil {
		info = adapter.SessionInfo()
	}
	session := &Session{
		ID:        fmt.Sprintf("session-%d", time.Now().UnixNano()),
		StartTime: time.Now(),
		Workspace: info.Workspace,
		Model:     info.Model,
		Agent:     info.Agent,
		Mode:      info.Mode,
	}

	status := StatusBar{
		workspace:  session.Workspace,
		model:      session.Model,
		agent:      session.Agent,
		mode:       session.Mode,
		tokens:     session.TotalTokens,
		duration:   session.TotalDuration,
		lastUpdate: time.Now(),
	}

	ctx := &AgentContext{
		Files:     []string{},
		MaxTokens: info.MaxTokens,
	}

	return Model{
		runtime:       adapter,
		hitl:          hitlSvc,
		hitlCh:        hitlCh,
		hitlOff:       hitlOff,
		feed:          vp,
		input:         input,
		spinner:       sp,
		statusBar:     status,
		messages:      []Message{},
		context:       ctx,
		session:       session,
		mode:          ModeNormal,
		autoFollow:    true,
		expandTarget:  "thinking",
		allowParallel: false,
		runStates:     make(map[string]*RunState),
		commandPalette: commandPaletteState{
			items:    nil,
			selected: 0,
		},
	}
}

func (m Model) setMessages(messages []Message) Model {
	m.messages = messages
	return m.refreshFeedContent()
}

func (m Model) refreshFeedContent() Model {
	if !m.ready || m.feed == nil {
		return m
	}
	m.feed.SetContent(m.renderMessages())
	if m.autoFollow {
		m.feed.GotoBottom()
	}
	return m
}

func (m Model) adjustLayout() Model {
	if m.width == 0 || m.height == 0 || m.feed == nil {
		return m
	}
	statusBarHeight := 1
	promptBarHeight := 1
	panelHeight := m.panelHeight()
	feedHeight := max(1, m.height-statusBarHeight-promptBarHeight-panelHeight)
	m.feed.Width = m.width
	m.feed.Height = feedHeight
	return m
}

func (m Model) enterHITL(req *runtime.PermissionRequest) Model {
	if req == nil {
		return m
	}
	if m.mode != ModeHITL {
		m.hitlPreviousMode = m.mode
		m.hitlPreviousValue = m.input.Value()
		m.hitlPreviousPrompt = m.input.Placeholder
	}
	m.hitlRequest = req
	m.mode = ModeHITL
	m.input.SetValue("")
	m.input.Placeholder = ""
	m.input.Focus()
	return m
}

func (m Model) exitHITL() Model {
	if m.mode != ModeHITL {
		return m
	}
	m.hitlRequest = nil
	m.mode = m.hitlPreviousMode
	m.input.Placeholder = m.hitlPreviousPrompt
	m.input.SetValue(m.hitlPreviousValue)
	m.input.Focus()
	return m
}

// submitPrompt orchestrates sending the current input to the agent runtime.
func (m Model) submitPrompt() (Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, nil
	}

	userMsg := Message{
		ID:        generateID(),
		Timestamp: time.Now(),
		Role:      RoleUser,
		Content: MessageContent{
			Text: value,
		},
	}
	m.messages = append(m.messages, userMsg)
	m = m.refreshFeedContent()

	m.input.SetValue("")
	m.mode = ModeNormal

	return m.startRun(value)
}

func (m Model) cleanup() {
	if m.hitlOff != nil {
		m.hitlOff()
	}
	for _, run := range m.runStates {
		if run.Cancel != nil {
			run.Cancel()
		}
	}
}

func (m Model) cleanupCmd() tea.Cmd {
	return func() tea.Msg {
		m.cleanup()
		return nil
	}
}
