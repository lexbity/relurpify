package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBoot_NoAgentsDir verifies the runtime boots and SubmitTurn succeeds when
// the relurpify_cfg/agents/ directory does not exist. The agent baseline comes
// entirely from the built-in contract (euclocontract.DefaultContract).
func TestBoot_NoAgentsDir(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})

	agentsDir := filepath.Join(workspace, "relurpify_cfg", "agents")
	if err := os.RemoveAll(agentsDir); err != nil {
		t.Fatalf("remove agents dir: %v", err)
	}

	runner := &recordingRunner{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime with agents/ removed: %v", err)
	}

	info := rt.AgentWorkspace()
	if info == nil {
		t.Fatal("AgentWorkspace() must not be nil")
	}
	if info.AgentSpec == nil {
		t.Fatal("AgentSpec must not be nil after fileless boot")
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestBoot_MissingSecurityOverlay verifies the runtime boots with built-in
// defaults when a security policy file is missing (not present, not malformed).
func TestBoot_MissingSecurityOverlay(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})

	sandboxPolicy := filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
	if err := os.Remove(sandboxPolicy); err != nil {
		t.Fatalf("remove sandbox policy: %v", err)
	}

	runner := &recordingRunner{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime with missing sandbox policy: %v", err)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestBoot_MissingAllSecurityOverlays verifies boot succeeds when all four
// security policy files are absent.
func TestBoot_MissingAllSecurityOverlays(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})

	securityDir := filepath.Join(workspace, "relurpify_cfg", "security")
	entries, err := os.ReadDir(securityDir)
	if err != nil {
		t.Fatalf("read security dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			if err := os.Remove(filepath.Join(securityDir, entry.Name())); err != nil {
				t.Fatalf("remove %s: %v", entry.Name(), err)
			}
		}
	}

	runner := &recordingRunner{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime with all security policies removed: %v", err)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
