package authorization

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// TestAuthorizeCommandEmptyBashDefaultAsks is the P-4 red-line: an empty
// BashConfig must resolve to ask (HITL), never allow.
func TestAuthorizeCommandEmptyBashDefaultAsks(t *testing.T) {
	m := testPermissionManager(t)
	err := AuthorizeCommand(context.Background(), m, "agent-1", &BashConfig{}, CommandAuthorizationRequest{
		Command: []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("empty bash default must ask (HITL), not allow")
	}
}

func TestAuthorizeCommandExplicitAllow(t *testing.T) {
	m := testPermissionManager(t)
	err := AuthorizeCommand(context.Background(), m, "agent-1", &BashConfig{Default: permissions.DecisionAllow}, CommandAuthorizationRequest{
		Command: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("explicit bash default allow should permit the command, got: %v", err)
	}
}

func TestAuthorizeCommandExplicitDeny(t *testing.T) {
	m := testPermissionManager(t)
	err := AuthorizeCommand(context.Background(), m, "agent-1", &BashConfig{Default: permissions.DecisionDeny}, CommandAuthorizationRequest{
		Command: []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("explicit bash default deny must block the command")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Fatalf("deny should name the reason, got: %v", err)
	}
}

func TestAuthorizeCommandDenyPatternWinsOverAllowDefault(t *testing.T) {
	m := testPermissionManager(t)
	cfg := &BashConfig{
		DenyPatterns: []string{"echo *"},
		Default:      permissions.DecisionAllow,
	}
	err := AuthorizeCommand(context.Background(), m, "agent-1", cfg, CommandAuthorizationRequest{
		Command: []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("a matching deny pattern must block even with an allow default")
	}
}

func TestAuthorizeCommandAllowPatternOverridesAskDefault(t *testing.T) {
	m := testPermissionManager(t)
	cfg := &BashConfig{
		AllowPatterns: []string{"echo *"},
		Default:       permissions.DecisionAsk,
	}
	err := AuthorizeCommand(context.Background(), m, "agent-1", cfg, CommandAuthorizationRequest{
		Command: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("a matching allow pattern should permit the command, got: %v", err)
	}
}
