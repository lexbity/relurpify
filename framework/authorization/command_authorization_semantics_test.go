package authorization

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type mockHITLProvider struct {
	requestCount int
}

func (m *mockHITLProvider) RequestPermission(ctx context.Context, req PermissionRequest) (*PermissionGrant, error) {
	m.requestCount++
	return &PermissionGrant{
		ID:         "test-grant",
		Permission: req.Permission,
		Scope:      req.Scope,
		ApprovedBy: "mock-user",
	}, nil
}

func TestAuthorizeCommand_SemanticInterception(t *testing.T) {
	// Set up PermissionManager with specific declared permissions:
	// - Read access to /home/workspace/allowed.txt
	// - Write access to /home/workspace/build/output.txt
	declared := &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{
				Action: contracts.FileSystemRead,
				Path:   "/home/workspace/allowed.txt",
			},
			{
				Action: contracts.FileSystemWrite,
				Path:   "/home/workspace/build/output.txt",
			},
		},
		Executables: []contracts.ExecutablePermission{
			{
				Binary: "bash",
			},
			{
				Binary: "cat",
			},
		},
	}

	audit := core.NewInMemoryAuditLogger(10)
	hitl := &mockHITLProvider{}
	pm, err := NewPermissionManager("/home/workspace", declared, audit, hitl)
	if err != nil {
		t.Fatalf("unexpected NewPermissionManager error: %v", err)
	}

	// Default policy for undeclared operations is Deny so we can assert on hard failures
	pm.SetDefaultPolicy(agentspec.AgentPermissionDeny)

	spec := &agentspec.AgentRuntimeSpec{
		Bash: agentspec.AgentBashPermissions{
			Default: agentspec.AgentPermissionAllow,
		},
	}

	ctx := context.Background()

	// Case 1: Allowed shell-read command
	req1 := CommandAuthorizationRequest{
		Command: []string{"bash", "-c", "cat /home/workspace/allowed.txt"},
		Source:  "test",
	}
	if err := AuthorizeCommand(ctx, pm, "test-agent", spec, req1); err != nil {
		t.Errorf("expected allowed.txt read to pass, got error: %v", err)
	}

	// Case 2: Bypassed read command on protected path keys.pem (should be intercepted and blocked!)
	req2 := CommandAuthorizationRequest{
		Command: []string{"bash", "-c", "cat /home/workspace/keys.pem"},
		Source:  "test",
	}
	err2 := AuthorizeCommand(ctx, pm, "test-agent", spec, req2)
	if err2 == nil {
		t.Errorf("expected keys.pem read command to be blocked, but it passed")
	} else if !strings.Contains(err2.Error(), "semantic filesystem check denied") {
		t.Errorf("expected semantic filesystem check error, got: %v", err2)
	}

	// Case 3: Destructive rm command on allowed path (blocked because fs:delete is not declared)
	req3 := CommandAuthorizationRequest{
		Command: []string{"bash", "-c", "rm -rf /home/workspace/allowed.txt"},
		Source:  "test",
	}
	err3 := AuthorizeCommand(ctx, pm, "test-agent", spec, req3)
	if err3 == nil {
		t.Errorf("expected rm command to be blocked, but it passed")
	} else if !strings.Contains(err3.Error(), "semantic filesystem check denied") {
		t.Errorf("expected semantic filesystem check error, got: %v", err3)
	}

	// Case 4: Dynamic command execution (should require HITL approval!)
	req4 := CommandAuthorizationRequest{
		Command: []string{"bash", "-c", "eval $(something)"},
		Source:  "test",
	}
	// For HITL check, we set default policy to Ask
	pm.SetDefaultPolicy(agentspec.AgentPermissionAsk)
	err4 := AuthorizeCommand(ctx, pm, "test-agent", spec, req4)
	if err4 != nil {
		t.Errorf("expected dynamic command to request HITL and pass successfully, got error: %v", err4)
	}
	if hitl.requestCount != 1 {
		t.Errorf("expected 1 HITL request for dynamic execution, got %d", hitl.requestCount)
	}
}
