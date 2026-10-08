package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

func TestGenerateAgentID(t *testing.T) {
	if got := generateAgentID("myworkspace", "euclo"); got != "agent-myworkspace-euclo" {
		t.Errorf("workspace+agent: got %q", got)
	}
	if got := generateAgentID("My Workspace", "Euclo"); got != "agent-my-workspace-euclo" {
		t.Errorf("sanitized: got %q", got)
	}
	if got := generateAgentID("", ""); got != "agent-unknown-unknown" {
		t.Errorf("empty: got %q", got)
	}
	if got := generateAgentID("  ", "  "); got != "agent-unknown-unknown" {
		t.Errorf("blank: got %q", got)
	}
	if got := generateAgentID("team/repo", "euclo"); got != "agent-team-repo-euclo" {
		t.Errorf("nested path: got %q", got)
	}
	if got := generateAgentID("ws", ""); got != "agent-ws-unknown" {
		t.Errorf("missing agent: got %q", got)
	}
}

func TestRegisterAgentGeneratesDeterministicID(t *testing.T) {
	cfg := RuntimeConfig{
		DocumentSnapshot: struct{}{},
		Permissions:      permissions.PermissionSet{},
		Backend:          "unit",
		BackendFactory: func(_ context.Context, backend string, _ governanceports.SandboxConfig, _, _ string) (governanceports.SandboxRuntime, error) {
			return &fakeSandboxRuntime{name: backend}, nil
		},
		BaseFS:      t.TempDir(),
		WorkspaceID: "ws-1",
		AgentName:   "reviewer",
	}

	first, err := RegisterAgent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if first.ID != "agent-ws-1-reviewer" {
		t.Fatalf("ID = %q, want %q", first.ID, "agent-ws-1-reviewer")
	}

	second, err := RegisterAgent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RegisterAgent (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("ID is not deterministic: %q != %q", second.ID, first.ID)
	}
}
