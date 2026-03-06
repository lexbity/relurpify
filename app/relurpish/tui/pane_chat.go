package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
)

// chatSystemMsg is an internal message to add a system line to the chat feed.
type chatSystemMsg struct{ text string }

// ChatPane owns the message feed, run management, and HITL subscription.
// It is always held by pointer so mutations survive tea.Model value copies.
type ChatPane struct {
	feed      *Feed
	spinner   spinner.Model
	runStates map[string]*RunState

	context    *AgentContext
	session    *Session
	notifQ     *NotificationQueue
	hitlCh     <-chan fruntime.HITLEvent
	hitlOff    func()
	hitlSvc    hitlService
	runtime    RuntimeAdapter
	lastPrompt string

	allowParallel bool
	expandTarget  string

	width, height int
}

// NewChatPane initializes the ChatPane. The feed is created but not sized yet.
func NewChatPane(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) *ChatPane {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	var hitlCh <-chan fruntime.HITLEvent
	var hitlOff func()
	if rt != nil {
		hitlCh, hitlOff = rt.SubscribeHITL()
	}

	svc := hitlService(rt)

	return &ChatPane{
		feed:         NewFeed(),
		spinner:      sp,
		runStates:    make(map[string]*RunState),
		context:      ctx,
		session:      sess,
		notifQ:       notifQ,
		hitlCh:       hitlCh,
		hitlOff:      hitlOff,
		hitlSvc:      svc,
		runtime:      rt,
		expandTarget: "thinking",
	}
}

// Init starts the HITL listener and spinner.
func (p *ChatPane) Init() tea.Cmd {
	return tea.Batch(p.spinner.Tick, listenHITLEvents(p.hitlCh))
}

// Cleanup cancels all active runs and stops the HITL subscription.
func (p *ChatPane) Cleanup() {
	if p.hitlOff != nil {
		p.hitlOff()
	}
	for _, run := range p.runStates {
		if run.Cancel != nil {
			run.Cancel()
		}
	}
}

// SetSize resizes the pane.
func (p *ChatPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.feed.SetSize(w, h)
}

// HasActiveRuns returns true if any run is in flight.
func (p *ChatPane) HasActiveRuns() bool {
	return len(p.runStates) > 0
}

// Update dispatches tea messages relevant to the chat pane.
func (p *ChatPane) Update(msg tea.Msg) (*ChatPane, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		p.spinner, _ = p.spinner.Update(msg)
		p.feed.SetSpinner(p.spinner.View())
		return p, p.spinner.Tick

	case StreamTokenMsg:
		return p.handleStreamToken(msg)

	case StreamCompleteMsg:
		return p.handleStreamComplete(msg)

	case StreamErrorMsg:
		return p.handleStreamError(msg)

	case UpdateTaskMsg:
		return p.handleUpdateTask(msg)

	case hitlEventMsg:
		return p.handleHITLEvent(msg)

	case hitlResolvedMsg:
		return p.handleHITLResolved(msg)

	case chatSystemMsg:
		p.addSystemMessage(msg.text)
		return p, nil

	case tea.MouseMsg:
		f, cmd := p.feed.Update(msg)
		p.feed = f
		return p, cmd
	}
	return p, nil
}

// View renders the feed (input bar is rendered by root).
func (p *ChatPane) View() string {
	return p.feed.View()
}

// HandleInputSubmit processes text submitted from the input bar.
func (p *ChatPane) HandleInputSubmit(value string) tea.Cmd {
	cmd, _ := p.StartRun(value)
	return cmd
}

// StartRun begins an agent run. Returns the Bubble Tea command and the run ID.
// The run ID is empty when the run could not be started.
func (p *ChatPane) StartRun(prompt string) (tea.Cmd, string) {
	return p.StartRunWithMetadata(prompt, nil)
}

// StartRunWithMetadata begins an agent run with additional task metadata.
func (p *ChatPane) StartRunWithMetadata(prompt string, extra map[string]any) (tea.Cmd, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, ""
	}
	if p.runtime == nil {
		p.addSystemMessage("Runtime unavailable: cannot start run")
		return nil, ""
	}
	if !p.allowParallel && p.HasActiveRuns() {
		p.addSystemMessage("Run already in progress. Use /stop to cancel.")
		return nil, ""
	}

	userMsg := Message{
		ID:        generateID(),
		Timestamp: time.Now(),
		Role:      RoleUser,
		Content:   MessageContent{Text: prompt},
	}
	p.feed.AppendMessage(userMsg)

	runID := generateID()
	ch := make(chan tea.Msg, 256)
	builder := NewMessageBuilder(runID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	run := &RunState{
		ID:      runID,
		Prompt:  prompt,
		Started: time.Now(),
		Builder: builder,
		Ch:      ch,
		Cancel:  cancel,
	}
	p.runStates[runID] = run
	p.lastPrompt = prompt

	metadata := p.buildMetadata(ctx)
	for k, v := range extra {
		metadata[k] = v
	}
	go p.runStream(ctx, run, metadata)
	return listenToStream(ch), runID
}

// AddFile adds a file to the agent context.
func (p *ChatPane) AddFile(path string) tea.Cmd {
	if err := p.context.AddFile(path); err != nil {
		return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Context error: %v", err)} }
	}
	entry, err := fileEntryForPath(p.session.Workspace, path)
	if err != nil {
		return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Added to context: %s", path)} }
	}
	return func() tea.Msg {
		return chatSystemMsg{text: fmt.Sprintf("Added to context: %s (%s)", path, formatSizeToken(entry.SizeBytes, entry.TokenEstimate))}
	}
}

// StopLatestRun cancels the most recently started run.
func (p *ChatPane) StopLatestRun() tea.Cmd {
	if len(p.runStates) == 0 {
		return func() tea.Msg { return chatSystemMsg{text: "No active run to stop."} }
	}
	var latest *RunState
	for _, run := range p.runStates {
		if latest == nil || run.Started.After(latest.Started) {
			latest = run
		}
	}
	if latest == nil || latest.Cancel == nil {
		return func() tea.Msg { return chatSystemMsg{text: "No active run to stop."} }
	}
	latest.Cancel()
	return func() tea.Msg { return chatSystemMsg{text: fmt.Sprintf("Stopping run %s", latest.ID)} }
}

// RetryLastRun restarts the most recent prompt.
func (p *ChatPane) RetryLastRun() tea.Cmd {
	if strings.TrimSpace(p.lastPrompt) == "" {
		return func() tea.Msg { return chatSystemMsg{text: "No prior prompt to retry."} }
	}
	cmd, _ := p.StartRun(p.lastPrompt)
	return cmd
}

// ApplyPendingChanges bulk-approves or -rejects pending file changes.
func (p *ChatPane) ApplyPendingChanges(status ChangeStatus) int {
	messages := p.feed.messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := &messages[i]
		if msg.Role != RoleAgent || len(msg.Content.Changes) == 0 {
			continue
		}
		count := 0
		for j := range msg.Content.Changes {
			c := &msg.Content.Changes[j]
			if c.Status == StatusPending {
				c.Status = status
				count++
			}
		}
		if count > 0 {
			p.feed.refresh()
			return count
		}
	}
	return 0
}

// ToggleExpand toggles the current expand section in the last agent message.
func (p *ChatPane) ToggleExpand() {
	messages := p.feed.messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := &messages[i]
		if msg.Role != RoleAgent {
			continue
		}
		if msg.Content.Expanded == nil {
			msg.Content.Expanded = map[string]bool{}
		}
		sec := p.expandTarget
		if sec == "" {
			sec = "thinking"
		}
		msg.Content.Expanded[sec] = !msg.Content.Expanded[sec]
		break
	}
	p.feed.refresh()
}

// CycleExpandTarget advances to the next expand target.
func (p *ChatPane) CycleExpandTarget() {
	sections := []string{"thinking", "plan", "changes"}
	if p.expandTarget == "" {
		p.expandTarget = sections[0]
		return
	}
	for i, s := range sections {
		if s == p.expandTarget {
			p.expandTarget = sections[(i+1)%len(sections)]
			return
		}
	}
}

func (p *ChatPane) addSystemMessage(text string) {
	msg := Message{
		ID:        generateID(),
		Timestamp: time.Now(),
		Role:      RoleSystem,
		Content:   MessageContent{Text: text},
	}
	p.feed.AppendMessage(msg)
}

func (p *ChatPane) buildMetadata(ctx context.Context) map[string]any {
	meta := map[string]any{"source": "relurpish"}
	if p.context != nil && p.runtime != nil {
		files := p.context.List()
		if len(files) > 0 {
			res := p.runtime.ResolveContextFiles(ctx, files)
			if len(res.Allowed) > 0 {
				meta["context_files"] = res.Allowed
				if len(res.Contents) > 0 {
					meta["context_file_contents"] = res.Contents
				}
			}
		}
	}
	if p.session != nil {
		if p.session.Mode != "" {
			meta["mode"] = p.session.Mode
		}
		if p.session.Strategy != "" {
			meta["strategy"] = p.session.Strategy
		}
	}
	return meta
}

func (p *ChatPane) runStream(ctx context.Context, run *RunState, metadata map[string]any) {
	start := time.Now()
	sendRunMsg(run, StreamTokenMsg{
		RunID:     run.ID,
		TokenType: TokenThinking,
		Metadata: map[string]interface{}{
			"kind":        "start",
			"stepType":    string(StepAnalyzing),
			"description": "Analyzing request",
		},
	})

	callback := func(token string) {
		sendRunMsg(run, StreamTokenMsg{RunID: run.ID, TokenType: TokenText, Token: token})
	}

	result, err := p.runtime.ExecuteInstructionStream(ctx, run.Prompt, core.TaskTypeCodeGeneration, metadata, callback)
	if err != nil {
		sendRunFinal(run, StreamErrorMsg{RunID: run.ID, Error: err})
		sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: 0})
		close(run.Ch)
		return
	}

	summary := summarizeResult(result)
	if summary != "" {
		sendRunMsg(run, StreamTokenMsg{RunID: run.ID, TokenType: TokenText, Token: summary})
	}
	sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: estimateTokens(summary)})
	close(run.Ch)
}

func (p *ChatPane) handleStreamToken(msg StreamTokenMsg) (*ChatPane, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	run.Builder.AddToken(msg)
	partial := run.Builder.BuildPartial()
	p.feed.UpdateMessage(partial)
	return p, listenToStream(run.Ch)
}

func (p *ChatPane) handleStreamComplete(msg StreamCompleteMsg) (*ChatPane, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	final := run.Builder.Build(msg.Duration, msg.TokensUsed)
	p.feed.UpdateMessage(final)
	if p.session != nil {
		p.session.TotalTokens += msg.TokensUsed
		p.session.TotalDuration += msg.Duration
	}
	if dropped := atomic.LoadInt64(&run.Dropped); dropped > 0 {
		p.addSystemMessage(fmt.Sprintf("Stream backpressure: dropped %d update(s)", dropped))
	}
	delete(p.runStates, msg.RunID)
	return p, streamCompletedCmd(msg)
}

func (p *ChatPane) handleStreamError(msg StreamErrorMsg) (*ChatPane, tea.Cmd) {
	delete(p.runStates, msg.RunID)
	if msg.Error != nil && errors.Is(msg.Error, context.Canceled) {
		p.addSystemMessage(fmt.Sprintf("Run %s canceled", msg.RunID))
	} else {
		p.addSystemMessage(fmt.Sprintf("Agent error: %v", msg.Error))
	}
	return p, nil
}

func (p *ChatPane) handleUpdateTask(msg UpdateTaskMsg) (*ChatPane, tea.Cmd) {
	messages := p.feed.messages
	for i := len(messages) - 1; i >= 0; i-- {
		content := &messages[i].Content
		if content.Plan == nil {
			continue
		}
		if msg.TaskIndex >= 0 && msg.TaskIndex < len(content.Plan.Tasks) {
			t := &content.Plan.Tasks[msg.TaskIndex]
			t.Status = msg.Status
			switch msg.Status {
			case TaskInProgress:
				t.StartTime = time.Now()
			case TaskCompleted:
				t.EndTime = time.Now()
			}
			break
		}
	}
	p.feed.refresh()
	return p, nil
}

func (p *ChatPane) handleHITLEvent(msg hitlEventMsg) (*ChatPane, tea.Cmd) {
	next := listenHITLEvents(p.hitlCh)
	var pending []*fruntime.PermissionRequest
	if p.hitlSvc != nil {
		pending = p.hitlSvc.PendingHITL()
	}
	switch msg.event.Type {
	case fruntime.HITLEventRequested:
		req := msg.event.Request
		if req == nil && len(pending) > 0 {
			req = pending[0]
		}
		if req != nil && p.notifQ != nil {
			p.notifQ.PushHITL(req)
		}
	case fruntime.HITLEventResolved, fruntime.HITLEventExpired:
		if msg.event.Request != nil && p.notifQ != nil {
			p.notifQ.Resolve(msg.event.Request.ID)
		}
		if msg.event.Type == fruntime.HITLEventExpired && msg.event.Request != nil {
			reason := msg.event.Error
			if reason == "" {
				reason = "expired"
			}
			p.addSystemMessage(fmt.Sprintf("Permission %s expired: %s", msg.event.Request.ID, reason))
		}
	}
	return p, next
}

func (p *ChatPane) handleHITLResolved(msg hitlResolvedMsg) (*ChatPane, tea.Cmd) {
	if p.notifQ != nil {
		p.notifQ.Resolve(msg.requestID)
	}
	if msg.err != nil {
		p.addSystemMessage(fmt.Sprintf("HITL %s failed: %v", msg.requestID, msg.err))
	} else if msg.approved {
		p.addSystemMessage(fmt.Sprintf("Approved %s", msg.requestID))
	} else {
		p.addSystemMessage(fmt.Sprintf("Denied %s", msg.requestID))
	}
	return p, listenHITLEvents(p.hitlCh)
}

// streamCompletedCmd is a fire-and-forget used by the root model to trigger auto-save.
type streamDoneMsg struct{ RunID string }

func streamCompletedCmd(msg StreamCompleteMsg) tea.Cmd {
	return func() tea.Msg { return streamDoneMsg{RunID: msg.RunID} }
}

// summarizeResult turns a core.Result into human-readable text.
func summarizeResult(res *core.Result) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Task node: ")
	b.WriteString(res.NodeID)
	b.WriteString("\nSuccess: ")
	if res.Success {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	if len(res.Data) > 0 {
		b.WriteString("\nData: ")
		b.WriteString(fmt.Sprintf("%v", res.Data))
	}
	if res.Error != nil {
		b.WriteString("\nError: ")
		b.WriteString(res.Error.Error())
	}
	return b.String()
}
