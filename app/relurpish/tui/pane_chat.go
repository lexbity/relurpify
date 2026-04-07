package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lexcodex/relurpify/framework/core"
)

// ChatSystemMsg is a message to add a system line to the chat feed.
// Exported so euclotui can emit this message type.
type ChatSystemMsg struct{ Text string }

// chatSystemMsg is the package-internal name for ChatSystemMsg.
// It exists so that existing tui code can continue to use the lower-case
// constructor syntax: chatSystemMsg{Text: "..."}
type chatSystemMsg = ChatSystemMsg

// ChatSubTabPolicy declares the execution behaviour for a specific chat subtab.
type ChatSubTabPolicy struct {
	// ModeHint is passed as "mode" metadata to guide the agent's execution style.
	ModeHint string
	// EditEnabled controls whether write-path capabilities are active.
	EditEnabled bool
	// OnlineToolsEnabled controls whether network-fetching tools are active.
	OnlineToolsEnabled bool
}

// chatSubTabPolicies maps each chat subtab to its execution policy.
var chatSubTabPolicies = map[SubTabID]ChatSubTabPolicy{
	SubTabChatLocalRead:   {ModeHint: "review", EditEnabled: false, OnlineToolsEnabled: false},
	SubTabChatLocalEdit:   {ModeHint: "code", EditEnabled: true, OnlineToolsEnabled: false},
	SubTabChatOnlineRead:  {ModeHint: "research", EditEnabled: false, OnlineToolsEnabled: true},
	SubTabChatOnlineEdit:  {ModeHint: "code", EditEnabled: true, OnlineToolsEnabled: true},
}

// ChatPane owns the message feed and run management.
// It is always held by pointer so mutations survive tea.Model value copies.
type ChatPane struct {
	feed      *Feed
	spinner   spinner.Model
	runStates map[string]*RunState

	context    *AgentContext
	session    *Session
	notifQ     *NotificationQueue
	hitlSvc    hitlService
	runtime    RuntimeAdapter
	lastPrompt string

	allowParallel bool
	expandTarget  string
	activeSubTab  SubTabID

	width, height int

	// Feed snapshots for undo/redo: each snapshot is a full copy of message list
	undoStack [][]Message
	redoStack [][]Message

	// compact run tracking: set by rootHandleCompact, cleared on completion
	compactRunID    string
	compactMsgCount int

	// Context sidebar state
	showSidebar      bool
	sidebarWidth     int
	sidebarCursor    int
	sidebarViewport  viewport.Model
	contextEntries   []ContextSidebarEntry
}

// NewChatPane initializes the ChatPane. The feed is created but not sized yet.
func NewChatPane(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) *ChatPane {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	svc := hitlService(rt)

	// Initialize sidebar viewport
	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{
		Up:   key.NewBinding(key.WithKeys("up", "k")),
		Down: key.NewBinding(key.WithKeys("down", "j")),
	}

	return &ChatPane{
		feed:            NewFeed(),
		spinner:         sp,
		runStates:       make(map[string]*RunState),
		context:         ctx,
		session:         sess,
		notifQ:          notifQ,
		hitlSvc:         svc,
		runtime:         rt,
		expandTarget:    "thinking",
		showSidebar:     false,
		sidebarWidth:    30,
		sidebarCursor:   0,
		sidebarViewport: vp,
		contextEntries:  []ContextSidebarEntry{},
	}
}
}

// SetSubTab updates the active chat subtab and adjusts execution policy.
func (p *ChatPane) SetSubTab(id SubTabID) {
	p.activeSubTab = id
}

// ActiveSubTab returns the current chat subtab.
func (p *ChatPane) ActiveSubTab() SubTabID { return p.activeSubTab }

// Init starts the spinner.
func (p *ChatPane) Init() tea.Cmd {
	return p.spinner.Tick
}

// Cleanup cancels all active runs.
func (p *ChatPane) Cleanup() {
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
	
	// Calculate available width for feed (accounting for sidebar)
	feedWidth := w
	if p.showSidebar && w > 90 {
		feedWidth = w - p.sidebarWidth
	}
	p.feed.SetSize(feedWidth, h)
	
	// Update sidebar viewport
	if p.showSidebar {
		p.sidebarViewport.Width = p.sidebarWidth
		p.sidebarViewport.Height = h - 2 // Leave room for header
		p.updateSidebarContent()
	}
}

// Undo reverts the feed to the previous snapshot state.
// Returns true on success, false if undo not possible (active run or empty stack).
func (p *ChatPane) Undo() bool {
	// Do not undo if a run is active
	if p.HasActiveRuns() {
		return false
	}
	if len(p.undoStack) == 0 {
		return false
	}

	// Save current state to redo stack
	currentSnapshot := make([]Message, len(p.feed.Messages()))
	copy(currentSnapshot, p.feed.Messages())
	p.redoStack = append(p.redoStack, currentSnapshot)

	// Restore previous snapshot
	lastSnapshot := p.undoStack[len(p.undoStack)-1]
	p.undoStack = p.undoStack[:len(p.undoStack)-1]

	// Replace feed contents with snapshot
	p.feed.ClearMessages()
	for _, msg := range lastSnapshot {
		p.feed.AppendMessage(msg)
	}

	return true
}

// Redo restores the feed to the next snapshot state.
// Returns true on success, false if redo not possible (active run or empty stack).
func (p *ChatPane) Redo() bool {
	// Do not redo if a run is active
	if p.HasActiveRuns() {
		return false
	}
	if len(p.redoStack) == 0 {
		return false
	}

	// Save current state to undo stack
	currentSnapshot := make([]Message, len(p.feed.Messages()))
	copy(currentSnapshot, p.feed.Messages())
	p.undoStack = append(p.undoStack, currentSnapshot)

	// Restore next snapshot
	nextSnapshot := p.redoStack[len(p.redoStack)-1]
	p.redoStack = p.redoStack[:len(p.redoStack)-1]

	// Replace feed contents with snapshot
	p.feed.ClearMessages()
	for _, msg := range nextSnapshot {
		p.feed.AppendMessage(msg)
	}

	return true
}

// ToggleCompact toggles between normal and compact message display.
func (p *ChatPane) ToggleCompact() {
	// Cycle through display modes: thinking -> plan -> all (or back to thinking)
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

// HasActiveRuns returns true if any run is in flight.
func (p *ChatPane) HasActiveRuns() bool {
	return len(p.runStates) > 0
}

// Update dispatches tea messages relevant to the chat pane.
// Returns (ChatPaner, tea.Cmd) to satisfy the ChatPaner interface.
func (p *ChatPane) Update(msg tea.Msg) (ChatPaner, tea.Cmd) {
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

	case chatSystemMsg:
		p.addSystemMessage(msg.Text)
		return p, nil

	case tea.KeyMsg:
		// Handle sidebar keys when sidebar is shown
		if p.showSidebar && p.width > 90 {
			switch msg.String() {
			case "ctrl+]":
				p.ToggleSidebar()
				return p, nil
			case "up", "k", "down", "j", "x", "d", "a":
				if cmd := p.HandleSidebarKey(msg.String()); cmd != nil {
					return p, cmd
				}
				return p, nil
			}
		}
		// Handle sidebar toggle
		if msg.String() == "ctrl+]" {
			p.ToggleSidebar()
			return p, nil
		}
	case tea.MouseMsg:
		f, cmd := p.feed.Update(msg)
		p.feed = f
		return p, cmd
	}
	return p, nil
}

// View renders the feed (input bar is rendered by root).
func (p *ChatPane) View() string {
	if !p.showSidebar || p.width <= 90 {
		return p.feed.View()
	}
	
	// Render with sidebar
	feedView := p.feed.View()
	sidebarView := p.renderSidebar()
	
	return lipgloss.JoinHorizontal(lipgloss.Top, feedView, sidebarView)
}

// HandleInputSubmit processes text submitted from the input bar.
func (p *ChatPane) HandleInputSubmit(value string) tea.Cmd {
	// Extract @file tokens and resolve them
	cleanedValue := value
	files := extractFileTokens(value)
	if len(files) > 0 && p.runtime != nil {
		// Resolve file tokens to actual context files
		resolution := p.runtime.ResolveContextFiles(context.Background(), files)
		if len(resolution.Allowed) > 0 {
			if p.context != nil {
				p.context.Files = append(p.context.Files, resolution.Allowed...)
			}
		}
		// Remove @tokens from the prompt text
		cleanedValue = removeFileTokens(value)
	}

	// Snapshot current feed state for undo before starting the run
	if len(p.feed.Messages()) > 0 {
		snapshot := make([]Message, len(p.feed.Messages()))
		copy(snapshot, p.feed.Messages())
		p.undoStack = append(p.undoStack, snapshot)
		// Clear redo stack when taking a new action
		p.redoStack = nil
	}

	cmd, _ := p.StartRun(cleanedValue)
	return cmd
}

// StartRun begins an agent run. Returns the Bubble Tea command and the run ID.
// The run ID is empty when the run could not be started.
func (p *ChatPane) StartRun(prompt string) (tea.Cmd, string) {
	return p.StartRunWithMetadata(prompt, nil)
}

// StartRunSilent begins an agent run without appending a user message to the feed.
// Used by /compact so the summarisation prompt is not visible in the chat.
// The caller is responsible for setting p.compactRunID = runID after this returns.
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
	metadata := p.buildMetadata(ctx)
	metadata["compact"] = true
	go p.runStream(ctx, run, metadata)
	return listenToStream(ch), runID
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
		return func() tea.Msg { return chatSystemMsg{Text: fmt.Sprintf("Context error: %v", err)} }
	}
	if p.runtime == nil {
		return func() tea.Msg { return chatSystemMsg{Text: fmt.Sprintf("Added to context: %s", path)} }
	}
	resolution := p.runtime.ResolveContextFiles(context.Background(), []string{path})
	if len(resolution.Contents) == 0 {
		return func() tea.Msg { return chatSystemMsg{Text: fmt.Sprintf("Added to context: %s", path)} }
	}
	content := resolution.Contents[0].Content
	size := int64(len(content))
	if resolution.Contents[0].Truncated {
		size = contextFileMaxBytes
	}
	return func() tea.Msg {
		return chatSystemMsg{Text: fmt.Sprintf("Added to context: %s (%s)", path, formatSizeToken(size, estimateTokensFromBytes(size)))}
	}
}

// StopLatestRun cancels the most recently started run.
func (p *ChatPane) StopLatestRun() tea.Cmd {
	if len(p.runStates) == 0 {
		return func() tea.Msg { return chatSystemMsg{Text: "No active run to stop."} }
	}
	var latest *RunState
	for _, run := range p.runStates {
		if latest == nil || run.Started.After(latest.Started) {
			latest = run
		}
	}
	if latest == nil || latest.Cancel == nil {
		return func() tea.Msg { return chatSystemMsg{Text: "No active run to stop."} }
	}
	latest.Cancel()
	return func() tea.Msg { return chatSystemMsg{Text: fmt.Sprintf("Stopping run %s", latest.ID)} }
}

// RetryLastRun restarts the most recent prompt.
func (p *ChatPane) RetryLastRun() tea.Cmd {
	if strings.TrimSpace(p.lastPrompt) == "" {
		return func() tea.Msg { return chatSystemMsg{Text: "No prior prompt to retry."} }
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

// updateSidebarContent refreshes the sidebar entries from context
func (p *ChatPane) updateSidebarContent() {
	if p.context == nil {
		p.contextEntries = []ContextSidebarEntry{}
		return
	}
	
	// Convert context files to sidebar entries
	entries := make([]ContextSidebarEntry, 0, len(p.context.Files))
	for _, file := range p.context.Files {
		// Determine insertion action (simplified - would need actual logic)
		insertionAction := "direct"
		if strings.HasSuffix(file, ".md") || strings.HasSuffix(file, ".txt") {
			insertionAction = "direct"
		} else if strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".py") {
			insertionAction = "direct"
		} else {
			insertionAction = "metadata-only"
		}
		
		// Check if it's a session pin (simplified)
		isPin := false
		if p.session != nil && strings.Contains(file, p.session.Workspace) {
			isPin = true
		}
		
		entries = append(entries, ContextSidebarEntry{
			Path:            file,
			InsertionAction: insertionAction,
			IsPin:           isPin,
		})
	}
	p.contextEntries = entries
	
	// Update viewport content
	content := p.renderSidebarContent()
	p.sidebarViewport.SetContent(content)
}

// renderSidebarContent generates the sidebar content
func (p *ChatPane) renderSidebarContent() string {
	var b strings.Builder
	
	b.WriteString(sectionHeaderStyle.Render("Context") + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", p.sidebarWidth-2)) + "\n")
	
	if len(p.contextEntries) == 0 {
		b.WriteString(dimStyle.Render("No files in context") + "\n")
	} else {
		for i, entry := range p.contextEntries {
			line := p.renderSidebarEntry(entry, i == p.sidebarCursor)
			b.WriteString(line + "\n")
		}
	}
	
	b.WriteString("\n" + dimStyle.Render(strings.Repeat("─", p.sidebarWidth-2)) + "\n")
	b.WriteString(dimStyle.Render("[a] add  [x] remove"))
	
	return b.String()
}

// renderSidebarEntry renders a single sidebar entry
func (p *ChatPane) renderSidebarEntry(entry ContextSidebarEntry, selected bool) string {
	// Format path for display
	displayPath := entry.Path
	if len(displayPath) > p.sidebarWidth-10 {
		displayPath = "..." + displayPath[len(displayPath)-(p.sidebarWidth-10):]
	}
	
	// Add pin indicator
	prefix := "  "
	if entry.IsPin {
		prefix = "· "
	}
	
	// Add insertion action badge
	badge := ""
	switch entry.InsertionAction {
	case "direct":
		badge = "[dir]"
	case "summarized":
		badge = "[sum]"
	case "metadata-only":
		badge = "[ref]"
	}
	
	line := fmt.Sprintf("%s%s %s", prefix, displayPath, dimStyle.Render(badge))
	
	if selected {
		return panelItemActiveStyle.Render(line)
	}
	return panelItemStyle.Render(line)
}

// renderSidebar renders the entire sidebar
func (p *ChatPane) renderSidebar() string {
	return lipgloss.NewStyle().
		Width(p.sidebarWidth).
		Height(p.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(p.sidebarViewport.View())
}

// ToggleSidebar shows/hides the context sidebar
func (p *ChatPane) ToggleSidebar() {
	p.showSidebar = !p.showSidebar
	p.SetSize(p.width, p.height) // Recalculate sizes
}

// AddFileToSidebar adds a file to the context and updates sidebar
func (p *ChatPane) AddFileToSidebar(path string) error {
	if p.context == nil {
		return fmt.Errorf("context unavailable")
	}
	if err := p.context.AddFile(path); err != nil {
		return err
	}
	p.updateSidebarContent()
	return nil
}

// RemoveFileFromSidebar removes a file from context and updates sidebar
func (p *ChatPane) RemoveFileFromSidebar(path string) {
	if p.context == nil {
		return
	}
	p.context.RemoveFile(path)
	p.updateSidebarContent()
}

// HandleSidebarKey handles key events when sidebar is focused
func (p *ChatPane) HandleSidebarKey(key string) tea.Cmd {
	switch key {
	case "up", "k":
		if p.sidebarCursor > 0 {
			p.sidebarCursor--
			p.sidebarViewport.LineUp(1)
		}
	case "down", "j":
		if p.sidebarCursor < len(p.contextEntries)-1 {
			p.sidebarCursor++
			p.sidebarViewport.LineDown(1)
		}
	case "x", "d":
		if p.sidebarCursor >= 0 && p.sidebarCursor < len(p.contextEntries) {
			entry := p.contextEntries[p.sidebarCursor]
			p.RemoveFileFromSidebar(entry.Path)
			// Adjust cursor if needed
			if p.sidebarCursor >= len(p.contextEntries) {
				p.sidebarCursor = max(0, len(p.contextEntries)-1)
			}
			return func() tea.Msg {
				return chatSystemMsg{Text: fmt.Sprintf("Removed from context: %s", entry.Path)}
			}
		}
	case "a":
		// Open file picker (would need integration with input bar)
		return func() tea.Msg {
			return chatSystemMsg{Text: "File picker not yet implemented"}
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────
// ChatPaner interface implementation
// ──────────────────────────────────────────────────────────────

// AddSystemMessage is the exported form of addSystemMessage, required by ChatPaner.
func (p *ChatPane) AddSystemMessage(text string) { p.addSystemMessage(text) }

// AppendMessage adds a message to the chat feed.
func (p *ChatPane) AppendMessage(msg Message) { p.feed.AppendMessage(msg) }

// ClearMessages clears the chat feed.
func (p *ChatPane) ClearMessages() { p.feed.ClearMessages() }

// Messages returns the current message list.
func (p *ChatPane) Messages() []Message { return p.feed.Messages() }

// SetSearchFilter sets the feed search filter.
func (p *ChatPane) SetSearchFilter(filter string) { p.feed.SetSearchFilter(filter) }

// ScrollUp scrolls the feed up by one line.
func (p *ChatPane) ScrollUp() { p.feed.ScrollUp() }

// PageDown scrolls the feed down by one page.
func (p *ChatPane) PageDown() { p.feed.PageDown() }

// PageUp scrolls the feed up by one page.
func (p *ChatPane) PageUp() { p.feed.PageUp() }

// RollbackLastUndo removes the top undo snapshot (called on compact failure).
func (p *ChatPane) RollbackLastUndo() {
	if len(p.undoStack) > 0 {
		p.undoStack = p.undoStack[:len(p.undoStack)-1]
	}
}

// HITLService returns the underlying HITL service.
func (p *ChatPane) HITLService() HITLServiceIface { return p.hitlSvc }

// AllowParallel returns whether parallel runs are enabled.
func (p *ChatPane) AllowParallel() bool { return p.allowParallel }

// SetAllowParallel sets the parallel run mode.
func (p *ChatPane) SetAllowParallel(v bool) { p.allowParallel = v }

// LastPrompt returns the last submitted prompt text.
func (p *ChatPane) LastPrompt() string { return p.lastPrompt }

// MutateMessages calls fn with a mutable slice of all messages, then refreshes.
// fn may modify message fields in-place (e.g. toggling Expanded on a FileChange).
func (p *ChatPane) MutateMessages(fn func(msgs []Message)) {
	fn(p.feed.messages)
	p.feed.refresh()
}

// SetCompactRunID records the run ID and original message count for the
// in-flight compact operation. handleStreamComplete uses it to intercept the
// compact result and rebuild the feed.
func (p *ChatPane) SetCompactRunID(runID string, msgCount int) {
	p.compactRunID = runID
	p.compactMsgCount = msgCount
}

// PushUndoSnapshot appends a snapshot of the current messages to the undo
// stack and clears the redo stack. Called before destructive operations like
// /compact so that ctrl+z can restore the previous state.
func (p *ChatPane) PushUndoSnapshot(msgs []Message) {
	snapshot := make([]Message, len(msgs))
	copy(snapshot, msgs)
	p.undoStack = append(p.undoStack, snapshot)
	p.redoStack = nil
}

// Verify that *ChatPane satisfies ChatPaner at compile time.
var _ ChatPaner = (*ChatPane)(nil)

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
	// Apply subtab policy hints (override session mode when set).
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

	tokenCount := 0
	if summary := summarizeResult(result); summary != "" {
		tokenCount = estimateTokens(summary)
	}
	sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: tokenCount, Result: result})
	close(run.Ch)
}

func (p *ChatPane) handleStreamToken(msg StreamTokenMsg) (ChatPaner, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	run.Builder.AddToken(msg)
	partial := run.Builder.BuildPartial()
	p.feed.UpdateMessage(partial)
	return p, listenToStream(run.Ch)
}

func (p *ChatPane) handleStreamComplete(msg StreamCompleteMsg) (ChatPaner, tea.Cmd) {
	run, ok := p.runStates[msg.RunID]
	if !ok || run.Builder == nil {
		return p, nil
	}
	run.Builder.SetResult(structuredResultFromCore(msg.Result))
	final := run.Builder.Build(msg.Duration, msg.TokensUsed)

	// Compact run: don't append to feed; emit compactResultMsg instead.
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
				return compactResultMsg{err: fmt.Errorf("model returned empty summary"), originalCount: count}
			}
			return compactResultMsg{summary: summary, originalCount: count}
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
	return p, streamCompletedCmd(msg)
}

func (p *ChatPane) handleStreamError(msg StreamErrorMsg) (ChatPaner, tea.Cmd) {
	delete(p.runStates, msg.RunID)

	// Compact run error: emit compactResultMsg so model.go can roll back the undo snapshot.
	if p.compactRunID != "" && msg.RunID == p.compactRunID {
		count := p.compactMsgCount
		p.compactRunID = ""
		p.compactMsgCount = 0
		return p, func() tea.Msg {
			return compactResultMsg{err: msg.Error, originalCount: count}
		}
	}

	if msg.Error != nil && errors.Is(msg.Error, context.Canceled) {
		p.addSystemMessage(fmt.Sprintf("Run %s canceled", msg.RunID))
	} else {
		p.addSystemMessage(fmt.Sprintf("Agent error: %v", msg.Error))
	}
	return p, nil
}

func (p *ChatPane) handleUpdateTask(msg UpdateTaskMsg) (ChatPaner, tea.Cmd) {
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

func structuredResultFromCore(res *core.Result) *StructuredResult {
	if res == nil {
		return nil
	}
	rendered := &StructuredResult{
		NodeID:  strings.TrimSpace(res.NodeID),
		Success: res.Success,
	}
	if res.Error != nil {
		rendered.ErrorText = res.Error.Error()
	}
	if envelope := extractResultEnvelope(res); envelope != nil {
		rendered.Envelope = structuredEnvelopeFromCore(envelope)
	}
	if rendered.NodeID == "" && rendered.Envelope == nil && rendered.ErrorText == "" {
		return nil
	}
	return rendered
}

func extractResultEnvelope(res *core.Result) *core.CapabilityResultEnvelope {
	if res == nil || len(res.Data) == 0 {
		return nil
	}
	for _, key := range []string{"result", "tool_result", "capability_result"} {
		raw, ok := res.Data[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case *core.ToolResult:
			if envelope, ok := core.ToolResultEnvelope(typed); ok {
				return envelope
			}
		case core.ToolResult:
			copy := typed
			if envelope, ok := core.ToolResultEnvelope(&copy); ok {
				return envelope
			}
		case *core.CapabilityResultEnvelope:
			return typed
		case core.CapabilityResultEnvelope:
			copy := typed
			return &copy
		}
	}
	return nil
}

func structuredEnvelopeFromCore(envelope *core.CapabilityResultEnvelope) *StructuredResultEnvelope {
	if envelope == nil {
		return nil
	}
	rendered := &StructuredResultEnvelope{
		CapabilityID:   envelope.Descriptor.ID,
		CapabilityName: envelope.Descriptor.Name,
		TrustClass:     string(envelope.Descriptor.TrustClass),
		Disposition:    string(envelope.Disposition),
		Insertion: StructuredInsertion{
			Action:       string(envelope.Insertion.Action),
			Reason:       envelope.Insertion.Reason,
			RequiresHITL: envelope.Insertion.RequiresHITL,
		},
		Blocks: make([]StructuredContentBlock, 0, len(envelope.ContentBlocks)),
	}
	if envelope.Approval != nil {
		rendered.Approval = &StructuredApprovalBinding{
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
	insertionsByType := map[string]StructuredInsertion{}
	for _, insertion := range envelope.BlockInsertions {
		insertionsByType[insertion.ContentType] = StructuredInsertion{
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

func structuredBlockFromCore(block core.ContentBlock) StructuredContentBlock {
	switch typed := block.(type) {
	case core.TextContentBlock:
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "text output",
			Body:       strings.TrimSpace(typed.Text),
			Provenance: provenanceMap(typed.Provenance),
		}
	case core.StructuredContentBlock:
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "structured output",
			Body:       formatStructuredData(typed.Data),
			Provenance: provenanceMap(typed.Provenance),
		}
	case core.ResourceLinkContentBlock:
		summary := "linked resource"
		if typed.Name != "" {
			summary = typed.Name
		}
		body := typed.URI
		if typed.MIMEType != "" {
			body += "\nMIME: " + typed.MIMEType
		}
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    summary,
			Body:       body,
			Provenance: provenanceMap(typed.Provenance),
		}
	case core.EmbeddedResourceContentBlock:
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "embedded resource",
			Body:       formatStructuredData(typed.Resource),
			Provenance: provenanceMap(typed.Provenance),
		}
	case core.BinaryReferenceContentBlock:
		body := typed.Ref
		if typed.MIMEType != "" {
			body += "\nMIME: " + typed.MIMEType
		}
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "binary reference",
			Body:       body,
			Provenance: provenanceMap(typed.Provenance),
		}
	case core.ErrorContentBlock:
		body := strings.TrimSpace(typed.Message)
		if typed.Code != "" {
			body = typed.Code + ": " + body
		}
		return StructuredContentBlock{
			Type:       typed.ContentType(),
			Summary:    "error output",
			Body:       body,
			Provenance: provenanceMap(typed.Provenance),
		}
	default:
		return StructuredContentBlock{
			Type:    "unknown",
			Summary: "unknown output",
			Body:    fmt.Sprintf("%v", block),
		}
	}
}

func provenanceMap(provenance core.ContentProvenance) map[string]string {
	out := map[string]string{}
	if provenance.CapabilityID != "" {
		out["capability"] = provenance.CapabilityID
	}
	if provenance.ProviderID != "" {
		out["provider"] = provenance.ProviderID
	}
	if provenance.TrustClass != "" {
		out["trust"] = string(provenance.TrustClass)
	}
	if provenance.Disposition != "" {
		out["disposition"] = string(provenance.Disposition)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectClassLabels(classes []core.EffectClass) []string {
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

func insertionBadge(insertion StructuredInsertion) string {
	if insertion.Action == "" {
		return ""
	}
	if insertion.RequiresHITL {
		return "(" + insertion.Action + ", hitl)"
	}
	return "(" + insertion.Action + ")"
}

func formatStructuredData(value any) string {
	if value == nil {
		return ""
	}
	if data, err := json.MarshalIndent(value, "", "  "); err == nil && len(data) > 0 {
		return string(data)
	}
	return fmt.Sprintf("%v", value)
}

// extractFileTokens extracts @path tokens from a text string.
func extractFileTokens(text string) []string {
	tokens := []string{}
	words := strings.Fields(text)
	for _, word := range words {
		if strings.HasPrefix(word, "@") && len(word) > 1 {
			path := strings.TrimPrefix(word, "@")
			tokens = append(tokens, path)
		}
	}
	return tokens
}

// removeFileTokens removes @path tokens from a text string, preserving other text.
func removeFileTokens(text string) string {
	words := strings.Fields(text)
	var result []string
	for _, word := range words {
		if !strings.HasPrefix(word, "@") || len(word) == 1 {
			result = append(result, word)
		}
	}
	return strings.TrimSpace(strings.Join(result, " "))
}
