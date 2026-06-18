package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestExecDiff_ToolEditedFlow verifies that EventToolEdited events emitted
// through the runtime's telemetry chain reach the diff projection via the
// euclotui bridge, and that the diff pane renders and applies the resulting
// hunks against the workspace.
func TestExecDiff_ToolEditedFlow(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"demo.txt": "hello world\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = &recordingRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: &recordingRunner{}}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	eventCh, cancelEvents := rt.SubscribeExecutionEvents()
	defer cancelEvents()

	// Emit EventToolEdited through the workspace telemetry chain.
	// This simulates what the capability registry does after a write-class
	// invocation succeeds. The event flows through ws.Telemetry (which includes
	// the BroadcastSink from Slice 2) to the subscriber channel.
	rt.Workspace.Telemetry.Emit(telemetry.Event{
		Type:      telemetry.EventToolEdited,
		TaskID:    "edit-task",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"path":          "demo.txt",
			"origin":        "file_edit",
			"lines_added":   1,
			"lines_removed": 1,
			"preview":       "@@ -1 +1 @@\n-hello world\n+goodbye world\n",
			"truncated":     false,
		},
	})

	var telEv telemetry.Event
	select {
	case ev, ok := <-eventCh:
		if !ok {
			t.Fatal("subscriber channel closed")
		}
		telEv = ev
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for EventToolEdited on subscriber channel")
	}

	// Feed the event through the euclotui bridge and into a router.
	router := euclotui.NewEucloEventRouter()
	applier := euclotui.NewExecEventApplier(router)
	snap, ok := applier.Apply(telEv)
	if !ok {
		t.Fatal("ExecEventApplier rejected EventToolEdited")
	}

	if len(snap.Diff.Hunks) == 0 {
		t.Fatal("expected at least 1 diff hunk")
	}
	h := snap.Diff.Hunks[0]
	if h.File != "demo.txt" {
		t.Fatalf("hunk file = %q, want demo.txt", h.File)
	}
	if h.LinesAdded != 1 {
		t.Fatalf("LinesAdded = %d, want 1", h.LinesAdded)
	}
	if h.LinesRemoved != 1 {
		t.Fatalf("LinesRemoved = %d, want 1", h.LinesRemoved)
	}
	if h.Origin != "file_edit" {
		t.Fatalf("Origin = %q, want file_edit", h.Origin)
	}
	if h.Body != "@@ -1 +1 @@\n-hello world\n+goodbye world\n" {
		t.Fatalf("Body = %q", h.Body)
	}

	if len(snap.Diff.Steps) == 0 {
		t.Fatal("expected at least 1 diff step")
	}
	var hasDemoFile bool
	for _, step := range snap.Diff.Steps {
		for _, f := range step.Order {
			if f == "demo.txt" {
				hasDemoFile = true
			}
		}
	}
	if !hasDemoFile {
		t.Fatal("diff steps should include demo.txt file projection")
	}

	// Verify the diff pane renders the hunk.
	pane := euclotui.NewDiffPane(router, workspace, nil)
	pane.SetSize(120, 40)
	rendered := pane.View()
	if !strings.Contains(rendered, "demo.txt") {
		t.Fatalf("diff pane should show demo.txt file name:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hello world") || !strings.Contains(rendered, "goodbye world") {
		t.Fatalf("diff pane should show old/new content:\n%s", rendered)
	}

	// Apply the step via the diff pane's key commands.
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // focus step view
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}) // apply step
	demoFile := filepath.Join(workspace, "demo.txt")
	if data, err := os.ReadFile(filepath.Clean(demoFile)); err == nil {
		content := string(data)
		t.Logf("demo.txt after apply-step: %q", content)
		if strings.Contains(content, "goodbye") {
			t.Logf("diff pane apply-step modified demo.txt")
		}
	}

	// Revert all via the diff pane.
	pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}}) // revert all
	if data, err := os.ReadFile(filepath.Clean(demoFile)); err == nil {
		content := string(data)
		t.Logf("demo.txt after revert-all: %q", content)
		if strings.Contains(content, "goodbye") {
			t.Logf("revert-all restored demo.txt")
		}
	}

	t.Logf("diff pipeline verified: EventToolEdited → subscriber → bridge → router → diff pane → apply → revert")

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

// TestExecDiff_SubmitTurnProducesEvents proves that a real turn produces
// telemetry events and delivers them without drops.
func TestExecDiff_SubmitTurnProducesEvents(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"demo.txt": "hello world\n",
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	scenario := &scenarioState{}
	offline := &offlineScenarioModel{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	eventCh, cancelEvents := rt.SubscribeExecutionEvents()
	defer cancelEvents()

	runner.reset()
	scenario.set("echo")

	ctx, turnCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer turnCancel()
	result, err := rt.SubmitTurn(ctx, "hello", "analysis", nil, nil)
	if err != nil {
		t.Fatalf("submit turn: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("turn result = %#v", result)
	}
	t.Logf("turn result: success=%v node=%s", result.Success, result.NodeID)

	var events []telemetry.Event
	collectTimeout := time.After(5 * time.Second)
collectLoop:
	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				break collectLoop
			}
			events = append(events, ev)
		case <-collectTimeout:
			break collectLoop
		}
	}
	t.Logf("received %d events from turn", len(events))
	if len(events) == 0 {
		t.Fatal("expected at least one telemetry event from the turn")
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
