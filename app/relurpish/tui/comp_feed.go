package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
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
	th          *theme.Theme
}

// NewFeed creates an empty Feed.
func NewFeed() *Feed {
	return &Feed{
		autoFollow: true,
		th:         theme.Default(),
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

// SetTheme sets the active semantic style source for the feed.
func (f *Feed) SetTheme(th *theme.Theme) {
	if th != nil {
		f.th = th
	}
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

// Mutate exposes the underlying message slice to controlled in-place updates.
// Callers must refresh any derived state through the provided function.
func (f *Feed) Mutate(fn func(msgs []Message)) {
	if fn == nil {
		return
	}
	fn(f.messages)
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

// ScrollUp scrolls the feed up by one line.
func (f *Feed) ScrollUp() {
	if f.ready {
		f.vp.ScrollUp(1)
		f.autoFollow = f.vp.AtBottom()
	}
}

// ScrollDown scrolls the feed down by one line.
func (f *Feed) ScrollDown() {
	if f.ready {
		f.vp.ScrollDown(1)
		f.autoFollow = f.vp.AtBottom()
	}
}

// PageUp scrolls the feed up by one page.
func (f *Feed) PageUp() {
	if f.ready {
		f.vp.PageUp()
		f.autoFollow = f.vp.AtBottom()
	}
}

// PageDown scrolls the feed down by one page.
func (f *Feed) PageDown() {
	if f.ready {
		f.vp.PageDown()
		f.autoFollow = f.vp.AtBottom()
	}
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
			return f.th.Dim().Render(fmt.Sprintf("No messages matching %q", f.searchQuery))
		}
	} else if len(msgs) == 0 {
		return f.th.Detail().Render("Welcome! Type a message or /help for commands.")
	}
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, RenderMessage(f.th, msg, f.vp.Width, f.spinnerView))
	}
	return strings.Join(parts, "\n")
}

// RenderMessage converts a Message into a styled string for display.
func RenderMessage(th *theme.Theme, msg Message, width int, spinnerView string) string {
	var b strings.Builder
	b.WriteString(renderMsgHeader(th, msg))
	b.WriteString("\n")
	switch msg.Role {
	case RoleUser:
		b.WriteString(th.Body().Render(msg.Content.Text))
	case RoleAgent:
		b.WriteString(renderAgentContent(th, msg, width, spinnerView))
	case RoleSystem:
		b.WriteString(th.Dim().Render(msg.Content.Text))
	}
	if msg.Metadata.Duration > 0 {
		b.WriteString("\n")
		b.WriteString(th.Dim().Render(fmt.Sprintf("⏱  %s | %d tok", formatDur(msg.Metadata.Duration), msg.Metadata.TokensUsed)))
	}
	boxW := max(0, width-4)
	return th.Panel().Width(boxW).Render(b.String())
}

func renderMsgHeader(th *theme.Theme, msg Message) string {
	ts := formatMessageTimestamp(msg.Timestamp)
	icon, role := "💬", "User"
	switch msg.Role {
	case RoleUser:
		icon, role = "👤", "You"
	case RoleAgent:
		icon, role = "🤖", "Agent"
	case RoleSystem:
		icon, role = "⚙", "System"
	}
	return th.Header().Render(fmt.Sprintf("%s [%s] %s", icon, ts, role))
}

func formatMessageTimestamp(t time.Time) string {
	now := time.Now()
	sameDay := t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day()
	if sameDay {
		return t.Format("15:04")
	}
	return t.Format("Jan 02 15:04")
}

func renderAgentContent(th *theme.Theme, msg Message, width int, spinnerView string) string {
	var b strings.Builder
	if len(msg.Content.Thinking) > 0 {
		b.WriteString(renderThinkingBlock(th, msg.Content.Thinking, msg.Content.Expanded["thinking"], spinnerView))
		b.WriteString("\n\n")
	}
	if msg.Content.Plan != nil {
		b.WriteString(renderPlanBlock(th, msg.Content.Plan, msg.Content.Expanded["plan"], spinnerView))
		b.WriteString("\n\n")
	}
	if len(msg.Content.Changes) > 0 {
		b.WriteString(renderChangesBlock(th, msg.Content.Changes, msg.Content.Expanded["changes"], width))
		b.WriteString("\n\n")
	}
	if msg.Content.Text != "" {
		b.WriteString(th.Body().Render(msg.Content.Text))
	}
	if msg.Content.Result != nil {
		if msg.Content.Text != "" {
			b.WriteString("\n\n")
		}
		b.WriteString(renderStructuredResultBlock(th, msg.Content.Result, width))
	}
	return b.String()
}

func renderStructuredResultBlock(th *theme.Theme, result *StructuredResult, width int) string {
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
	b.WriteString(th.Subhead().Render("🧾 Result"))
	b.WriteString("\n")
	b.WriteString(th.Detail().Render(strings.Join(headerBits, " | ")))
	if result.Envelope != nil {
		b.WriteString("\n")
		b.WriteString(renderResultEnvelope(th, result.Envelope, width))
	}
	if result.ErrorText != "" {
		b.WriteString("\n")
		b.WriteString(th.Error().Render("error: " + result.ErrorText))
	}
	return b.String()
}

func renderResultEnvelope(th *theme.Theme, envelope *StructuredResultEnvelope, width int) string {
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
		b.WriteString(th.Dim().Render(strings.Join(parts, " | ")))
	}
	if envelope.Insertion.Reason != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(th.Detail().Render("reason: " + envelope.Insertion.Reason))
	}
	if envelope.Approval != nil {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderApprovalBinding(th, envelope.Approval))
	}
	if len(envelope.Blocks) > 0 {
		for _, block := range envelope.Blocks {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(renderStructuredContentBlock(th, block, width))
		}
	}
	return b.String()
}

func renderApprovalBinding(th *theme.Theme, approval *StructuredApprovalBinding) string {
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
	return th.Detail().Render("approval: " + strings.Join(fields, " | "))
}

func renderStructuredContentBlock(th *theme.Theme, block StructuredContentBlock, width int) string {
	var b strings.Builder
	title := block.Type
	if block.Summary != "" {
		title = block.Summary
	}
	b.WriteString(th.Detail().Render("[" + block.Type + "] " + title))
	if block.Body != "" {
		body := strings.TrimSpace(block.Body)
		if block.Type == "structured" || block.Type == "embedded-resource" {
			body = indentStructuredBody(body, max(20, width-10))
		}
		b.WriteString("\n")
		b.WriteString(th.Body().Render(body))
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
			b.WriteString(th.Dim().Render(strings.Join(pairs, " | ")))
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

func renderThinkingBlock(th *theme.Theme, steps []ThinkingStep, expanded bool, spinnerView string) string {
	var b strings.Builder
	toggle := "[−]"
	if !expanded {
		toggle = "[+]"
	}
	b.WriteString(th.Subhead().Render(fmt.Sprintf("🤔 Thinking %s", th.Dim().Render(toggle))))
	b.WriteString("\n")
	if !expanded {
		b.WriteString(th.Dim().Render(fmt.Sprintf("%d steps", len(steps))))
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
			dur = th.Dim().Render(fmt.Sprintf(" (%s)", formatDur(step.EndTime.Sub(step.StartTime))))
		}
		fmt.Fprintf(&b, "%s %s %s%s\n", th.Dim().Render(prefix), icon, step.Description, dur)
		for _, d := range step.Details {
			sub := "│ "
			if isLast {
				sub = "  "
			}
			b.WriteString(th.Dim().Render(sub) + "  " + th.Detail().Render(d) + "\n")
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

func renderPlanBlock(th *theme.Theme, plan *TaskPlan, expanded bool, spinnerView string) string {
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
	b.WriteString(th.Subhead().Render(fmt.Sprintf("💡 Plan (%d/%d) %s", done, len(plan.Tasks), th.Dim().Render(toggle))))
	b.WriteString("\n")
	if !expanded {
		return b.String()
	}
	for _, t := range plan.Tasks {
		var icon string
		var style lipgloss.Style
		switch t.Status {
		case TaskCompleted:
			icon, style = "✅", th.Success()
		case TaskInProgress:
			icon, style = spinnerView, th.Warning()
		default:
			icon, style = "☐", th.Pending()
		}
		dur := ""
		if t.Status == TaskCompleted && !t.EndTime.IsZero() {
			dur = th.Dim().Render(fmt.Sprintf(" (%s)", formatDur(t.EndTime.Sub(t.StartTime))))
		}
		fmt.Fprintf(&b, "%s %s%s\n", icon, style.Render(t.Description), dur)
	}
	return b.String()
}

func renderChangesBlock(th *theme.Theme, changes []FileChange, expanded bool, width int) string {
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
	b.WriteString(th.Subhead().Render(fmt.Sprintf("✏  Changes (%d files, +%d -%d) %s", len(changes), added, removed, th.Dim().Render(toggle))))
	b.WriteString("\n")
	for i, c := range changes {
		if i > 0 && expanded {
			b.WriteString("\n")
		}
		if expanded {
			b.WriteString(renderChangeFull(th, c, width))
		} else {
			b.WriteString(renderChangeCompact(th, c) + "\n")
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
			b.WriteString(th.Button(true).Render("/approve") + "  " + th.Button(true).Render("/reject"))
		}
	}
	return b.String()
}

func renderChangeCompact(th *theme.Theme, c FileChange) string {
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
	return fmt.Sprintf("%s %s %s %s", statusIcon, th.Subhead().Render(c.Path), th.Dim().Render(icon), th.Dim().Render(fmt.Sprintf("+%d -%d", c.LinesAdded, c.LinesRemoved)))
}

func renderChangeFull(th *theme.Theme, c FileChange, width int) string {
	var b strings.Builder
	b.WriteString(renderChangeCompact(th, c))
	b.WriteString("\n")
	if c.Expanded {
		b.WriteString(th.Box().Width(max(0, width-6)).Render(renderDiffText(th, c.Diff)))
	}
	return b.String()
}

func renderDiffText(th *theme.Theme, diff string) string {
	lines := strings.Split(diff, "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			rendered = append(rendered, "")
			continue
		}
		style := th.Body()
		switch line[0] {
		case '+':
			style = th.Success()
		case '-':
			style = th.Error()
		case '@':
			style = th.Subhead()
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
