package e2e

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBootRender_RealAdapter_NoPanic verifies that the TUI boot path renders
// a first frame with no panic when driven through a real runtime adapter.
// This is the Tier-1 boot-render gate — it exercises the exact same
// newRootModel → Init → resize → View path that pty_safety.RunWithSurface
// uses, without a PTY.
func TestBootRender_RealAdapter_NoPanic(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	defer rt.Close(context.Background())

	adapter := tui.NewRuntimeAdapter(rt)
	if adapter == nil {
		t.Fatal("NewRuntimeAdapter returned nil")
	}

	// AC-1: SessionInfo has MaxTokens > 0 (populated Context fix).
	info := adapter.SessionInfo()
	if info.MaxTokens <= 0 {
		t.Errorf("SessionInfo().MaxTokens = %d, want > 0", info.MaxTokens)
	}

	// Tier-1 boot-render: Init + resize + startup doctor report + View (no PTY).
	// Init() dispatches startupDoctorReportCmd asynchronously in the real app;
	// here we execute that command and feed its DoctorStatusMsg back through
	// Update, mirroring the bubbletea runtime, so the startup gate re-evaluates
	// against the real (ready) report and unlocks chat.
	m := tui.NewTestRootModel(adapter, euclotui.NewSurfaceFactory())
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("Init() returned nil command; startup doctor probe not dispatched")
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm, ok := updated.(tui.RootModel)
	if !ok {
		t.Fatal("Update returned non-RootModel")
	}
	// Deliver the startup doctor report (what Init's async command produces).
	report := adapter.BuildDoctorReport(context.Background())
	if !report.Ready() {
		t.Fatalf("clean offline workspace not Ready: %+v", report)
	}
	updated, _ = rm.Update(tui.DoctorStatusMsg{Action: "refresh", Report: report})
	rm = updated.(tui.RootModel)
	frame := rm.View()

	if frame == "" {
		t.Fatal("View() returned empty frame")
	}
	if strings.Contains(frame, "Initializing...") {
		t.Fatal("View() still returning Initializing after resize")
	}
	// AC-1: rendered frame must reference the active agent name.
	if !strings.Contains(frame, "euclo") {
		t.Errorf("frame should contain 'euclo', full frame (len=%d):\n%s", len(frame), frame)
	}
}
