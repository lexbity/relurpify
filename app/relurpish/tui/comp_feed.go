package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Feed wraps a viewport and owns the message list plus render logic.
// It is always held by pointer to avoid Bubble Tea value-copy pitfalls.
type Feed struct {
	vp          viewport.Model
	messages    []Message
	spinnerView string
	searchQuery string
	autoFollow  bool
	ready       bool
}

// NewFeed creates an empty Feed.
func NewFeed() *Feed {
	return &Feed{
		autoFollow: true,
	}
}

// SetSize resizes the feed viewport.
func (f *Feed) SetSize(w, h int) {
	if !f.ready {
		f.vp = viewport.New(w, h)
		f.ready = true
	} else {
		f.vp.Width = w
		f.vp.Height = h
	}
	f.refresh()
}

// SetSpinner updates the spinner view string used while streaming.
func (f *Feed) SetSpinner(s string) {
	f.spinnerView = s
}

// Messages returns a copy of the current message list.
func (f *Feed) Messages() []Message {
	out := make([]Message, len(f.messages))
	copy(out, f.messages)
	return out
}

// AppendMessage adds a message to the end of the feed.
func (f *Feed) AppendMessage(msg Message) {
	f.messages = append(f.messages, msg)
	f.refresh()
}

// UpdateMessage upserts a message by ID (append if not found).
func (f *Feed) UpdateMessage(msg Message) {
	if msg.ID == "" {
		f.messages = append(f.messages, msg)
		f.refresh()
		return
	}
	for i := len(f.messages) - 1; i >= 0; i-- {
		if f.messages[i].ID == msg.ID {
			f.messages[i] = msg
			f.refresh()
			return
		}
	}
	f.messages = append(f.messages, msg)
	f.refresh()
}

// ClearMessages removes all messages from the feed.
func (f *Feed) ClearMessages() {
	f.messages = nil
	f.refresh()
}

// SetSearchFilter applies a live text filter to the feed display.
// Pass an empty string to clear the filter and show all messages.
func (f *Feed) SetSearchFilter(query string) {
	f.searchQuery = query
	f.refresh()
}

// FilterMessages returns messages whose text content contains the query.
func (f *Feed) FilterMessages(query string) []Message {
	if query == "" {
		return f.Messages()
	}
	q := strings.ToLower(query)
	var out []Message
	for _, m := range f.messages {
		if strings.Contains(strings.ToLower(m.Content.Text), q) {
			out = append(out, m)
		}
	}
	return out
}

// Update passes viewport scroll events through.
func (f *Feed) Update(msg tea.Msg) (*Feed, tea.Cmd) {
	if !f.ready {
		return f, nil
	}
	var cmd tea.Cmd
	f.vp, cmd = f.vp.Update(msg)
	f.autoFollow = f.vp.AtBottom()
	return f, cmd
}

// View renders the viewport content.
func (f *Feed) View() string {
	if !f.ready {
		return ""
	}
	return f.vp.View()
}

func (f *Feed) refresh() {
	if !f.ready {
		return
	}
	f.vp.SetContent(f.renderAll())
	if f.autoFollow {
		f.vp.GotoBottom()
	}
}

func (f *Feed) renderAll() string {
	msgs := f.messages
	if f.searchQuery != "" {
		msgs = f.FilterMessages(f.searchQuery)
		if len(msgs) == 0 {
			return dimStyle.Render(fmt.Sprintf("No messages matching %q", f.searchQuery))
		}
	} else if len(msgs) == 0 {
		return welcomeStyle.Render("Welcome! Type a message or /help for commands.")
	}
	parts := make([]string, 0, len(msgs))
	spinner := f.spinnerView
	for _, msg := range msgs {
		parts = append(parts, RenderMessage(msg, f.vp.Width, spinner))
	}
	return strings.Join(parts, "\n")
}

// RenderMessage converts a Message into a styled string for display.
// This is the canonical render function (moved from render.go).
func RenderMessage(msg Message, width int, spinnerView string) string {
	var b strings.Builder
	b.WriteString(renderMsgHeader(msg))
	b.WriteString("\n")
	switch msg.Role {
	case RoleUser:
		b.WriteString(textStyle.Render(msg.Content.Text))
	case RoleAgent:
		b.WriteString(renderAgentContent(msg, width, spinnerView))
	case RoleSystem:
		b.WriteString(dimStyle.Render(msg.Content.Text))
	}
	if msg.Metadata.Duration > 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("⏱  %s | %d tok", formatDur(msg.Metadata.Duration), msg.Metadata.TokensUsed)))
	}
	boxW := max(0, width-4)
	return messageBoxStyle.Width(boxW).Render(b.String())
}

func renderMsgHeader(msg Message) string {
	ts := msg.Timestamp.Format("15:04:05")
	icon, role := "💬", "User"
	switch msg.Role {
	case RoleUser:
		icon, role = "👤", "You"
	case RoleAgent:
		icon, role = "🤖", "Agent"
	case RoleSystem:
		icon, role = "⚙", "System"
	}
	return headerStyle.Render(fmt.Sprintf("%s [%s] %s", icon, ts, role))
}

func renderAgentContent(msg Message, width int, spinnerView string) string {
	var b strings.Builder
	if len(msg.Content.Thinking) > 0 {
		b.WriteString(renderThinkingBlock(msg.Content.Thinking, msg.Content.Expanded["thinking"], spinnerView))
		b.WriteString("\n\n")
	}
	if msg.Content.Plan != nil {
		b.WriteString(renderPlanBlock(msg.Content.Plan, msg.Content.Expanded["plan"], spinnerView))
		b.WriteString("\n\n")
	}
	if len(msg.Content.Changes) > 0 {
		b.WriteString(renderChangesBlock(msg.Content.Changes, msg.Content.Expanded["changes"], width))
		b.WriteString("\n\n")
	}
	if msg.Content.Text != "" {
		b.WriteString(textStyle.Render(msg.Content.Text))
	}
	if msg.Content.Result != nil {
		if msg.Content.Text != "" {
			b.WriteString("\n\n")
		}
		b.WriteString(renderStructuredResultBlock(msg.Content.Result, width))
	}
	return b.String()
}

func renderStructuredResultBlock(result *StructuredResult, width int) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	status := "failed"
	if result.Success {
		status = "ok"
	}
	nodeID := result.NodeID
	if nodeID == "" {
		nodeID = "unknown"
	}
	headerBits := []string{"node=" + nodeID, "status=" + status}
	if result.Envelope != nil && result.Envelope.CapabilityName != "" {
		headerBits = append(headerBits, "capability="+result.Envelope.CapabilityName)
	}
	b.WriteString(sectionHeaderStyle.Render("🧾 Result"))
	b.WriteString("\n")
	b.WriteString(detailStyle.Render(strings.Join(headerBits, " | ")))
	if result.Envelope != nil {
		b.WriteString("\n")
		b.WriteString(renderResultEnvelope(result.Envelope, width))
	}
	if result.ErrorText != "" {
		b.WriteString("\n")
		b.WriteString(diffRemoveStyle.Render("error: " + result.ErrorText))
	}
	return b.String()
}

func renderResultEnvelope(envelope *StructuredResultEnvelope, width int) string {
	if envelope == nil {
		return ""
	}
	var parts []string
	if envelope.CapabilityID != "" {
		parts = append(parts, "id="+envelope.CapabilityID)
	}
	if envelope.TrustClass != "" {
		parts = append(parts, "trust="+envelope.TrustClass)
	}
	if envelope.Disposition != "" {
		parts = append(parts, "disposition="+envelope.Disposition)
	}
	if envelope.Insertion.Action != "" {
		parts = append(parts, "insertion="+envelope.Insertion.Action)
	}
	var b strings.Builder
	if len(parts) > 0 {
		b.WriteString(dimStyle.Render(strings.Join(parts, " | ")))
	}
	if envelope.Insertion.Reason != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(detailStyle.Render("reason: " + envelope.Insertion.Reason))
	}
	if envelope.Approval != nil {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderApprovalBinding(envelope.Approval))
	}
	if len(envelope.Blocks) > 0 {
		for _, block := range envelope.Blocks {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(renderStructuredContentBlock(block, width))
		}
	}
	return b.String()
}

func renderApprovalBinding(approval *StructuredApprovalBinding) string {
	if approval == nil {
		return ""
	}
	fields := make([]string, 0, 6)
	if approval.ProviderID != "" {
		fields = append(fields, "provider="+approval.ProviderID)
	}
	if approval.SessionID != "" {
		fields = append(fields, "session="+approval.SessionID)
	}
	if approval.TargetResource != "" {
		fields = append(fields, "target="+approval.TargetResource)
	}
	if approval.WorkflowID != "" {
		fields = append(fields, "workflow="+approval.WorkflowID)
	}
	if approval.TaskID != "" {
		fields = append(fields, "task="+approval.TaskID)
	}
	if len(approval.EffectClasses) > 0 {
		fields = append(fields, "effects="+strings.Join(approval.EffectClasses, ","))
	}
	return detailStyle.Render("approval: " + strings.Join(fields, " | "))
}

func renderStructuredContentBlock(block StructuredContentBlock, width int) string {
	var b strings.Builder
	title := block.Type
	if block.Summary != "" {
		title = block.Summary
	}
	b.WriteString(detailStyle.Render("[" + block.Type + "] " + title))
	if block.Body != "" {
		body := strings.TrimSpace(block.Body)
		if block.Type == "structured" || block.Type == "embedded-resource" {
			body = indentStructuredBody(body, max(20, width-10))
		}
		b.WriteString("\n")
		b.WriteString(textStyle.Render(body))
	}
	if len(block.Provenance) > 0 {
		pairs := make([]string, 0, len(block.Provenance))
		for _, key := range []string{"capability", "provider", "trust", "disposition"} {
			if value := block.Provenance[key]; value != "" {
				pairs = append(pairs, key+"="+value)
			}
		}
		if len(pairs) > 0 {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render(strings.Join(pairs, " | ")))
		}
	}
	return b.String()
}

func indentStructuredBody(body string, _ int) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	var pretty any
	if err := json.Unmarshal([]byte(body), &pretty); err == nil {
		if data, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			return string(data)
		}
	}
	return body
}

func renderThinkingBlock(steps []ThinkingStep, expanded bool, spinnerView string) string {
	var b strings.Builder
	toggle := "[−]"
	if !expanded {
		toggle = "[+]"
	}
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("🤔 Thinking %s", dimStyle.Render(toggle))))
	b.WriteString("\n")
	if !expanded {
		b.WriteString(dimStyle.Render(fmt.Sprintf("%d steps", len(steps))))
		return b.String()
	}
	for i, step := range steps {
		isLast := i == len(steps)-1
		prefix := "├─"
		if isLast {
			prefix = "└─"
		}
		icon := stepIcon(step.Type)
		if isLast && step.EndTime.IsZero() {
			icon = spinnerView
		}
		dur := ""
		if !step.EndTime.IsZero() {
			dur = dimStyle.Render(fmt.Sprintf(" (%s)", formatDur(step.EndTime.Sub(step.StartTime))))
		}
		b.WriteString(fmt.Sprintf("%s %s %s%s\n", dimStyle.Render(prefix), icon, step.Description, dur))
		for _, d := range step.Details {
			sub := "│ "
			if isLast {
				sub = "  "
			}
			b.WriteString(dimStyle.Render(sub) + "  " + detailStyle.Render(d) + "\n")
		}
	}
	return b.String()
}

func stepIcon(t StepType) string {
	switch t {
	case StepAnalyzing:
		return "🔍"
	case StepPlanning:
		return "💭"
	case StepCoding:
		return "✏"
	case StepTesting:
		return "🧪"
	default:
		return "•"
	}
}

func renderPlanBlock(plan *TaskPlan, expanded bool, spinnerView string) string {
	var b strings.Builder
	done := 0
	for _, t := range plan.Tasks {
		if t.Status == TaskCompleted {
			done++
		}
	}
	toggle := "[−]"
	if !expanded {
		toggle = "[+]"
	}
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("💡 Plan (%d/%d) %s", done, len(plan.Tasks), dimStyle.Render(toggle))))
	b.WriteString("\n")
	if !expanded {
		return b.String()
	}
	for _, t := range plan.Tasks {
		var icon string
		var style lipgloss.Style
		switch t.Status {
		case TaskCompleted:
			icon, style = "✅", completedStyle
		case TaskInProgress:
			icon, style = spinnerView, inProgressStyle
		default:
			icon, style = "☐", pendingStyle
		}
		dur := ""
		if t.Status == TaskCompleted && !t.EndTime.IsZero() {
			dur = dimStyle.Render(fmt.Sprintf(" (%s)", formatDur(t.EndTime.Sub(t.StartTime))))
		}
		b.WriteString(fmt.Sprintf("%s %s%s\n", icon, style.Render(t.Description), dur))
	}
	return b.String()
}

func renderChangesBlock(changes []FileChange, expanded bool, width int) string {
	var b strings.Builder
	added, removed := 0, 0
	for _, c := range changes {
		added += c.LinesAdded
		removed += c.LinesRemoved
	}
	toggle := "[−]"
	if !expanded {
		toggle = "[+]"
	}
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("✏  Changes (%d files, +%d -%d) %s", len(changes), added, removed, dimStyle.Render(toggle))))
	b.WriteString("\n")
	for i, c := range changes {
		if i > 0 && expanded {
			b.WriteString("\n")
		}
		if expanded {
			b.WriteString(renderChangeFull(c, width))
		} else {
			b.WriteString(renderChangeCompact(c) + "\n")
		}
	}
	if expanded {
		pending := false
		for _, c := range changes {
			if c.Status == StatusPending {
				pending = true
				break
			}
		}
		if pending {
			b.WriteString("\n")
			b.WriteString(buttonStyle.Render("/approve") + "  " + buttonStyle.Render("/reject"))
		}
	}
	return b.String()
}

func renderChangeCompact(c FileChange) string {
	icon := "~"
	switch c.Type {
	case ChangeCreate:
		icon = "+"
	case ChangeDelete:
		icon = "-"
	}
	statusIcon := "🟡"
	switch c.Status {
	case StatusApproved:
		statusIcon = "✅"
	case StatusRejected:
		statusIcon = "❌"
	}
	return fmt.Sprintf("%s %s %s %s", statusIcon, filePathStyle.Render(c.Path), dimStyle.Render(icon), dimStyle.Render(fmt.Sprintf("+%d -%d", c.LinesAdded, c.LinesRemoved)))
}

func renderChangeFull(c FileChange, width int) string {
	var b strings.Builder
	b.WriteString(renderChangeCompact(c))
	b.WriteString("\n")
	if c.Expanded {
		b.WriteString(diffBoxStyle.Width(max(0, width-6)).Render(renderDiffText(c.Diff)))
	}
	return b.String()
}

func renderDiffText(diff string) string {
	lines := strings.Split(diff, "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			rendered = append(rendered, "")
			continue
		}
		style := diffContextStyle
		switch line[0] {
		case '+':
			style = diffAddStyle
		case '-':
			style = diffRemoveStyle
		case '@':
			style = diffHeaderStyle
		}
		rendered = append(rendered, style.Render(line))
	}
	return strings.Join(rendered, "\n")
}

func formatDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}
