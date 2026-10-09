package authorization

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

type capturingHITL struct {
	requests []PermissionRequest
}

func (c *capturingHITL) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	c.requests = append(c.requests, req)
	return &PermissionGrant{ID: "g", Permission: req.Permission, Scope: req.Scope, ApprovedBy: "test"}, nil
}

func newCommandApprovalManager(t *testing.T) (*PermissionManager, *capturingHITL) {
	t.Helper()
	declared := &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "echo"}, {Binary: "bash"}, {Binary: "curl"}},
	}
	hitl := &capturingHITL{}
	pm, err := NewPermissionManager("/tmp", declared, newTestAuditLogger(t), hitl)
	require.NoError(t, err)
	return pm, hitl
}

// TestAuthorizeCommandParseFailureAsks is the P-3 red-line: an unparseable
// command escalates to HITL even when the bash default is allow; it never
// silently runs.
func TestAuthorizeCommandParseFailureAsks(t *testing.T) {
	pm, hitl := newCommandApprovalManager(t)
	err := AuthorizeCommand(context.Background(), pm, "agent-1", &BashConfig{Default: permissions.DecisionAllow}, CommandAuthorizationRequest{
		Command: []string{"bash", "-c", "echo ${"},
	})
	require.NoError(t, err, "the approver approves the ask")
	require.Len(t, hitl.requests, 1)
	require.Equal(t, commandReasonParseFailed, hitl.requests[0].Permission.Metadata["reason"])
}

func TestAuthorizeCommandOpaqueConstructorAsks(t *testing.T) {
	pm, hitl := newCommandApprovalManager(t)
	err := AuthorizeCommand(context.Background(), pm, "agent-1", &BashConfig{Default: permissions.DecisionAllow}, CommandAuthorizationRequest{
		Command: []string{"sudo", "rm", "-rf", "/"},
	})
	require.NoError(t, err)
	require.Len(t, hitl.requests, 1)
	require.Equal(t, commandReasonOpaque, hitl.requests[0].Permission.Metadata["reason"])
}

// TestAuthorizeCommandDenyGlobCrossesSlash is the P-6 red-line: a command deny
// pattern with a trailing '*' matches a path-bearing invocation.
func TestAuthorizeCommandDenyGlobCrossesSlash(t *testing.T) {
	pm, _ := newCommandApprovalManager(t)
	cfg := &BashConfig{
		DenyPatterns: []string{"curl http://evil.com*"},
		Default:      permissions.DecisionAllow,
	}
	err := AuthorizeCommand(context.Background(), pm, "agent-1", cfg, CommandAuthorizationRequest{
		Command: []string{"curl", "http://evil.com/x"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied")
}

func TestMatchCommandGlobCaseSensitive(t *testing.T) {
	require.True(t, MatchCommandGlob("rm -rf*", "rm -rf /"))
	require.True(t, MatchCommandGlob("git push*", "git push --force"))
	require.False(t, MatchCommandGlob("rm -rf*", "RM -rf /"))
	require.True(t, MatchCommandGlob(`curl \*literal`, "curl *literal"))
}

func TestDecideCommandByPatternsReasons(t *testing.T) {
	denied := DecideCommandByPatterns("curl http://evil.com/x", nil, []string{"curl http://evil.com*"}, permissions.DecisionAllow)
	require.Equal(t, permissions.DecisionDeny, denied.Decision)
	require.Equal(t, commandReasonDenyPattern, denied.Reason)

	allowed := DecideCommandByPatterns("git status", []string{"git *"}, nil, permissions.DecisionAsk)
	require.Equal(t, permissions.DecisionAllow, allowed.Decision)
	require.Equal(t, commandReasonAllowPattern, allowed.Reason)

	fallback := DecideCommandByPatterns("git status", nil, nil, permissions.DecisionAsk)
	require.Equal(t, permissions.DecisionAsk, fallback.Decision)
	require.Equal(t, commandReasonDefault, fallback.Reason)
}

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
