package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/core"
)

// chatSystemMsg is an internal message to add a system line to the chat feed.
type chatSystemMsg struct{ text string }

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

	width, height int

	// Feed snapshots for undo/redo: each snapshot is a full copy of message list
	undoStack [][]Message
	redoStack [][]Message
}

// NewChatPane initializes the ChatPane. The feed is created but not sized yet.
func NewChatPane(rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) *ChatPane {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	svc := hitlService(rt)

	return &ChatPane{
		feed:         NewFeed(),
		spinner:      sp,
		runStates:    make(map[string]*RunState),
		context:      ctx,
		session:      sess,
		notifQ:       notifQ,
		hitlSvc:      svc,
		runtime:      rt,
		expandTarget: "thinking",
	}
}

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
	p.feed.SetSize(w, h)
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

	tokenCount := 0
	if summary := summarizeResult(result); summary != "" {
		tokenCount = estimateTokens(summary)
	}
	sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: tokenCount, Result: result})
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
	run.Builder.SetResult(structuredResultFromCore(msg.Result))
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
