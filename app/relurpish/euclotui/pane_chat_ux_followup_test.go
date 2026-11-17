package euclotui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/telemetry"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

func newTestChatPane(t *testing.T) *ChatPane {
	t.Helper()
	p := NewChatPane(context.Background(), nil, &tui.AgentContext{}, &tui.Session{ID: "s1"}, &tui.NotificationQueue{}, nil, nil, nil)
	p.SetSize(100, 30)
	return p
}

func TestChatUXStreamBufferTypewriterAndCursor(t *testing.T) {
	p := newTestChatPane(t)
	run := &chatRun{id: "run-1", started: time.Now(), ch: make(chan tea.Msg, 8), builder: tui.NewMessageBuilder("run-1")}
	p.runsMu.Lock()
	p.runs[run.id] = run
	p.runsMu.Unlock()

	p.ux.appendStreamToken(run.id, "hello world")
	if got := p.ux.streamText(run.id); got != "" {
		t.Fatalf("stream should buffer before advance, got %q", got)
	}
	changed := p.ux.advanceStreams(time.Now())
	if !changed {
		t.Fatal("expected advanceStreams to report change")
	}
	if got := p.ux.streamText(run.id); got != "hello world" {
		t.Fatalf("expected buffered stream to flush promptly, got %q", got)
	}
	if len(p.ux.streamBuffers[run.id].pending) != 0 {
		t.Fatal("expected pending buffer drained")
	}
	// cursor blink state still advances over time
	_ = p.ux.advanceStreams(time.Now().Add(500 * time.Millisecond))
}

func TestChatUXScrollPauseAndNewPill(t *testing.T) {
	p := newTestChatPane(t)
	// Fill content so viewport can scroll.
	for i := 0; i < 40; i++ {
		p.AppendMessage(tui.Message{Role: tui.RoleAgent, Content: tui.MessageContent{Text: "line"}})
	}
	p.syncUXViewport(true)
	if !p.ux.autoFollow {
		t.Fatal("expected autoFollow at bottom initially")
	}
	p.ux.scrollUp(5)
	if p.ux.autoFollow {
		t.Fatal("expected autoFollow disabled after scroll up")
	}
	before := p.ux.newCount
	p.onMessageAdded()
	if p.ux.newCount != before+1 {
		t.Fatalf("expected newCount increment when paused, got %d", p.ux.newCount)
	}
	pill := p.ux.newPill()
	if pill == "" {
		t.Fatal("expected new message pill when paused with new messages")
	}
	p.ux.gotoBottom()
	if !p.ux.autoFollow || p.ux.newCount != 0 {
		t.Fatalf("expected follow restored and count cleared, follow=%v count=%d", p.ux.autoFollow, p.ux.newCount)
	}
	if pill := p.ux.newPill(); pill != "" {
		t.Fatal("expected no pill when following bottom")
	}
}

func TestChatPanePermissionModalAuthDispatch(t *testing.T) {
	p := newTestChatPane(t)
	req := &fauthorization.PermissionRequest{
		ID: "req-1",
		Permission: ucperms.PermissionDescriptor{
			Action:   "tool:cli_mkdir",
			Resource: "/tmp/x",
		},
		Justification: "create directory",
		Risk:          policy.RiskLevel("medium"),
	}
	p.ShowHITLPermissionModal(req)
	if p.ux.permModal == nil || p.ux.permModal.RequestID != "req-1" {
		t.Fatal("expected auth permission modal to be shown")
	}
	// allow once
	next, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p = next.(*ChatPane)
	if p.ux.permModal != nil {
		t.Fatal("expected modal dismissed on allow")
	}
	if cmd == nil {
		t.Fatal("expected approve cmd")
	}
	msg := cmd()
	approve, ok := msg.(tui.NotifHITLApproveMsg)
	if !ok {
		t.Fatalf("expected NotifHITLApproveMsg, got %T", msg)
	}
	if approve.ID != "req-1" || approve.Scope != policy.GrantScopeOneTime || approve.Action != "tool:cli_mkdir" {
		t.Fatalf("unexpected approve msg: %#v", approve)
	}

	p.ShowHITLPermissionModal(req)
	_, cmd = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatal("expected always cmd")
	}
	msg = cmd()
	approve, ok = msg.(tui.NotifHITLApproveMsg)
	if !ok || approve.Scope != policy.GrantScopePersistent {
		t.Fatalf("expected persistent approve, got %#v", msg)
	}

	p.ShowHITLPermissionModal(req)
	_, cmd = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("expected deny cmd")
	}
	msg = cmd()
	deny, ok := msg.(tui.NotifHITLDenyMsg)
	if !ok || deny.ID != "req-1" {
		t.Fatalf("expected deny msg for req-1, got %#v", msg)
	}
}

func TestChatPanePermissionModalInteractionDispatch(t *testing.T) {
	p := newTestChatPane(t)
	frame := interaction.InteractionFrame{
		ID:       "frame-1",
		TaskID:   "task-1",
		Type:     interaction.FrameHITLApproval,
		Question: "Approve permission tool:cli_mkdir?",
		Choices:  []string{"approve", "reject", "skip"},
		Payload: map[string]any{
			"permission": "tool:cli_mkdir",
			"command":    "mkdir /tmp/x",
			"risk":       "high",
		},
	}
	p.ShowInteractionPermissionModal("notif-1", frame)
	if p.ux.permModal == nil || p.ux.permModal.Source != permModalSourceInteraction {
		t.Fatal("expected interaction permission modal")
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected interaction approve cmd")
	}
	msg := cmd()
	resolve, ok := msg.(tui.NotifInteractionResolveMsg)
	if !ok {
		t.Fatalf("expected NotifInteractionResolveMsg, got %T", msg)
	}
	if resolve.NotificationID != "notif-1" || resolve.FrameID != "frame-1" || resolve.TaskID != "task-1" || resolve.ChoiceID != "approve" {
		t.Fatalf("unexpected resolve msg: %#v", resolve)
	}

	p.ShowInteractionPermissionModal("notif-1", frame)
	_, cmd = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	msg = cmd()
	resolve = msg.(tui.NotifInteractionResolveMsg)
	if resolve.ChoiceID != "reject" {
		t.Fatalf("expected reject choice, got %#v", resolve)
	}
}

func TestChatPaneToolCardExpandAndLifecycle(t *testing.T) {
	p := newTestChatPane(t)
	now := time.Now()
	p.ObserveTelemetryEvent(telemetry.Event{
		Type:      telemetry.EventToolCall,
		Timestamp: now,
		TaskID:    "task-1",
		SpanID:    "span-1",
		Metadata:  map[string]any{"tool": "shell", "args": map[string]any{"cmd": "ls -la"}},
	})
	if len(p.ux.toolCards) != 1 {
		t.Fatalf("expected 1 tool card, got %d", len(p.ux.toolCards))
	}
	card := p.ux.toolCards[0]
	if card.State != toolRunning || card.Name != "shell" || card.Args == "" {
		t.Fatalf("unexpected running card: %#v", card)
	}
	if !p.needsAnimTick() {
		t.Fatal("expected anim tick needed while tool running")
	}
	// focus and expand
	p.ux.toolFocus = true
	p.ux.toolSel = 0
	p.ux.toggleSelectedTool()
	if !p.ux.toolCards[0].Expanded {
		t.Fatal("expected selected tool card expanded")
	}
	p.ObserveTelemetryEvent(telemetry.Event{
		Type:      telemetry.EventToolResult,
		Timestamp: now.Add(150 * time.Millisecond),
		TaskID:    "task-1",
		SpanID:    "span-1",
		Metadata:  map[string]any{"tool": "shell", "success": true, "output": "ok", "duration_ms": 150},
	})
	if len(p.ux.toolCards) != 1 {
		t.Fatalf("expected result to update same card, got %d", len(p.ux.toolCards))
	}
	card = p.ux.toolCards[0]
	if card.State != toolDone || card.Output != "ok" {
		t.Fatalf("unexpected completed card: %#v", card)
	}
	// spring anim progress
	before := p.ux.toolCards[0].Anim
	p.ux.toolCards[0].Anim = 0
	p.ux.toolCards[0].AnimVel = 0
	p.ux.toolCards[0].Target = 1
	changed := p.ux.advanceStreams(time.Now())
	if !changed || p.ux.toolCards[0].Anim <= before {
		// before was 0
		if p.ux.toolCards[0].Anim <= 0 {
			t.Fatalf("expected spring animation to advance, anim=%v changed=%v", p.ux.toolCards[0].Anim, changed)
		}
	}
}

func TestChatPaneDismissModalByID(t *testing.T) {
	p := newTestChatPane(t)
	req := &fauthorization.PermissionRequest{ID: "req-x", Permission: ucperms.PermissionDescriptor{Action: "tool:x"}}
	p.ShowHITLPermissionModal(req)
	p.DismissHITLPermissionModal("other")
	if p.ux.permModal == nil {
		t.Fatal("modal should remain for different id")
	}
	p.DismissHITLPermissionModal("req-x")
	if p.ux.permModal != nil {
		t.Fatal("modal should dismiss for matching id")
	}
}
