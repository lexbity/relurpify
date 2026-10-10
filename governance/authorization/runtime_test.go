package authorization

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"github.com/stretchr/testify/require"
)

// TestRegisterAgentFailClosedWhenAuditDirUnwritable proves registration fails
// (rather than silently proceeding audit-less) when the audit chain cannot be
// initialized (SBH-1 D-10).
func TestRegisterAgentFailClosedWhenAuditDirUnwritable(t *testing.T) {
	// StateDir points AT a regular file: MkdirAll for audit/<agentID> must
	// fail regardless of euid, so the fail-closed path is deterministic.
	notADir := filepath.Join(t.TempDir(), "state-file")
	require.NoError(t, os.WriteFile(notADir, []byte("occupied"), 0o600))

	cfg := RuntimeConfig{
		DocumentSnapshot: struct{}{},
		Permissions:      ucperms.PermissionSet{},
		Backend:          "unit",
		BackendFactory: func(_ context.Context, backend string, _ governanceports.SandboxConfig, _, _ string) (governanceports.SandboxRuntime, error) {
			return &fakeSandboxRuntime{name: backend}, nil
		},
		BaseFS:      t.TempDir(),
		StateDir:    notADir,
		WorkspaceID: "ws-1",
		AgentName:   "reviewer",
	}
	_, err := RegisterAgent(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "audit chain init")
}

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
		Permissions:      ucperms.PermissionSet{},
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
