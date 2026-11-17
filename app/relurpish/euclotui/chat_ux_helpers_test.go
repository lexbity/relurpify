package euclotui

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestDraculaThemeAndPalette(t *testing.T) {
	th := DraculaTheme()
	if th == nil {
		t.Fatal("DraculaTheme returned nil")
	}
	pal := chatPaletteFromTheme(th)
	if pal.fg.Light == "" || pal.fg.Dark == "" {
		t.Error("palette fg unset")
	}
	if pal.bg.Light == "" || pal.surface.Light == "" {
		t.Error("palette bg/surface unset")
	}
	// nil theme falls back to defaults.
	def := chatPaletteFromTheme(nil)
	if def.fg.Light == "" || def.fg.Dark == "" {
		t.Error("default palette fg unset")
	}
	if got := newDraculaStyles(); got.fg.Light == "" {
		t.Error("newDraculaStyles fg unset")
	}
}

func TestToolEventHelpers(t *testing.T) {
	if got := firstMetaString(map[string]any{"call_id": "  c1  "}, "call_id"); got != "c1" {
		t.Errorf("firstMetaString = %q, want c1", got)
	}
	if got := firstMetaString(map[string]any{"call_id": 42}, "call_id", "tool_call_id"); got != "" {
		t.Errorf("firstMetaString non-string = %q, want empty", got)
	}
	if got := firstMetaString(map[string]any{"x": "  "}, "x", "y"); got != "" {
		t.Errorf("firstMetaString whitespace = %q, want empty", got)
	}

	if got := formatToolArgs(map[string]any{"args": "a b"}); got != "a b" {
		t.Errorf("formatToolArgs args = %q", got)
	}
	if got := formatToolArgs(map[string]any{"raw_args": map[string]any{"k": "v"}}); got == "" {
		t.Error("formatToolArgs raw_args produced empty output")
	}
	if got := formatToolArgs(map[string]any{"command": "git status"}); got != "git status" {
		t.Errorf("formatToolArgs command = %q", got)
	}
	if got := formatToolArgs(map[string]any{}); got != "" {
		t.Errorf("formatToolArgs empty = %q, want empty", got)
	}

	if got := formatToolOutput(map[string]any{"output": "ok"}); got != "ok" {
		t.Errorf("formatToolOutput output = %q", got)
	}
	got := formatToolOutput(map[string]any{"artifact_ref": "a.txt", "exit_code": 2, "stdout_bytes": 10})
	for _, want := range []string{"ref: a.txt", "exit_code: 2", "stdout_bytes: 10"} {
		if !containsSubstring(got, want) {
			t.Errorf("formatToolOutput missing %q in %q", want, got)
		}
	}
	if got := formatToolOutput(map[string]any{}); got != "" {
		t.Errorf("formatToolOutput empty = %q, want empty", got)
	}

	if got := stringifyToolValue("str"); got != "str" {
		t.Errorf("stringifyToolValue string = %q", got)
	}
	if got := stringifyToolValue([]byte("bytes")); got != "bytes" {
		t.Errorf("stringifyToolValue bytes = %q", got)
	}
	if got := stringifyToolValue(map[string]any{"a": 1}); got == "" {
		t.Error("stringifyToolValue map produced empty output")
	}
	if got := stringifyToolValue(complex(1, 2)); got == "" {
		t.Error("stringifyToolValue complex produced empty output")
	}

	if b, ok := metaBool(map[string]any{"f": true}, "f"); !b || !ok {
		t.Errorf("metaBool true = %v, %v", b, ok)
	}
	if b, ok := metaBool(map[string]any{"f": "yes"}, "f"); b || ok {
		t.Errorf("metaBool non-bool = %v, %v", b, ok)
	}
	if b, ok := metaBool(map[string]any{}, "f"); b || ok {
		t.Errorf("metaBool absent = %v, %v", b, ok)
	}

	if got := metaInt(map[string]any{"n": 3}, "n"); got != 3 {
		t.Errorf("metaInt int = %d", got)
	}
	if got := metaInt(map[string]any{"n": int64(4)}, "n"); got != 4 {
		t.Errorf("metaInt int64 = %d", got)
	}
	if got := metaInt(map[string]any{"n": 5.5}, "n"); got != 5 {
		t.Errorf("metaInt float64 = %d", got)
	}
	if got := metaInt(map[string]any{"n": "x"}, "n"); got != 0 {
		t.Errorf("metaInt non-int = %d", got)
	}
	if got := metaInt(map[string]any{}, "n"); got != 0 {
		t.Errorf("metaInt absent = %d", got)
	}

	if !metaHas(map[string]any{"k": nil}, "k") {
		t.Error("metaHas(k:nil) = false")
	}
	if metaHas(map[string]any{}, "k") {
		t.Error("metaHas absent = true")
	}
}

func TestToolEventCorrelationID(t *testing.T) {
	base := telemetry.Event{Seq: 7}
	if got := toolEventCorrelationID(base, map[string]any{"invocation_id": "inv9"}); got != "inv9" {
		t.Errorf("correlation meta = %q", got)
	}
	if got := toolEventCorrelationID(telemetry.Event{SpanID: "span1"}, nil); got != "span1" {
		t.Errorf("correlation span = %q", got)
	}
	if got := toolEventCorrelationID(telemetry.Event{NodeID: "n1", Type: telemetry.EventType("run")}, nil); got != "n1:run" {
		t.Errorf("correlation node = %q", got)
	}
	if got := toolEventCorrelationID(telemetry.Event{TraceID: "tr1", Seq: 2}, nil); got != "tr1:2" {
		t.Errorf("correlation trace = %q", got)
	}
	if got := toolEventCorrelationID(telemetry.Event{TaskID: "task1", Seq: 3}, nil); got != "task1:3" {
		t.Errorf("correlation task = %q", got)
	}
}

func TestChatUXStateStreams(t *testing.T) {
	s := newChatUXState()
	if !s.autoFollow {
		t.Error("new state should auto-follow")
	}
	if s.streamText("r1") != "" {
		t.Error("empty stream should produce empty text")
	}
	s.appendStreamToken("r1", "hel")
	s.appendStreamToken("r1", "lo")
	if s.streamText("r1") != "" {
		t.Error("pending tokens must not be visible before advance")
	}
	now := time.Now()
	if !s.advanceStreams(now) {
		t.Error("advanceStreams with pending tokens reported no change")
	}
	if got := s.streamText("r1"); got != "hello" {
		t.Errorf("streamText = %q, want hello", got)
	}
	// Blink: second advance within the animation window does not toggle.
	if s.advanceStreams(now.Add(100 * time.Millisecond)) {
		t.Error("advanceStreams within blink window reported change")
	}
	if !s.advanceStreams(now.Add(500 * time.Millisecond)) {
		t.Error("advanceStreams past blink window reported no change")
	}

	s.finishStream("r1", "final")
	if got := s.streamText("r1"); got != "final" {
		t.Errorf("finishStream final = %q", got)
	}
	s.appendStreamToken("r2", "partial")
	s.finishStream("r2", "")
	if got := s.streamText("r2"); got != "partial" {
		t.Errorf("finishStream flush = %q", got)
	}
	s.finishStream("absent", "created")
	if got := s.streamText("absent"); got != "created" {
		t.Errorf("finishStream creates buffer = %q", got)
	}
	s.clearStream("r1")
	if s.streamText("r1") != "" {
		t.Error("clearStream did not drop buffer")
	}
	s.appendStreamToken("r3", "")
	if s.streamText("r3") != "" {
		t.Error("empty token created a buffer")
	}
}

func TestChatUXStateViewportOps(t *testing.T) {
	s := newChatUXState()
	s.scrollUp(1) // no viewport yet: no-op
	if !s.autoFollow {
		t.Error("scrollUp without viewport changed autoFollow")
	}
	s.setSize(60, 20)
	if !s.vpReady || s.width != 60 || s.height != 20 {
		t.Fatalf("setSize did not prepare viewport: %+v", s.vpReady)
	}
	if s.mdRenderer == nil {
		t.Error("setSize did not build markdown renderer")
	}
	s.gotoBottom()
	if !s.autoFollow {
		t.Error("gotoBottom should set autoFollow")
	}
	s.scrollUp(3)
	if s.autoFollow {
		t.Error("scrollUp should leave auto-follow")
	}
	s.scrollDown(3)
	s.pageUp()
	if s.autoFollow {
		t.Error("pageUp should leave auto-follow")
	}
	s.pageDown()
}

func TestChatUXMarkdownAndThinking(t *testing.T) {
	s := newChatUXState()
	s.setSize(80, 24)
	if got := s.renderMarkdown("   "); got != "" {
		t.Errorf("renderMarkdown blank = %q, want empty", got)
	}
	if got := s.renderMarkdown("**bold** text"); got == "" {
		t.Error("renderMarkdown returned empty for markdown")
	}
	if s.thinkingView(false, "⣾") != "" {
		t.Error("thinkingView inactive should be empty")
	}
	if got := s.thinkingView(true, ""); got == "" {
		t.Error("thinkingView active returned empty")
	}
	if got := s.thinkingView(true, "⣾"); got == "" {
		t.Error("thinkingView with spinner returned empty")
	}
}

func TestChatUXPillAndIndent(t *testing.T) {
	s := newChatUXState()
	if got := s.newPill(); got != "" {
		t.Errorf("newPill while auto-follow = %q, want empty", got)
	}
	s.autoFollow = false
	s.newCount = 2
	if got := s.newPill(); got == "" {
		t.Error("newPill with unseen messages returned empty")
	}
	if got := indentBlock("a\nb", "  "); got != "  a\n  b" {
		t.Errorf("indentBlock = %q", got)
	}
}

func TestChatUXToolCards(t *testing.T) {
	s := newChatUXState()
	s.setSize(80, 24)
	if got := s.renderToolCards(80); got != "" {
		t.Errorf("renderToolCards empty = %q, want empty", got)
	}

	card := toolCallCard{ID: "t1", Name: "git", State: toolDone, Args: "status", Output: "clean", Anim: 0, Target: 1}
	s.upsertToolCard(card)
	if len(s.toolCards) != 1 {
		t.Fatalf("upsertToolCard did not insert: %d cards", len(s.toolCards))
	}
	// Second upsert preserves animation state (same ID).
	s.toolCards[0].Anim = 0.5
	s.upsertToolCard(toolCallCard{ID: "t1", Name: "git", State: toolDone})
	if s.toolCards[0].Anim != 0.5 {
		t.Error("upsertToolCard did not preserve animation")
	}

	// Tool card rendering per state.
	for _, tc := range []struct {
		state toolCardState
		icon string
	}{
		{toolPending, "○"},
		{toolRunning, ""},
		{toolDone, "✓"},
		{toolError, "✗"},
	} {
		c := toolCallCard{ID: "s-" + string(tc.state), Name: "tool", State: tc.state, Started: time.Now(), Expanded: true}
		got := s.renderToolCard(c, 80, false)
		if got == "" {
			t.Errorf("renderToolCard(%s) empty", tc.state)
		}
		if tc.icon != "" && !containsSubstring(got, tc.icon) {
			t.Errorf("renderToolCard(%s) missing icon %q in %q", tc.state, tc.icon, got)
		}
	}

	// Selected + collapsed shows the expand hint.
	sel := s.renderToolCard(toolCallCard{ID: "sel", Name: "tool", State: toolPending}, 80, true)
	if !containsSubstring(sel, "enter: expand") {
		t.Error("selected collapsed card lacks expand hint")
	}
	// Expanded with no details shows placeholder.
	plain := s.renderToolCard(toolCallCard{ID: "plain", Name: "tool", State: toolPending, Expanded: true}, 80, false)
	if !containsSubstring(plain, "(no details)") {
		t.Error("expanded card without details lacks placeholder")
	}

	// toggleSelectedTool toggles expansion.
	s.toolSel = 0
	s.toolFocus = true
	before := s.toolCards[0].Expanded
	s.toggleSelectedTool()
	if s.toolCards[0].Expanded == before {
		t.Error("toggleSelectedTool did not toggle expansion")
	}
	s.toolSel = 99
	s.toggleSelectedTool() // out of range: no-op
	if got := s.renderToolCards(80); got == "" {
		t.Error("renderToolCards non-empty returned empty")
	}
}

func TestChatUXPermissionModal(t *testing.T) {
	s := newChatUXState()
	s.setSize(80, 24)
	if got := s.renderPermissionModal(80, 24); got != "" {
		t.Errorf("renderPermissionModal without modal = %q, want empty", got)
	}
	s.permModal = &permissionModal{
		Source: permModalSourceAuthHITL,
		Title:  "Approve",
		Tool:   "git",
		Detail: "run git status",
		Args:   "status",
		Action: "fs.read",
		Target: "/workspace",
		Reason: "demo",
	}
	got := s.renderPermissionModal(80, 24)
	for _, want := range []string{"Approve", "git", "fs.read", "/workspace", "demo", "y allow"} {
		if !containsSubstring(got, want) {
			t.Errorf("renderPermissionModal missing %q in %q", want, got)
		}
	}

	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	if !frameHasChoice(frame, "review") || !frameHasChoice(frame, " implement ") {
		t.Error("frameHasChoice missed a listed choice")
	}
	if frameHasChoice(frame, "missing") {
		t.Error("frameHasChoice matched an absent choice")
	}
	if frameHasChoice(nil, "review") {
		t.Error("frameHasChoice(nil) matched")
	}

	s.permModal = &permissionModal{Source: permModalSourceInteraction, Frame: frame, Title: "Approve"}
	got = s.renderPermissionModal(80, 24)
	if !containsSubstring(got, "y approve") {
		t.Error("interaction modal should offer approve/reject")
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
