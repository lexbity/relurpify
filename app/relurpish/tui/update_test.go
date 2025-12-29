package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"testing"
	"time"
)

func TestToggleExpandTargetsSectionOnly(t *testing.T) {
	m := Model{
		messages: []Message{{
			Role: RoleAgent,
			Content: MessageContent{
				Expanded: map[string]bool{
					"thinking": true,
					"plan":     false,
					"changes":  false,
				},
			},
		}},
		expandTarget: "plan",
	}
	updatedAny, _ := m.toggleExpandAtCursor()
	updated := updatedAny.(Model)
	expanded := updated.messages[0].Content.Expanded
	if expanded["plan"] != true {
		t.Fatalf("expected plan toggled true, got %v", expanded["plan"])
	}
	if expanded["thinking"] != true {
		t.Fatalf("expected thinking unchanged, got %v", expanded["thinking"])
	}
	if expanded["changes"] != false {
		t.Fatalf("expected changes unchanged, got %v", expanded["changes"])
	}
}

func TestCycleExpandTarget(t *testing.T) {
	m := Model{expandTarget: "thinking"}
	updatedAny, _ := m.cycleExpandTarget()
	updated := updatedAny.(Model)
	if updated.expandTarget != "plan" {
		t.Fatalf("expected expand target plan, got %s", updated.expandTarget)
	}
}

func TestHandleStreamCompleteUpdatesContextTokens(t *testing.T) {
	runID := "run-1"
	m := Model{
		runStates: map[string]*RunState{
			runID: {
				ID:      runID,
				Builder: NewMessageBuilder(runID),
			},
		},
		session: &Session{},
		context: &AgentContext{},
	}
	updatedAny, _ := m.handleStreamComplete(StreamCompleteMsg{
		RunID:      runID,
		Duration:   2 * time.Second,
		TokensUsed: 42,
	})
	updated := updatedAny.(Model)
	if updated.context.UsedTokens != 42 {
		t.Fatalf("expected context used tokens 42, got %d", updated.context.UsedTokens)
	}
	if updated.session.TotalTokens != 42 {
		t.Fatalf("expected session total tokens 42, got %d", updated.session.TotalTokens)
	}
}

func TestHITLEventRequestedWithoutPendingPrompts(t *testing.T) {
	hitl := newFakeHITL()
	input := textinput.New()
	input.Focus()
	req := &runtime.PermissionRequest{
		ID:            "hitl-req",
		Permission:    core.PermissionDescriptor{Action: "file:write", Resource: "README.md"},
		Justification: "test",
	}
	m := Model{
		hitl:     hitl,
		hitlCh:   hitl.ch,
		input:    input,
		mode:     ModeNormal,
		messages: []Message{},
	}
	updatedAny, _ := m.Update(hitlEventMsg{event: runtime.HITLEvent{Type: runtime.HITLEventRequested, Request: req}})
	updated := updatedAny.(Model)
	if updated.mode != ModeHITL {
		t.Fatalf("expected ModeHITL, got %v", updated.mode)
	}
	if updated.hitlRequest == nil || updated.hitlRequest.ID != "hitl-req" {
		t.Fatalf("expected hitlRequest hitl-req, got %+v", updated.hitlRequest)
	}
}
