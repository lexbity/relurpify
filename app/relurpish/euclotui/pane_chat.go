package euclotui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	execution "codeburg.org/lexbit/relurpify/execution"
	capability "codeburg.org/lexbit/relurpify/framework/capability"
)

const chatSidebarMinWidth = 60
const contextFileMaxBytes = 8000

var spinnerFrames = []string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"}

var chatSubTabPolicies = map[tui.SubTabID]struct {
	ModeHint           string
	EditEnabled        bool
	OnlineToolsEnabled bool
}{
	tui.SubTabChatLocalRead:  {ModeHint: "review"},
	tui.SubTabChatLocalEdit:  {ModeHint: "code", EditEnabled: true},
	tui.SubTabChatOnlineRead: {ModeHint: "research", OnlineToolsEnabled: true},
	tui.SubTabChatOnlineEdit: {ModeHint: "code", EditEnabled: true, OnlineToolsEnabled: true},
}

type chatFocusPane int

const (
	chatFocusFeed chatFocusPane = iota
	chatFocusSidebar
)

type ChatPane struct {
	feed      *tui.Feed
	spinner   spinner.Model
	runStates map[string]*tui.RunState
	th        *theme.Theme
	anim      *tui.AnimationManager

	context *tui.AgentContext
	session *tui.Session
	store   *tui.SessionStore
	notifQ  *tui.NotificationQueue
	hitlSvc tui.HITLServiceIface
	runtime tui.RuntimeAdapter
	router  *EucloEventRouter
	diff    *DiffPane

	lastPrompt    string
	allowParallel bool
	expandTarget  string
	activeSubTab  tui.SubTabID
	activeTab     tui.TabID

	width, height int

	undoStack [][]tui.Message
	redoStack [][]tui.Message

	compactRunID    string
	compactMsgCount int

	spinnerAnimID tui.AnimationID
	spinnerIdx    int

	showSidebar        bool
	sidebarFocused     bool
	sidebarEntries     []tui.ContextSidebarEntry
	sidebarSelection   int
	workspaceSelection []string
	selectionEnv       *contextdata.Envelope
	astOption          string
}

var _ tui.ChatPaner = (*ChatPane)(nil)
var _ tui.ChatSidebarController = (*ChatPane)(nil)

// NewChatPane constructs the Euclo chat surface.
func NewChatPane(rt tui.RuntimeAdapter, ctx *tui.AgentContext, sess *tui.Session, notifQ *tui.NotificationQueue, router *EucloEventRouter, th *theme.Theme, anim *tui.AnimationManager) *ChatPane {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	if router == nil {
		router = NewEucloEventRouter()
	}
	if th == nil {
		th = theme.Default()
	}
	pane := &ChatPane{
		feed:             tui.NewFeed(),
		spinner:          sp,
		th:               th,
		anim:             anim,
		runStates:        make(map[string]*tui.RunState),
		context:          ctx,
		session:          sess,
		store:            nil,
		notifQ:           notifQ,
		hitlSvc:          tui.HITLServiceIface(rt),
		runtime:          rt,
		router:           router,
		expandTarget:     "thinking",
		astOption:        "workspace",
		diff:             NewDiffPane(router, sessionWorkspace(sess), th),
		showSidebar:      false,
		sidebarFocused:   false,
		sidebarEntries:   nil,
		sidebarSelection: 0,
	}
	if rt != nil {
		pane.diff.SetRuntime(rt)
	}
	pane.feed.SetTheme(th)
	return pane
}

func (p *ChatPane) Init() tea.Cmd {
	if p.HasActiveRuns() {
		return p.spinner.Tick
	}
	return nil
}

func (p *ChatPane) Cleanup() {
	for _, run := range p.runStates {
		if run.Cancel != nil {
			run.Cancel()
		}
	}
}

func (p *ChatPane) SetSubTab(id tui.SubTabID)  { p.activeSubTab = id }
func (p *ChatPane) ActiveSubTab() tui.SubTabID { return p.activeSubTab }
func (p *ChatPane) SetSessionStore(store *tui.SessionStore) {
	p.store = store
	if p.diff != nil {
		p.diff.SetSessionStore(store)
	}
}
func (p *ChatPane) SetActiveTab(id tui.TabID) {
	p.activeTab = id
	if p.diff != nil {
		p.diff.SetRouter(p.router)
		p.diff.SetWorkspace(sessionWorkspace(p.session))
	}
}

func (p *ChatPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	feedWidth := w
	if p.splitSidebarVisible() {
		feedWidth = w - p.sidebarWidth(w) - 1
	}
	if feedWidth < 0 {
		feedWidth = 0
	}
	p.feed.SetSize(feedWidth, h)
	if p.diff != nil {
		p.diff.SetSize(w, h)
	}
}

func (p *ChatPane) Update(msg tea.Msg) (tui.ChatPaner, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		p.spinner, _ = p.spinner.Update(msg)
		p.feed.SetSpinner(p.spinner.View())
		if p.HasActiveRuns() {
			return p, p.spinner.Tick
		}
		return p, nil
	case tui.AnimationTickMsg:
		if !p.HasActiveRuns() {
			p.deregisterSpinnerAnim()
			return p, nil
		}
		p.spinnerIdx++
		frame := spinnerFrames[p.spinnerIdx%len(spinnerFrames)]
		p.feed.SetSpinner(frame)
		return p, nil
	case tui.StreamTokenMsg:
		return p.handleStreamToken(msg)
	case tui.StreamCompleteMsg:
		return p.handleStreamComplete(msg)
	case tui.StreamErrorMsg:
		return p.handleStreamError(msg)
	case tui.UpdateTaskMsg:
		return p.handleUpdateTask(msg)
	case tui.ChatSystemMsg:
		p.addSystemMessage(msg.Text)
		return p, nil
	case tea.KeyMsg:
		if p.activeTab == tui.TabDiff && p.diff != nil {
			if cmd := p.diff.Update(msg); cmd != nil {
				return p, cmd
			}
			return p, nil
		}
		return p.handleKey(msg)
	case tea.MouseMsg:
		f, cmd := p.feed.Update(msg)
		p.feed = f
		return p, cmd
	case tui.CompactResultMsg:
		// Ignore: the host handles compact outcomes.
		return p, nil
	}
	return p, nil
}

func (p *ChatPane) View() string {
	switch p.activeTab {
	case tui.TabDiff:
		if p.diff != nil {
			return p.diff.View()
		}
	}
	if p.width < 60 {
		return lipgloss.JoinVertical(lipgloss.Left,
			p.th.Dim().Render("Terminal too narrow. Minimum 60 columns required."),
			p.feed.View(),
		)
	}
	if !p.splitSidebarVisible() {
		if p.showSidebar && p.width < 90 {
			return lipgloss.JoinVertical(lipgloss.Left,
				p.th.Dim().Render("Sidebar collapsed automatically below 90 columns."),
				p.feed.View(),
			)
		}
		return p.feed.View()
	}
	feedView := p.feed.View()
	sidebarView := p.renderSidebar()
	return lipgloss.JoinHorizontal(lipgloss.Top, feedView, sidebarView)
}

func (p *ChatPane) registerSpinnerAnim() {
	if p.anim == nil || p.spinnerAnimID != 0 {
		return
	}
	id := p.anim.Register(func() tui.AnimationFrame {
		return tui.AnimationFrame{Text: "", Done: false}
	})
	p.spinnerAnimID = id
}

func (p *ChatPane) deregisterSpinnerAnim() {
	if p.anim == nil || p.spinnerAnimID == 0 {
		return
	}
	p.anim.Deregister(p.spinnerAnimID)
	p.spinnerAnimID = 0
	p.spinnerIdx = 0
}

func (p *ChatPane) HandleInputSubmit(value string) tea.Cmd {
	cleanedValue := value
	files := extractFileTokens(value)
	if len(files) > 0 && p.runtime != nil {
		resolution := p.runtime.ResolveContextFiles(context.Background(), files)
		if len(resolution.Allowed) > 0 && p.context != nil {
			p.context.Files = append(p.context.Files, resolution.Allowed...)
		}
		cleanedValue = removeFileTokens(value)
	}
	if len(p.feed.Messages()) > 0 {
		snapshot := make([]tui.Message, len(p.feed.Messages()))
		copy(snapshot, p.feed.Messages())
		p.undoStack = append(p.undoStack, snapshot)
		p.redoStack = nil
	}
	cmd, _ := p.StartRun(cleanedValue)
	return cmd
}

func (p *ChatPane) StartRun(prompt string) (tea.Cmd, string) {
	return p.StartRunWithMetadata(prompt, nil)
}

func (p *ChatPane) StartRunSilent(prompt string) (tea.Cmd, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, ""
	}
	if p.runtime == nil {
		p.addSystemMessage("Runtime unavailable: cannot start run")
		return nil, ""
	}
	if p.HasActiveRuns() {
		p.addSystemMessage("Run already in progress.")
		return nil, ""
	}
	runID := tui.GenerateID()
	ch := make(chan tea.Msg, 256)
	builder := tui.NewMessageBuilder(runID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	run := &tui.RunState{
		ID:      runID,
		Prompt:  prompt,
		Started: time.Now(),
		Builder: builder,
		Ch:      ch,
		Cancel:  cancel,
	}
	p.runStates[runID] = run
	p.registerSpinnerAnim()
	metadata := p.buildMetadata(ctx)
	metadata["compact"] = true
	go p.runStream(ctx, run, metadata)
	return tea.Batch(listenToStream(ch), p.spinner.Tick), runID
}

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
	userMsg := tui.Message{
		ID:        tui.GenerateID(),
		Timestamp: time.Now(),
		Role:      tui.RoleUser,
		Content:   tui.MessageContent{Text: prompt},
	}
	p.feed.AppendMessage(userMsg)

	runID := tui.GenerateID()
	ch := make(chan tea.Msg, 256)
	builder := tui.NewMessageBuilder(runID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	run := &tui.RunState{
		ID:      runID,
		Prompt:  prompt,
		Started: time.Now(),
		Builder: builder,
		Ch:      ch,
		Cancel:  cancel,
	}
	p.runStates[runID] = run
	p.lastPrompt = prompt
	p.registerSpinnerAnim()

	metadata := p.buildMetadata(ctx)
	for k, v := range extra {
		metadata[k] = v
	}
	go p.runStream(ctx, run, metadata)
	return tea.Batch(listenToStream(ch), p.spinner.Tick), runID
}

func (p *ChatPane) HasActiveRuns() bool { return len(p.runStates) > 0 }
func (p *ChatPane) Undo() bool          { return p.restoreSnapshot(&p.undoStack, &p.redoStack) }
func (p *ChatPane) Redo() bool          { return p.restoreSnapshot(&p.redoStack, &p.undoStack) }
func (p *ChatPane) ToggleCompact() {
	switch p.expandTarget {
	case "thinking":
		p.expandTarget = "plan"
	case "plan":
		p.expandTarget = "all"
	default:
		p.expandTarget = "thinking"
	}
	p.addSystemMessage(fmt.Sprintf("Display mode: %s", p.expandTarget))
}
func (p *ChatPane) ToggleSidebar() {
	p.showSidebar = !p.showSidebar
	if p.showSidebar && len(p.sidebarEntries) == 0 {
		p.updateSidebarContent()
	}
	p.SetSize(p.width, p.height)
}
func (p *ChatPane) AddFileToSidebar(path string) error {
	if p.context == nil {
		return fmt.Errorf("context unavailable")
	}
	if err := p.context.AddFile(path); err != nil {
		return err
	}
	p.addSelectedFile(path)
	p.updateSidebarContent()
	return nil
}
func (p *ChatPane) RemoveFileFromSidebar(path string) {
	if p.context != nil {
		p.context.RemoveFile(path)
	}
	p.removeSelectedFile(path)
	p.updateSidebarContent()
}
func (p *ChatPane) UpdateSidebarFromFrame(frame any) {
	switch frame := frame.(type) {
	case interaction.InteractionFrame:
		if payloadFiles, ok := frame.Payload["euclo.user_selected_files"]; ok {
			if files, ok := payloadFiles.([]string); ok {
				p.replaceSelectedFiles(files)
			}
		}
	}
}
func (p *ChatPane) AddSystemMessage(text string) { p.addSystemMessage(text) }
func (p *ChatPane) AppendMessage(msg tui.Message) {
	msg = p.normalizeMilestoneMessage(msg)
	if strings.TrimSpace(msg.Content.Text) == "" && msg.Role == tui.RoleAgent {
		return
	}
	p.feed.AppendMessage(msg)
}
func (p *ChatPane) ClearMessages()                { p.feed.ClearMessages() }
func (p *ChatPane) Messages() []tui.Message       { return p.feed.Messages() }
func (p *ChatPane) SetSearchFilter(filter string) { p.feed.SetSearchFilter(filter) }
func (p *ChatPane) ScrollUp()                     { p.feed.ScrollUp() }
func (p *ChatPane) PageDown()                     { p.feed.PageDown() }
func (p *ChatPane) PageUp()                       { p.feed.PageUp() }
func (p *ChatPane) RollbackLastUndo() {
	if len(p.undoStack) > 0 {
		p.undoStack = p.undoStack[:len(p.undoStack)-1]
	}
}
func (p *ChatPane) PushUndoSnapshot(msgs []tui.Message) {
	snapshot := make([]tui.Message, len(msgs))
	copy(snapshot, msgs)
	p.undoStack = append(p.undoStack, snapshot)
	p.redoStack = nil
}
func (p *ChatPane) HITLService() tui.HITLServiceIface { return p.hitlSvc }
func (p *ChatPane) AllowParallel() bool               { return p.allowParallel }
func (p *ChatPane) SetAllowParallel(v bool)           { p.allowParallel = v }
func (p *ChatPane) LastPrompt() string                { return p.lastPrompt }
func (p *ChatPane) SetCompactRunID(runID string, msgCount int) {
	p.compactRunID = runID
	p.compactMsgCount = msgCount
}
func (p *ChatPane) StopLatestRun() tea.Cmd {
	if len(p.runStates) == 0 {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: "No active run to stop."} }
	}
	var latest *tui.RunState
	for _, run := range p.runStates {
		if latest == nil || run.Started.After(latest.Started) {
			latest = run
		}
	}
	if latest == nil || latest.Cancel == nil {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: "No active run to stop."} }
	}
	latest.Cancel()
	return func() tea.Msg { return tui.ChatSystemMsg{Text: fmt.Sprintf("Stopping run %s", latest.ID)} }
}
func (p *ChatPane) RetryLastRun() tea.Cmd {
	if strings.TrimSpace(p.lastPrompt) == "" {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: "No prior prompt to retry."} }
	}
	cmd, _ := p.StartRun(p.lastPrompt)
	return cmd
}
func (p *ChatPane) ApplyPendingChanges(status tui.ChangeStatus) int {
	count := 0
	p.feed.Mutate(func(msgs []tui.Message) {
		for i := len(msgs) - 1; i >= 0; i-- {
			msg := &msgs[i]
			if msg.Role != tui.RoleAgent || len(msg.Content.Changes) == 0 {
				continue
			}
			for j := range msg.Content.Changes {
				c := &msg.Content.Changes[j]
				if c.Status == tui.StatusPending {
					c.Status = status
					count++
				}
			}
			if count > 0 {
				return
			}
		}
	})
	return count
}
func (p *ChatPane) MutateMessages(fn func(msgs []tui.Message)) { p.feed.Mutate(fn) }
func (p *ChatPane) AddFile(path string) tea.Cmd {
	if p.context == nil {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: fmt.Sprintf("Context error: context unavailable")} }
	}
	if err := p.context.AddFile(path); err != nil {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: fmt.Sprintf("Context error: %v", err)} }
	}
	p.addSelectedFile(path)
	p.updateSidebarContent()
	if p.runtime == nil {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: fmt.Sprintf("Added to context: %s", path)} }
	}
	resolution := p.runtime.ResolveContextFiles(context.Background(), []string{path})
	if len(resolution.Contents) == 0 {
		return func() tea.Msg { return tui.ChatSystemMsg{Text: fmt.Sprintf("Added to context: %s", path)} }
	}
	content := resolution.Contents[0].Content
	size := int64(len(content))
	if resolution.Contents[0].Truncated {
		size = contextFileMaxBytes
	}
	return func() tea.Msg {
		return tui.ChatSystemMsg{Text: fmt.Sprintf("Added to context: %s (%s)", path, tui.FormatSizeToken(size, tui.EstimateTokensFromBytes(size)))}
	}
}

func (p *ChatPane) handleKey(msg tea.KeyMsg) (tui.ChatPaner, tea.Cmd) {
	key := msg.String()
	if p.showSidebar && p.splitSidebarVisible() {
		switch key {
		case "ctrl+]":
			p.ToggleSidebar()
			return p, nil
		case "tab":
			p.sidebarFocused = !p.sidebarFocused
			return p, nil
		case "left":
			if p.sidebarFocused {
				p.ToggleSidebar()
			} else {
				p.sidebarFocused = true
			}
			return p, nil
		case "right":
			p.sidebarFocused = false
			return p, nil
		case "up", "k":
			if p.sidebarFocused {
				if p.sidebarSelection > 0 {
					p.sidebarSelection--
				}
				return p, nil
			}
			p.feed.PageUp()
			return p, nil
		case "down", "j":
			if p.sidebarFocused {
				if p.sidebarSelection < len(p.sidebarEntries)-1 {
					p.sidebarSelection++
				}
				return p, nil
			}
			p.feed.PageDown()
			return p, nil
		case "a":
			if p.sidebarFocused {
				if entry := p.selectedSidebarEntry(); entry.Path != "" {
					return p, p.AddFile(entry.Path)
				}
			}
		case "x":
			if p.sidebarFocused {
				if entry := p.selectedSidebarEntry(); entry.Path != "" {
					p.RemoveFileFromSidebar(entry.Path)
				}
				return p, nil
			}
		}
	}
	if key == "ctrl+]" {
		p.ToggleSidebar()
		return p, nil
	}
	if key == "tab" && p.splitSidebarVisible() {
		p.sidebarFocused = !p.sidebarFocused
		return p, nil
	}
	return p, nil
}

func (p *ChatPane) handleStreamToken(msg tui.StreamTokenMsg) (tui.ChatPaner, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	run.Builder.AddToken(msg)
	partial := run.Builder.BuildPartial()
	p.feed.UpdateMessage(partial)
	return p, listenToStream(run.Ch)
}

func (p *ChatPane) handleStreamComplete(msg tui.StreamCompleteMsg) (tui.ChatPaner, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	run.Builder.SetResult(structuredResultFromCore(msg.Result))
	final := run.Builder.Build(msg.Duration, msg.TokensUsed)
	if p.compactRunID != "" && msg.RunID == p.compactRunID {
		count := p.compactMsgCount
		p.compactRunID = ""
		p.compactMsgCount = 0
		delete(p.runStates, msg.RunID)
		summary := strings.TrimSpace(final.Content.Text)
		if summary == "" {
			summary = extractCompactSummary(msg.Result)
		}
		return p, func() tea.Msg {
			if summary == "" {
				return tui.CompactResultMsg{Err: fmt.Errorf("model returned empty summary"), OriginalCount: count}
			}
			return tui.CompactResultMsg{Summary: summary, OriginalCount: count}
		}
	}
	p.feed.UpdateMessage(final)
	if p.session != nil {
		p.session.TotalTokens += msg.TokensUsed
		p.session.TotalDuration += msg.Duration
	}
	if dropped := atomic.LoadInt64(&run.Dropped); dropped > 0 {
		p.addSystemMessage(fmt.Sprintf("Stream backpressure: dropped %d update(s)", dropped))
	}
	delete(p.runStates, msg.RunID)
	if !p.HasActiveRuns() {
		p.deregisterSpinnerAnim()
	}
	return p, func() tea.Msg { return tui.StreamDoneMsg{RunID: msg.RunID} }
}

func (p *ChatPane) handleStreamError(msg tui.StreamErrorMsg) (tui.ChatPaner, tea.Cmd) {
	delete(p.runStates, msg.RunID)
	if !p.HasActiveRuns() {
		p.deregisterSpinnerAnim()
	}
	if p.compactRunID != "" && msg.RunID == p.compactRunID {
		count := p.compactMsgCount
		p.compactRunID = ""
		p.compactMsgCount = 0
		return p, func() tea.Msg { return tui.CompactResultMsg{Err: msg.Error, OriginalCount: count} }
	}
	if msg.Error != nil && errors.Is(msg.Error, context.Canceled) {
		p.addSystemMessage(fmt.Sprintf("Run %s canceled", msg.RunID))
	} else {
		p.addSystemMessage(fmt.Sprintf("Agent error: %v", msg.Error))
	}
	return p, nil
}

func (p *ChatPane) handleUpdateTask(msg tui.UpdateTaskMsg) (tui.ChatPaner, tea.Cmd) {
	p.feed.Mutate(func(msgs []tui.Message) {
		for i := len(msgs) - 1; i >= 0; i-- {
			content := &msgs[i].Content
			if content.Plan == nil {
				continue
			}
			if msg.TaskIndex >= 0 && msg.TaskIndex < len(content.Plan.Tasks) {
				t := &content.Plan.Tasks[msg.TaskIndex]
				t.Status = msg.Status
				switch msg.Status {
				case tui.TaskInProgress:
					t.StartTime = time.Now()
				case tui.TaskCompleted:
					t.EndTime = time.Now()
				}
				return
			}
		}
	})
	return p, nil
}

func (p *ChatPane) runStream(ctx context.Context, run *tui.RunState, metadata map[string]any) {
	start := time.Now()
	sendRunMsg(run, tui.StreamTokenMsg{
		RunID:     run.ID,
		TokenType: tui.TokenThinking,
		Metadata: map[string]interface{}{
			"kind":        "start",
			"stepType":    string(tui.StepAnalyzing),
			"description": "Analyzing request",
		},
	})
	callback := func(token string) {
		sendRunMsg(run, tui.StreamTokenMsg{RunID: run.ID, TokenType: tui.TokenText, Token: token})
	}
	result, err := p.runtime.ExecuteInstructionStream(ctx, run.Prompt, execution.TaskTypeCodeGeneration, metadata, callback)
	if err != nil {
		sendRunFinal(run, tui.StreamErrorMsg{RunID: run.ID, Error: err})
		sendRunFinal(run, tui.StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: 0})
		close(run.Ch)
		return
	}
	tokenCount := 0
	if summary := summarizeResult(result); summary != "" {
		tokenCount = tui.EstimateTokens(summary)
	}
	sendRunFinal(run, tui.StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: tokenCount, Result: result})
	close(run.Ch)
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
	if len(p.workspaceSelection) > 0 {
		meta["euclo.user_selected_files"] = append([]string(nil), p.workspaceSelection...)
	}
	if p.session != nil {
		if p.session.Mode != "" {
			meta["mode"] = p.session.Mode
		}
		if p.session.Strategy != "" {
			meta["strategy"] = p.session.Strategy
		}
	}
	if p.activeSubTab != "" {
		if policy, ok := chatSubTabPolicies[p.activeSubTab]; ok {
			if policy.ModeHint != "" {
				meta["mode"] = policy.ModeHint
			}
			meta["edit_enabled"] = policy.EditEnabled
			meta["online_tools_enabled"] = policy.OnlineToolsEnabled
		}
	}
	return meta
}

func (p *ChatPane) normalizeMilestoneMessage(msg tui.Message) tui.Message {
	if msg.Role != tui.RoleAgent {
		return msg
	}
	text := strings.TrimSpace(msg.Content.Text)
	if text == "" {
		return msg
	}
	if isRawEventNoise(text) {
		msg.Content.Text = ""
		return msg
	}
	msg.Content.Text = summarizeMilestoneText(text)
	return msg
}

func (p *ChatPane) restoreSnapshot(primary, secondary *[][]tui.Message) bool {
	if p.HasActiveRuns() || len(*primary) == 0 {
		return false
	}
	currentSnapshot := make([]tui.Message, len(p.feed.Messages()))
	copy(currentSnapshot, p.feed.Messages())
	*secondary = append(*secondary, currentSnapshot)
	lastSnapshot := (*primary)[len(*primary)-1]
	*primary = (*primary)[:len(*primary)-1]
	p.feed.ClearMessages()
	for _, msg := range lastSnapshot {
		p.feed.AppendMessage(msg)
	}
	return true
}

func (p *ChatPane) splitSidebarVisible() bool {
	return p.showSidebar && p.sidebarWidth(p.width) > 0
}

func (p *ChatPane) sidebarWidth(totalWidth int) int {
	if totalWidth < 90 {
		return 0
	}
	if totalWidth < 120 {
		return 28
	}
	return 32
}

func (p *ChatPane) renderSidebar() string {
	width := p.sidebarWidth(p.width)
	if width <= 0 {
		return ""
	}
	header := "Euclo Chat"
	if p.sidebarFocused {
		header += " [sidebar]"
	} else {
		header += " [feed]"
	}
	var parts []string
	parts = append(parts, p.th.Subhead().Render(header))
	parts = append(parts, p.th.Dim().Render("Recipe Scope"))
	for _, tag := range p.recipeScopeTags() {
		parts = append(parts, "  "+p.checkbox(false)+" "+tag)
	}
	parts = append(parts, "")
	parts = append(parts, p.th.Dim().Render("Workspace Files"))
	if len(p.sidebarEntries) == 0 {
		parts = append(parts, "  "+p.th.Dim().Render("no files selected"))
	} else {
		for i, entry := range p.sidebarEntries {
			checked := p.isSelected(entry.Path)
			line := fmt.Sprintf("  %s %s", p.checkbox(checked), entry.Path)
			if i == p.sidebarSelection && p.sidebarFocused {
				line = p.th.Active().Render(line)
			} else {
				line = p.th.Body().Render(line)
			}
			parts = append(parts, line)
		}
	}
	parts = append(parts, "")
	parts = append(parts, p.th.Dim().Render("AST Options"))
	for _, opt := range p.astOptions() {
		active := opt == p.astOption
		line := fmt.Sprintf("  %s %s", p.checkbox(active), opt)
		if active {
			line = p.th.Active().Render(line)
		}
		parts = append(parts, line)
	}
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.th.Palette().Secondary).
		Padding(0, 1).
		Render(strings.Join(parts, "\n"))
}

func (p *ChatPane) checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

func (p *ChatPane) recipeScopeTags() []string {
	return []string{"local", "workspace", "review"}
}

func (p *ChatPane) astOptions() []string {
	return []string{"workspace", "diff", "session"}
}

func (p *ChatPane) isSelected(path string) bool {
	for _, selected := range p.workspaceSelection {
		if selected == path {
			return true
		}
	}
	return false
}

func (p *ChatPane) selectedSidebarEntry() tui.ContextSidebarEntry {
	if p.sidebarSelection < 0 || p.sidebarSelection >= len(p.sidebarEntries) {
		return tui.ContextSidebarEntry{}
	}
	return p.sidebarEntries[p.sidebarSelection]
}

func (p *ChatPane) updateSidebarContent() {
	if p.context == nil {
		p.sidebarEntries = nil
		return
	}
	entries := make([]tui.ContextSidebarEntry, 0, len(p.context.Files))
	for _, file := range p.context.Files {
		entries = append(entries, tui.ContextSidebarEntry{Path: file, InsertionAction: "direct"})
	}
	p.sidebarEntries = entries
	if p.sidebarSelection >= len(p.sidebarEntries) {
		p.sidebarSelection = max(0, len(p.sidebarEntries)-1)
	}
}

func (p *ChatPane) addSelectedFile(path string) {
	path = strings.TrimSpace(path)
	if path == "" || p.isSelected(path) {
		p.syncSelectionEnv()
		return
	}
	p.workspaceSelection = append(p.workspaceSelection, path)
	p.syncSelectionEnv()
}

func (p *ChatPane) removeSelectedFile(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	filtered := p.workspaceSelection[:0]
	for _, selected := range p.workspaceSelection {
		if selected != path {
			filtered = append(filtered, selected)
		}
	}
	p.workspaceSelection = append([]string(nil), filtered...)
	p.syncSelectionEnv()
}

func (p *ChatPane) replaceSelectedFiles(files []string) {
	seen := make(map[string]struct{}, len(files))
	p.workspaceSelection = p.workspaceSelection[:0]
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		p.workspaceSelection = append(p.workspaceSelection, file)
	}
	p.syncSelectionEnv()
}

func (p *ChatPane) syncSelectionEnv() {
	if p.selectionEnv == nil {
		taskID := ""
		sessionID := ""
		if p.session != nil {
			taskID = p.session.ID
			sessionID = p.session.ID
		}
		p.selectionEnv = contextdata.NewEnvelope(taskID, sessionID)
	}
	euclostate.SetUserSelectedFiles(p.selectionEnv, append([]string(nil), p.workspaceSelection...))
}

func (p *ChatPane) addSystemMessage(text string) {
	msg := tui.Message{
		ID:        tui.GenerateID(),
		Timestamp: time.Now(),
		Role:      tui.RoleSystem,
		Content:   tui.MessageContent{Text: text},
	}
	p.feed.AppendMessage(msg)
}

func (p *ChatPane) normalizeRenderedFrameText(text string) string {
	return summarizeMilestoneText(text)
}

func extractFileTokens(text string) []string {
	var files []string
	fields := strings.Fields(text)
	for _, field := range fields {
		if strings.HasPrefix(field, "@") && len(field) > 1 {
			files = append(files, strings.TrimPrefix(field, "@"))
		}
	}
	return files
}

func sessionWorkspace(sess *tui.Session) string {
	if sess == nil {
		return ""
	}
	return strings.TrimSpace(sess.Workspace)
}

func removeFileTokens(text string) string {
	fields := strings.Fields(text)
	var kept []string
	for _, field := range fields {
		if strings.HasPrefix(field, "@") {
			continue
		}
		kept = append(kept, field)
	}
	return strings.Join(kept, " ")
}

func summarizeMilestoneText(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			line = strings.TrimSpace(line[strings.Index(line, "]")+1:])
		}
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "●") {
			line = "● " + line
		}
		return line
	}
	return ""
}

func isRawEventNoise(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "map[") || strings.Contains(trimmed, "\"frame_id\"")
}

func listenToStream(ch <-chan tea.Msg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func sendRunMsg(run *tui.RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		atomic.AddInt64(&run.Dropped, 1)
	}
}

func sendRunFinal(run *tui.RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		go func() { run.Ch <- msg }()
	}
}

func summarizeResult(res *execution.Result) string {
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
	if fields := execution.ResultFields(res.Data); len(fields) > 0 {
		b.WriteString("\nData: ")
		b.WriteString(fmt.Sprintf("%v", fields))
	}
	if strings.TrimSpace(res.Error) != "" {
		b.WriteString("\nError: ")
		b.WriteString(res.Error)
	}
	return b.String()
}

func extractCompactSummary(result *execution.Result) string {
	if result == nil {
		return ""
	}
	fields := execution.ResultFields(result.Data)
	if len(fields) == 0 {
		return ""
	}
	for _, key := range []string{"final_output", "text", "summary"} {
		if v, ok := fields[key]; ok {
			if s, ok := v.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func structuredResultFromCore(res *execution.Result) *tui.StructuredResult {
	if res == nil {
		return nil
	}
	rendered := &tui.StructuredResult{
		NodeID:  strings.TrimSpace(res.NodeID),
		Success: res.Success,
	}
	if strings.TrimSpace(res.Error) != "" {
		rendered.ErrorText = res.Error
	}
	if envelope := extractResultEnvelope(res); envelope != nil {
		rendered.Envelope = structuredEnvelopeFromCore(envelope)
	}
	if rendered.NodeID == "" && rendered.Envelope == nil && rendered.ErrorText == "" {
		return nil
	}
	return rendered
}

func extractResultEnvelope(res *execution.Result) *capability.CapabilityResultEnvelope {
	if res == nil {
		return nil
	}
	fields := execution.ResultFields(res.Data)
	if len(fields) == 0 {
		return nil
	}
	for _, key := range []string{"result", "tool_result", "capability_result"} {
		raw, ok := fields[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case *contracts.ToolResult:
			if envelope, ok := capability.ToolResultEnvelope(typed); ok {
				return envelope
			}
		case contracts.ToolResult:
			copy := typed
			if envelope, ok := capability.ToolResultEnvelope(&copy); ok {
				return envelope
			}
		case *capability.CapabilityResultEnvelope:
			return typed
		case capability.CapabilityResultEnvelope:
			copy := typed
			return &copy
		}
	}
	return nil
}

func structuredEnvelopeFromCore(envelope *capability.CapabilityResultEnvelope) *tui.StructuredResultEnvelope {
	if envelope == nil {
		return nil
	}
	rendered := &tui.StructuredResultEnvelope{
		CapabilityID:   envelope.Descriptor.ID,
		CapabilityName: envelope.Descriptor.Name,
		TrustClass:     string(envelope.Descriptor.TrustClass),
		Disposition:    string(envelope.Disposition),
		Insertion: tui.StructuredInsertion{
			Action:       string(envelope.Insertion.Action),
			Reason:       envelope.Insertion.Reason,
			RequiresHITL: envelope.Insertion.RequiresHITL,
		},
		Blocks: make([]tui.StructuredContentBlock, 0, len(envelope.ContentBlocks)),
	}
	if envelope.Approval != nil {
		rendered.Approval = &tui.StructuredApprovalBinding{
			CapabilityID:   envelope.Approval.CapabilityID,
			CapabilityName: envelope.Approval.CapabilityName,
			ProviderID:     envelope.Approval.ProviderID,
			SessionID:      envelope.Approval.SessionID,
			TargetResource: envelope.Approval.TargetResource,
			TaskID:         envelope.Approval.TaskID,
			WorkflowID:     envelope.Approval.WorkflowID,
			EffectClasses:  effectClassLabels(envelope.Approval.EffectClasses),
		}
	}
	insertionsByType := map[string]tui.StructuredInsertion{}
	for _, insertion := range envelope.BlockInsertions {
		insertionsByType[insertion.ContentType] = tui.StructuredInsertion{
			Action:       string(insertion.Decision.Action),
			Reason:       insertion.Decision.Reason,
			RequiresHITL: insertion.Decision.RequiresHITL,
		}
	}
	for _, block := range envelope.ContentBlocks {
		if block == nil {
			continue
		}
		renderedBlock := structuredBlockFromCore(block)
		if insertion, ok := insertionsByType[block.ContentType()]; ok {
			renderedBlock.Summary = strings.TrimSpace(strings.Join([]string{renderedBlock.Summary, insertionBadge(insertion)}, " "))
		}
		rendered.Blocks = append(rendered.Blocks, renderedBlock)
	}
	return rendered
}

func structuredBlockFromCore(block capability.ContentBlock) tui.StructuredContentBlock {
	switch typed := block.(type) {
	case capability.TextContentBlock:
		return tui.StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "text output",
			Body:       strings.TrimSpace(typed.Text),
			Provenance: provenanceMap(typed.Provenance),
		}
	case capability.StructuredContentBlock:
		return tui.StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "structured output",
			Body:       formatStructuredData(typed.Data),
			Provenance: provenanceMap(typed.Provenance),
		}
	case capability.ResourceLinkContentBlock:
		summary := "linked resource"
		if typed.Name != "" {
			summary = typed.Name
		}
		body := typed.URI
		if typed.MIMEType != "" {
			body += "\nMIME: " + typed.MIMEType
		}
		return tui.StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    summary,
			Body:       body,
			Provenance: provenanceMap(typed.Provenance),
		}
	case capability.EmbeddedResourceContentBlock:
		return tui.StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "embedded resource",
			Body:       strings.TrimSpace(fmt.Sprintf("%v", typed.Resource)),
			Provenance: provenanceMap(typed.Provenance),
		}
	default:
		return tui.StructuredContentBlock{
			Type:    block.ContentType(),
			Summary: "content",
			Body:    fmt.Sprintf("%v", block),
		}
	}
}

func formatStructuredData(data any) string {
	if data == nil {
		return ""
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(raw)
}

func provenanceMap(prov capability.ContentProvenance) map[string]string {
	out := make(map[string]string, 4)
	if prov.CapabilityID != "" {
		out["capability_id"] = prov.CapabilityID
	}
	if prov.ProviderID != "" {
		out["provider_id"] = prov.ProviderID
	}
	if prov.TrustClass != "" {
		out["trust_class"] = string(prov.TrustClass)
	}
	if prov.Disposition != "" {
		out["disposition"] = string(prov.Disposition)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func insertionBadge(insertion tui.StructuredInsertion) string {
	switch insertion.Action {
	case "allow":
		return "[allow]"
	case "summarize":
		return "[sum]"
	case "reference":
		return "[ref]"
	default:
		return "[raw]"
	}
}

func effectClassLabels(classes []agentspec.EffectClass) []string {
	if len(classes) == 0 {
		return nil
	}
	out := make([]string, 0, len(classes))
	for _, class := range classes {
		if class == "" {
			continue
		}
		out = append(out, string(class))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *ChatPane) SetAnimManager(m *tui.AnimationManager) {
	p.anim = m
	if p.anim != nil && p.HasActiveRuns() {
		p.registerSpinnerAnim()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
