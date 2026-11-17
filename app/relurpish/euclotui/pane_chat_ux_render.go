package euclotui

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func formatChatDur(d time.Duration) string {
	if d < time.Second {
		return d.Truncate(10 * time.Millisecond).String()
	}
	if d < time.Minute {
		return d.Truncate(100 * time.Millisecond).String()
	}
	return d.Truncate(time.Second).String()
}

func (p *ChatPane) onMessageAdded() {
	if p.ux == nil || !p.ux.vpReady {
		return
	}
	if !p.ux.autoFollow {
		p.ux.newCount++
	}
	p.syncUXViewport(true)
}

func (p *ChatPane) hasStreamingState() bool {
	if p.ux == nil {
		return p.HasActiveRuns()
	}
	if !p.HasActiveRuns() {
		return false
	}
	// Thinking/active state until some output text has appeared for active runs.
	for _, run := range p.listRuns() {
		if run == nil {
			continue
		}
		if run.builder != nil {
			partial := run.builder.BuildPartial()
			if strings.TrimSpace(partial.Content.Text) == "" && len(p.ux.streamBuffers[run.id].pendingOrShown()) == 0 {
				return true
			}
		}
	}
	// fallback: active runs imply work in progress
	return false
}

func (b *streamBuffer) pendingOrShown() string {
	if b == nil {
		return ""
	}
	if b.shown.Len() > 0 || len(b.pending) > 0 {
		return "x"
	}
	return ""
}

func (p *ChatPane) needsAnimTick() bool {
	if p.ux == nil {
		return false
	}
	if p.HasActiveRuns() {
		return true
	}
	if p.ux.thinkingActive {
		return true
	}
	for _, b := range p.ux.streamBuffers {
		if b != nil && len(b.pending) > 0 {
			return true
		}
	}
	for _, c := range p.ux.toolCards {
		if c.Anim < c.Target-0.001 || c.Anim > c.Target+0.001 || c.AnimVel != 0 || c.State == toolRunning {
			return true
		}
	}
	return false
}

func (p *ChatPane) syncUXViewport(forceBottom bool) {
	if p.ux == nil || !p.ux.vpReady {
		return
	}
	atBottom := p.ux.autoFollow || p.ux.vp.AtBottom()
	content := p.renderMessagesContent()
	p.ux.vp.SetContent(content)
	if p.ux.autoFollow || forceBottom && atBottom {
		if p.ux.autoFollow {
			p.ux.vp.GotoBottom()
			p.ux.newCount = 0
		}
	}
}

func (p *ChatPane) renderChatBody() string {
	if p.ux == nil || !p.ux.vpReady {
		return p.feed.View()
	}
	p.syncUXViewport(false)
	view := p.ux.vp.View()
	if p.ux.permModal != nil {
		return p.ux.renderPermissionModal(p.ux.width, p.ux.height)
	}
	pill := p.ux.newPill()
	if pill == "" {
		return view
	}
	// overlay "↓ new" pill near bottom-right
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return pill
	}
	row := len(lines) - 2
	if row < 0 {
		row = len(lines) - 1
	}
	line := lines[row]
	w := lipgloss.Width(line)
	pw := lipgloss.Width(pill)
	plain := ansi.Strip(line)
	_ = plain
	if w > pw+1 {
		// place pill at right side
		pad := w - pw
		if pad < 0 {
			pad = 0
		}
		lines[row] = strings.Repeat(" ", pad) + pill
	} else {
		lines[row] = pill
	}
	return strings.Join(lines, "\n")
}

func (p *ChatPane) renderMessagesContent() string {
	msgs := p.feed.Messages()
	if p.searchQuery != "" {
		msgs = p.feed.FilterMessages(p.searchQuery)
		if len(msgs) == 0 {
			return p.th.Dim().Render(fmt.Sprintf("No messages matching %q", p.searchQuery))
		}
	}
	if len(msgs) == 0 && !p.HasActiveRuns() && (p.ux == nil || len(p.ux.toolCards) == 0) {
		return p.renderWelcome()
	}
	parts := make([]string, 0, len(msgs)+4)
	for _, msg := range msgs {
		parts = append(parts, p.renderChatMessage(msg))
	}
	// active streaming/thinking indicators for runs
	if p.HasActiveRuns() && p.ux != nil {
		if thinking := p.ux.thinkingView(p.hasStreamingState(), p.spinner.View()); thinking != "" {
			parts = append(parts, p.renderBlock("euclo", thinking, true))
		}
	}
	if p.ux != nil {
		if cards := p.ux.renderToolCards(max(20, p.ux.width-2)); cards != "" {
			parts = append(parts, cards)
		}
	}
	if len(parts) == 0 {
		return p.renderWelcome()
	}
	return strings.Join(parts, "\n\n")
}

func (p *ChatPane) renderWelcome() string {
	bg := p.ux.styles.bg
	title := lipgloss.NewStyle().Foreground(p.ux.styles.cyan).Background(bg).Bold(true).Render("euclo")
	sub := lipgloss.NewStyle().Foreground(p.ux.styles.muted).Background(bg).Render("calm · dense · fast · alive")
	hint := lipgloss.NewStyle().Foreground(p.ux.styles.fg).Background(bg).Render("Type a request, or /help for commands.")
	return lipgloss.JoinVertical(lipgloss.Left, title, sub, "", hint)
}

func (p *ChatPane) renderChatMessage(msg tui.Message) string {
	label := "system"
	accent := p.ux.styles.muted
	switch msg.Role {
	case tui.RoleUser:
		label = "you"
		accent = p.ux.styles.green
	case tui.RoleAgent:
		label = "euclo"
		accent = p.ux.styles.purple
	}
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(label)
	body := p.renderMessageBody(msg)
	meta := ""
	if msg.Metadata.Duration > 0 {
		meta = lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render(fmt.Sprintf("⏱  %s · %d tok", formatChatDur(msg.Metadata.Duration), msg.Metadata.TokensUsed))
		body += "\n" + meta
	}
	boxW := max(20, p.ux.width-4)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Foreground(p.ux.styles.fg).
		Background(p.ux.styles.surface).
		Padding(0, 1).
		Width(boxW)
	return card.Render(header + "\n" + body)
}

func (p *ChatPane) renderBlock(label, body string, agent bool) string {
	accent := p.ux.styles.muted
	if agent {
		accent = p.ux.styles.purple
	}
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(label)
	boxW := max(20, p.ux.width-4)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Foreground(p.ux.styles.fg).
		Background(p.ux.styles.surface).
		Padding(0, 1).
		Width(boxW)
	return card.Render(header + "\n" + body)
}

func (p *ChatPane) renderMessageBody(msg tui.Message) string {
	switch msg.Role {
	case tui.RoleUser:
		return lipgloss.NewStyle().Foreground(p.ux.styles.fg).Background(p.ux.styles.surface).Render(msg.Content.Text)
	case tui.RoleSystem:
		return lipgloss.NewStyle().Foreground(p.ux.styles.muted).Background(p.ux.styles.surface).Render(msg.Content.Text)
	case tui.RoleAgent:
		return p.renderAgentBody(msg)
	default:
		return msg.Content.Text
	}
}

func (p *ChatPane) renderAgentBody(msg tui.Message) string {
	var sections []string
	// thinking steps
	if len(msg.Content.Thinking) > 0 {
		sections = append(sections, p.renderThinkingSteps(msg.Content.Thinking, msg.Content.Expanded["thinking"]))
	}
	// plan/changes rendered compactly if present
	if msg.Content.Plan != nil && len(msg.Content.Plan.Tasks) > 0 {
		sections = append(sections, p.renderPlan(msg.Content.Plan))
	}
	if len(msg.Content.Changes) > 0 {
		sections = append(sections, p.renderChanges(msg.Content.Changes))
	}
	text := msg.Content.Text
	if buf, ok := p.ux.streamBuffers[msg.ID]; ok && buf != nil {
		// Ensure any buffered pending text is visible even if animation ticks stall.
		if len(buf.pending) > 0 {
			buf.shown.WriteString(string(buf.pending))
			buf.pending = nil
		}
		shown := buf.shown.String()
		if strings.TrimSpace(text) == "" && shown != "" {
			text = shown
		} else if shown != "" && len(shown) > len(text) {
			text = shown
		}
		if buf.cursor && p.HasActiveRuns() {
			cursor := "█"
			if !p.ux.cursorOn {
				cursor = " "
			}
			// block cursor at end of streaming text
			if strings.TrimSpace(text) != "" || shown != "" || len(buf.pending) > 0 {
				rendered := p.renderAssistantText(text)
				sections = append(sections, rendered+lipgloss.NewStyle().Foreground(p.ux.styles.cyan).Background(p.ux.styles.surface).Render(cursor))
				return strings.Join(nonEmpty(sections), "\n\n")
			}
		}
	}
	if strings.TrimSpace(text) != "" {
		sections = append(sections, p.renderAssistantText(text))
	} else if len(msg.Content.Thinking) == 0 && msg.Content.Plan == nil && len(msg.Content.Changes) == 0 && msg.Content.Result == nil {
		// empty agent placeholder
	}
	if msg.Content.Result != nil {
		sections = append(sections, p.renderResult(msg.Content.Result))
	}
	return strings.Join(nonEmpty(sections), "\n\n")
}

func (p *ChatPane) renderAssistantText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	// Try glamour markdown rendering for assistant messages.
	if rendered := p.ux.renderMarkdown(text); strings.TrimSpace(rendered) != "" {
		return rendered
	}
	return lipgloss.NewStyle().Foreground(p.ux.styles.fg).Background(p.ux.styles.surface).Render(text)
}

func (p *ChatPane) renderThinkingSteps(steps []tui.ThinkingStep, expanded bool) string {
	if len(steps) == 0 {
		return ""
	}
	var b strings.Builder
	head := lipgloss.NewStyle().Foreground(p.ux.styles.cyan).Bold(true).Render("thinking")
	b.WriteString(head)
	if !expanded {
		b.WriteString(lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render(fmt.Sprintf(" · %d steps", len(steps))))
		return b.String()
	}
	b.WriteString("\n")
	for i, step := range steps {
		icon := "•"
		switch step.Type {
		case tui.StepAnalyzing:
			icon = "◎"
		case tui.StepPlanning:
			icon = "◇"
		case tui.StepCoding:
			icon = "✎"
		case tui.StepTesting:
			icon = "✓"
		}
		active := step.EndTime.IsZero()
		line := fmt.Sprintf("%s %s", icon, step.Description)
		if active {
			// shimmering "thinking…" feel for active step
			line = p.ux.thinkingView(true, p.spinner.View()) + lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render(" "+step.Description)
		} else if !step.StartTime.IsZero() {
			dur := step.EndTime.Sub(step.StartTime).Truncate(10 * time.Millisecond)
			line += lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render(fmt.Sprintf(" (%s)", dur))
		}
		b.WriteString("  " + line)
		if i < len(steps)-1 {
			b.WriteString("\n")
		}
		for _, d := range step.Details {
			if strings.TrimSpace(d) == "" {
				continue
			}
			b.WriteString("\n    " + lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render(d))
		}
	}
	return b.String()
}

func (p *ChatPane) renderPlan(plan *tui.TaskPlan) string {
	if plan == nil {
		return ""
	}
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(p.ux.styles.orange).Bold(true).Render("plan"))
	for _, task := range plan.Tasks {
		mark := "○"
		style := lipgloss.NewStyle().Foreground(p.ux.styles.muted)
		switch task.Status {
		case tui.TaskCompleted:
			mark = "✓"
			style = lipgloss.NewStyle().Foreground(p.ux.styles.green)
		case tui.TaskInProgress:
			mark = spinnerFrames[p.spinnerIdx%len(spinnerFrames)]
			style = lipgloss.NewStyle().Foreground(p.ux.styles.cyan)
		}
		lines = append(lines, "  "+style.Render(mark+" "+task.Description))
	}
	return strings.Join(lines, "\n")
}

func (p *ChatPane) renderChanges(changes []tui.FileChange) string {
	if len(changes) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(p.ux.styles.pink).Bold(true).Render("changes"))
	for _, c := range changes {
		lines = append(lines, fmt.Sprintf("  %s +%d -%d", c.Path, c.LinesAdded, c.LinesRemoved))
	}
	return strings.Join(lines, "\n")
}

func (p *ChatPane) renderResult(result *tui.StructuredResult) string {
	if result == nil {
		return ""
	}
	status := lipgloss.NewStyle().Foreground(p.ux.styles.green).Render("done")
	if !result.Success {
		status = lipgloss.NewStyle().Foreground(p.ux.styles.red).Render("error")
	}
	out := lipgloss.NewStyle().Foreground(p.ux.styles.muted).Render("result: ") + status
	if result.ErrorText != "" {
		out += "\n" + lipgloss.NewStyle().Foreground(p.ux.styles.red).Render(result.ErrorText)
	}
	return out
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
