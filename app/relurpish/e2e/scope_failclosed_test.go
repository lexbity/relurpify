package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/ports"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestScopeFailClosed verifies that when the runtime is degraded (no verified
// sandbox scope), all host-touching file operations are denied and no host
// filesystem mutations occur (NFR-2).
func TestScopeFailClosed_DegradedRuntimeDeniesFileTools(t *testing.T) {
	workspace := t.TempDir()
	canary := filepath.Join(workspace, "canary.txt")
	ctx := context.Background()

	// Create a degraded runtime by using an invalid workspace config.
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.ConfigPath = filepath.Join(workspace, ".relurpify_state", "workspace.yaml")

	// Write an invalid config file so New() degrades.
	cfgDir := filepath.Dir(cfg.ConfigPath)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.ConfigPath, []byte("invalid: true\nbroken: yes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg.SecurityRunner = &recordingRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return nil, nil
	}

	rt, err := relurpishruntime.New(ctx, cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("total construction must not return error: %v", err)
	}
	defer func() { _ = rt.Close(ctx) }()

	ws := rt.AgentWorkspace()
	if ws == nil {
		t.Fatal("AgentWorkspace() must not be nil even when degraded")
	}
	if !ws.Readiness.Degraded {
		t.Log("runtime is not degraded — scope check will pass")
	}

	// Register file tools through the real registry if not already present.
	// On a degraded runtime, tools have deny-all scope from construction
	// (Slice 1 + Slice 2).
	reg := rt.Tools
	if reg == nil {
		t.Fatal("runtime must have a capability registry")
	}

	// Attempt file operations that should be blocked by deny-all scope.
	// File tools registered in the registry have deny-all scope when no
	// verified sandbox scope was installed (UseSandboxScope not called).

	// Try file_read (read-only — still gated by scope).
	result, err := reg.InvokeCapability(ctx, simpleState(), "file_read", map[string]any{"path": canary})
	verifyDenied(t, "file_read", result, err, canary)

	// Try file_write (destructive — must be denied).
	result, err = reg.InvokeCapability(ctx, simpleState(), "file_write", map[string]any{"path": canary, "content": "data"})
	verifyDenied(t, "file_write", result, err, canary)

	// Try file_create (destructive — must be denied).
	result, err = reg.InvokeCapability(ctx, simpleState(), "file_create", map[string]any{"path": "new.txt", "content": "data"})
	verifyDenied(t, "file_create", result, err, canary)

	// Try file_delete (destructive — must be denied).
	result, err = reg.InvokeCapability(ctx, simpleState(), "file_delete", map[string]any{"path": canary})
	verifyDenied(t, "file_delete", result, err, canary)

	// Try file_edit (destructive — must be denied).
	result, err = reg.InvokeCapability(ctx, simpleState(), "file_edit", map[string]any{"path": canary, "old_string": "x", "new_string": "y"})
	verifyDenied(t, "file_edit", result, err, canary)

	// Verify canary file was NEVER created or modified.
	if _, statErr := os.Stat(canary); !os.IsNotExist(statErr) {
		t.Fatalf("canary file %s was created/modified — sandbox scope did not deny", canary)
	}
}

func verifyDenied(t *testing.T, toolName string, result *ports.ToolResult, err error, canary string) {
	t.Helper()
	if err != nil {
		// Error is expected — file operation was blocked.
		t.Logf("%s: denied with error: %v", toolName, err)
		return
	}
	if result != nil && !result.Success {
		// Expected: result shows failure.
		return
	}
	if result != nil && result.Success {
		t.Errorf("%s: unexpectedly succeeded — canary file %s may have been written", toolName, canary)
	}
}

type simpleEnv struct{}

func (simpleEnv) GetWorkingValue(key string) (any, bool) { return nil, false }
func (simpleEnv) SetWorkingValue(key string, value any)  {}
func (simpleEnv) DeleteWorkingValue(key string)          {}
func (simpleEnv) ClearWorkingData()                      {}
func (simpleEnv) WorkingMemoryKeys() []string            { return nil }
func (simpleEnv) Snapshot() map[string]any               { return nil }
func (simpleEnv) TaskID() string                         { return "test" }
func (simpleEnv) SessionID() string                      { return "test-session" }

func simpleState() ports.State { return simpleEnv{} }
