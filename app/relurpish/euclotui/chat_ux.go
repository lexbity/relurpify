package euclotui

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

// chatUXState holds enhanced rendering/interaction state for the Euclo chat.
type chatUXState struct {
	vp          viewport.Model
	vpReady     bool
	autoFollow  bool
	newCount    int
	width       int
	height      int
	mdRenderer  *glamour.TermRenderer
	mdWidth     int
	styles      chatPalette
	th          *theme.Theme
	shimmerTick int
	cursorOn    bool
	lastAnim    time.Time

	// typewriter streaming per run
	streamBuffers map[string]*streamBuffer
	// thinking animation when run active but no visible output yet
	thinkingActive bool

	// tool-call cards
	toolCards    []toolCallCard
	toolExpanded map[string]bool
	toolSel      int
	toolFocus    bool

	// permission modal
	permModal *permissionModal
}

type streamBuffer struct {
	pending []rune
	shown   strings.Builder
	cursor  bool
	last    time.Time
}

type toolCardState string

const (
	toolPending toolCardState = "pending"
	toolRunning toolCardState = "running"
	toolDone    toolCardState = "done"
	toolError   toolCardState = "error"
)

type toolCallCard struct {
	ID       string
	Name     string
	Args     string
	Output   string
	Err      string
	State    toolCardState
	Started  time.Time
	Ended    time.Time
	Expanded bool
	Anim     float64
	AnimVel  float64
	Target   float64
}

type permModalSource string

const (
	permModalSourceAuthHITL    permModalSource = "auth_hitl"
	permModalSourceInteraction permModalSource = "interaction"
)

type permissionModal struct {
	Source    permModalSource
	RequestID string // auth HITL request ID or interaction notification ID
	FrameID   string
	TaskID    string
	Action    string
	Title     string
	Detail    string
	Tool      string
	Args      string
	Target    string
	Reason    string
	Frame     *interaction.InteractionFrame
	Request   *authorization.PermissionRequest
}

func newChatUXState() *chatUXState {
	return &chatUXState{
		autoFollow:    true,
		styles:        chatPaletteFromTheme(nil),
		th:            theme.Default(),
		streamBuffers: make(map[string]*streamBuffer),
		toolExpanded:  make(map[string]bool),
	}
}

func (s *chatUXState) setSize(w, h int) {
	s.width, s.height = w, h
	if !s.vpReady {
		s.vp = viewport.New(w, h)
		s.vpReady = true
	} else {
		s.vp.Width, s.vp.Height = w, h
	}
	if s.mdWidth != w-8 || s.mdRenderer == nil {
		s.mdWidth = max(20, w-8)
		s.rebuildMarkdownRenderer()
	}
}

func (s *chatUXState) SetTheme(th *theme.Theme) {
	if th == nil {
		th = theme.Default()
	}
	s.th = th
	s.styles = chatPaletteFromTheme(th)
	s.rebuildMarkdownRenderer()
}

func (s *chatUXState) rebuildMarkdownRenderer() {
	if s.mdWidth <= 0 {
		s.mdWidth = 60
	}
	style := "dark"
	if s.th != nil {
		// heuristic: use dark style for dark backgrounds
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(max(20, s.mdWidth)),
	)
	if err == nil {
		s.mdRenderer = r
	}
}

func (s *chatUXState) viewportUpdate(msg tea.Msg) tea.Cmd {
	if !s.vpReady {
		return nil
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	switch m := msg.(type) {
	case tea.MouseMsg:
		s.autoFollow = s.vp.AtBottom()
		if s.autoFollow {
			s.newCount = 0
		}
	case tea.KeyMsg:
		// handled by pane; track follow state after scroll keys
		_ = m
	}
	s.autoFollow = s.vp.AtBottom()
	if s.autoFollow {
		s.newCount = 0
	}
	return cmd
}

func (s *chatUXState) scrollUp(n int) {
	if !s.vpReady {
		return
	}
	s.vp.ScrollUp(n)
	s.autoFollow = false
}
func (s *chatUXState) scrollDown(n int) {
	if !s.vpReady {
		return
	}
	s.vp.ScrollDown(n)
	s.autoFollow = s.vp.AtBottom()
	if s.autoFollow {
		s.newCount = 0
	}
}
func (s *chatUXState) pageUp() {
	if s.vpReady {
		s.vp.PageUp()
		s.autoFollow = false
	}
}
func (s *chatUXState) pageDown() {
	if s.vpReady {
		s.vp.PageDown()
		s.autoFollow = s.vp.AtBottom()
		if s.autoFollow {
			s.newCount = 0
		}
	}
}
func (s *chatUXState) gotoBottom() {
	if !s.vpReady {
		return
	}
	s.vp.GotoBottom()
	s.autoFollow = true
	s.newCount = 0
}

func (s *chatUXState) appendStreamToken(runID, token string) {
	if token == "" {
		return
	}
	b := s.streamBuffers[runID]
	if b == nil {
		b = &streamBuffer{}
		s.streamBuffers[runID] = b
	}
	b.pending = append(b.pending, []rune(token)...)
	b.cursor = true
}

func (s *chatUXState) streamText(runID string) string {
	b := s.streamBuffers[runID]
	if b == nil {
		return ""
	}
	return b.shown.String()
}

func (s *chatUXState) advanceStreams(now time.Time) bool {
	changed := false
	for _, b := range s.streamBuffers {
		if len(b.pending) == 0 {
			continue
		}
		// Reveal buffered stream immediately to avoid lag/stalls (esp. during approvals).
		n := len(b.pending)
		b.shown.WriteString(string(b.pending[:n]))
		b.pending = b.pending[n:]
		b.last = now
		changed = true
	}
	// blink cursor ~500ms
	if now.Sub(s.lastAnim) > 450*time.Millisecond {
		s.cursorOn = !s.cursorOn
		s.lastAnim = now
		s.shimmerTick++
		changed = true
	}
	// spring animation for tool cards
	spring := harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.5)
	for i := range s.toolCards {
		c := &s.toolCards[i]
		if math.Abs(c.Anim-c.Target) < 0.001 && math.Abs(c.AnimVel) < 0.001 {
			c.Anim, c.AnimVel = c.Target, 0
			continue
		}
		c.Anim, c.AnimVel = spring.Update(c.Anim, c.AnimVel, c.Target)
		changed = true
	}
	return changed
}

func (s *chatUXState) finishStream(runID string, finalText string) {
	b := s.streamBuffers[runID]
	if b == nil {
		b = &streamBuffer{}
		s.streamBuffers[runID] = b
	}
	// flush remaining pending into shown, prefer final text if provided
	if strings.TrimSpace(finalText) != "" {
		b.shown.Reset()
		b.shown.WriteString(finalText)
		b.pending = nil
	} else if len(b.pending) > 0 {
		b.shown.WriteString(string(b.pending))
		b.pending = nil
	}
	b.cursor = false
}

func (s *chatUXState) clearStream(runID string) { delete(s.streamBuffers, runID) }

func (s *chatUXState) renderMarkdown(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if s.mdRenderer == nil {
		return text
	}
	out, err := s.mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

func (s *chatUXState) thinkingView(active bool, spinner string) string {
	if !active {
		return ""
	}
	label := "thinking…"
	// shimmering gradient sweep across label
	colors := []lipgloss.AdaptiveColor{s.styles.muted, s.styles.cyan, s.styles.purple, s.styles.pink, s.styles.cyan, s.styles.muted}
	var b strings.Builder
	runes := []rune(label)
	for i, r := range runes {
		idx := (i + s.shimmerTick) % len(colors)
		st := lipgloss.NewStyle().Foreground(colors[idx]).Bold(true)
		b.WriteRune(' ')
		_ = st
		b.WriteString(st.Render(string(r)))
	}
	// simpler: render each char with shifted color
	b.Reset()
	for i, r := range runes {
		idx := (i + s.shimmerTick) % len(colors)
		b.WriteString(lipgloss.NewStyle().Foreground(colors[idx]).Bold(true).Render(string(r)))
	}
	spin := spinner
	if spin == "" {
		spin = "⣾"
	}
	line := b.String() + " " + lipgloss.NewStyle().Foreground(s.styles.cyan).Render(spin)
	return lipgloss.NewStyle().Foreground(s.styles.muted).Render(line)
}

func (s *chatUXState) newPill() string {
	if s.autoFollow || s.newCount <= 0 {
		return ""
	}
	label := "↓ new"
	if s.newCount > 1 {
		label = fmt.Sprintf("↓ %d new", s.newCount)
	}
	return lipgloss.NewStyle().
		Foreground(s.styles.bg).
		Background(s.styles.pink).
		Bold(true).
		Padding(0, 1).
		Render(label)
}

// tool card helpers
func (s *chatUXState) upsertToolCard(card toolCallCard) {
	for i := range s.toolCards {
		if s.toolCards[i].ID == card.ID {
			old := s.toolCards[i]
			if card.Anim == 0 && old.Anim != 0 {
				card.Anim, card.AnimVel = old.Anim, old.AnimVel
			}
			if _, ok := s.toolExpanded[card.ID]; ok {
				card.Expanded = s.toolExpanded[card.ID]
			}
			card.Target = 1
			s.toolCards[i] = card
			return
		}
	}
	card.Target = 1
	s.toolCards = append(s.toolCards, card)
}

func (s *chatUXState) toggleSelectedTool() {
	if s.toolSel < 0 || s.toolSel >= len(s.toolCards) {
		return
	}
	id := s.toolCards[s.toolSel].ID
	s.toolExpanded[id] = !s.toolExpanded[id]
	s.toolCards[s.toolSel].Expanded = s.toolExpanded[id]
}

func (s *chatUXState) renderToolCards(width int) string {
	if len(s.toolCards) == 0 {
		return ""
	}
	var parts []string
	for i, c := range s.toolCards {
		sel := s.toolFocus && i == s.toolSel
		parts = append(parts, s.renderToolCard(c, width, sel))
	}
	return strings.Join(parts, "\n")
}

func (s *chatUXState) renderToolCard(c toolCallCard, width int, selected bool) string {
	icon := "○"
	stateStyle := lipgloss.NewStyle().Foreground(s.styles.muted)
	status := "pending"
	switch c.State {
	case toolRunning:
		icon = spinnerFrames[s.shimmerTick%len(spinnerFrames)]
		stateStyle = lipgloss.NewStyle().Foreground(s.styles.cyan)
		elapsed := time.Since(c.Started).Truncate(100 * time.Millisecond)
		status = fmt.Sprintf("running %s", elapsed)
	case toolDone:
		icon = "✓"
		stateStyle = lipgloss.NewStyle().Foreground(s.styles.green)
		status = "done"
	case toolError:
		icon = "✗"
		stateStyle = lipgloss.NewStyle().Foreground(s.styles.red)
		status = "error"
	}
	name := c.Name
	if name == "" {
		name = "tool"
	}
	head := fmt.Sprintf("%s %s  %s", icon, name, status)
	border := s.styles.muted
	if selected {
		border = s.styles.cyan
	}
	if c.State == toolRunning {
		border = s.styles.purple
	}
	title := stateStyle.Bold(true).Render(head)
	body := ""
	expanded := c.Expanded || s.toolExpanded[c.ID]
	if expanded {
		var lines []string
		if strings.TrimSpace(c.Args) != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("args:"))
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.fg).Render(indentBlock(trimOneLine(c.Args, 800), "  ")))
		}
		if strings.TrimSpace(c.Output) != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("output:"))
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.fg).Render(indentBlock(trimOneLine(c.Output, 1200), "  ")))
		}
		if strings.TrimSpace(c.Err) != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.red).Render("error:"))
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.red).Render(indentBlock(trimOneLine(c.Err, 800), "  ")))
		}
		if len(lines) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("(no details)"))
		}
		// spring open feel: reveal proportional content
		full := strings.Join(lines, "\n")
		progress := c.Anim
		if c.Target == 0 {
			progress = 0
		}
		if progress <= 0.01 {
			progress = 1
		}
		if progress < 0.999 {
			r := []rune(full)
			n := int(math.Max(1, float64(len(r))*math.Min(1, progress)))
			if n < len(r) {
				full = string(r[:n])
			}
		}
		body = "\n" + full
	} else {
		hint := lipgloss.NewStyle().Foreground(s.styles.muted).Italic(true).Render("enter: expand")
		if selected {
			body = "\n" + hint
		}
	}
	cardW := max(20, width-6)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Foreground(s.styles.fg).
		Background(s.styles.surface).
		Padding(0, 1).
		Width(cardW).
		Render(title + body)
}

func (s *chatUXState) renderPermissionModal(width, height int) string {
	m := s.permModal
	if m == nil {
		return ""
	}
	titleText := strings.TrimSpace(m.Title)
	if titleText == "" {
		titleText = "Permission required"
	}
	title := lipgloss.NewStyle().Foreground(s.styles.orange).Bold(true).Render(titleText)
	toolName := strings.TrimSpace(m.Tool)
	if toolName == "" {
		toolName = "tool"
	}
	tool := lipgloss.NewStyle().Foreground(s.styles.cyan).Bold(true).Render(toolName)
	detail := strings.TrimSpace(m.Detail)
	if detail == "" {
		detail = "This action may modify state or run commands."
	}
	args := strings.TrimSpace(m.Args)
	if args == "" {
		args = "(no arguments)"
	}
	lines := []string{
		title,
		"",
		lipgloss.NewStyle().Foreground(s.styles.fg).Render(detail),
		"",
		lipgloss.NewStyle().Foreground(s.styles.muted).Render("tool/action: ") + tool,
	}
	if strings.TrimSpace(m.Action) != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("capability: ")+lipgloss.NewStyle().Foreground(s.styles.fg).Render(m.Action))
	}
	if strings.TrimSpace(m.Target) != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("target: ")+lipgloss.NewStyle().Foreground(s.styles.fg).Render(m.Target))
	}
	if strings.TrimSpace(m.Reason) != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.muted).Render("reason: ")+lipgloss.NewStyle().Foreground(s.styles.fg).Render(m.Reason))
	}
	lines = append(lines,
		"",
		lipgloss.NewStyle().Foreground(s.styles.muted).Render("will run:"),
		lipgloss.NewStyle().Foreground(s.styles.fg).Render(trimOneLine(args, 1200)),
		"",
	)
	keys := lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.green).Bold(true).Padding(0, 1).Render("y allow") + "  " +
		lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.red).Bold(true).Padding(0, 1).Render("n deny") + "  " +
		lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.purple).Bold(true).Padding(0, 1).Render("a always")
	if m.Source == permModalSourceInteraction && m.Frame != nil && !frameHasChoice(m.Frame, "always") {
		keys = lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.green).Bold(true).Padding(0, 1).Render("y approve") + "  " +
			lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.red).Bold(true).Padding(0, 1).Render("n reject") + "  " +
			lipgloss.NewStyle().Foreground(s.styles.bg).Background(s.styles.purple).Bold(true).Padding(0, 1).Render("a approve")
	}
	lines = append(lines, keys)
	body := strings.Join(lines, "\n")
	mw := min(max(40, width-10), 90)
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.styles.orange).
		Foreground(s.styles.fg).
		Background(s.styles.bg).
		Padding(1, 2).
		Width(mw).
		Render(body)
	// dim background by placing modal centered over dimmed overlay
	return lipgloss.Place(width, max(10, height), lipgloss.Center, lipgloss.Center, modal,
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(s.styles.surface),
	)
}

func frameHasChoice(frame *interaction.InteractionFrame, choice string) bool {
	if frame == nil {
		return false
	}
	choice = strings.TrimSpace(choice)
	for _, c := range frame.Choices {
		if strings.TrimSpace(c) == choice {
			return true
		}
	}
	for _, s := range frame.Slots {
		if strings.TrimSpace(s.ID) == choice || strings.TrimSpace(s.Action) == choice {
			return true
		}
	}
	return false
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func trimOneLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func hashString(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}
