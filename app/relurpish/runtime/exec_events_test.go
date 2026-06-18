package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestExecEvents_SubscriberReceivesEvents proves that events emitted through
// the runtime's telemetry chain reach a SubscribeExecutionEvents subscriber.
func TestExecEvents_SubscriberReceivesEvents(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	cfg := ConfigForWorkspace(Config{AgentName: AgentLabelEuclo}, workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}

	ch, cancel := rt.SubscribeExecutionEvents()
	t.Cleanup(func() {
		cancel()
		_ = rt.Close(context.Background())
	})

	// Emit a test event through the workspace telemetry chain. This simulates
	// what the euclo agent does when it calls EucloTelemetry.Emit() — the event
	// flows through ws.Telemetry (which now includes the BroadcastSink).
	if rt.Workspace != nil && rt.Workspace.Telemetry != nil {
		rt.Workspace.Telemetry.Emit(telemetry.Event{
			Type:      "euclo.step.started",
			TaskID:    "test-task",
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]any{"step_id": "test-step", "index": 1, "total": 3},
		})
	}

	// Read the event from the subscriber channel.
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("subscriber channel closed unexpectedly")
		}
		if ev.Type != "euclo.step.started" {
			t.Fatalf("expected euclo.step.started, got %s", ev.Type)
		}
		if ev.TaskID != "test-task" {
			t.Fatalf("expected TaskID test-task, got %s", ev.TaskID)
		}
		stepID, _ := ev.Metadata["step_id"].(string)
		if stepID != "test-step" {
			t.Fatalf("expected step_id test-step, got %s", stepID)
		}
		t.Logf("received event: type=%s task=%s step=%s", ev.Type, ev.TaskID, stepID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event on subscriber channel")
	}

	// Emit a second event with pre-set Seq.
	rt.Workspace.Telemetry.Emit(telemetry.Event{
		Type:      "euclo.step.completed",
		TaskID:    "test-task",
		Seq:       42,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"step_id": "test-step", "success": true},
	})
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("subscriber channel closed unexpectedly")
		}
		if ev.Type != "euclo.step.completed" {
			t.Fatalf("expected euclo.step.completed, got %s", ev.Type)
		}
		if ev.Seq != 42 {
			t.Fatalf("expected Seq 42 preserved, got %d", ev.Seq)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for second event")
	}
}

// TestExecEvents_EmitThroughWorkspaceTelemetry proves that events emitted
// through the workspace telemetry chain (as the euclo agent does) reach
// a SubscribeExecutionEvents subscriber. This validates the end-to-end
// wiring: buildRuntime registers execSink into ws.Telemetry so that any
// EucloTelemetry.Emit call is observable via SubscribeExecutionEvents.
func TestExecEvents_EmitThroughWorkspaceTelemetry(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	cfg := ConfigForWorkspace(Config{AgentName: AgentLabelEuclo}, workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}

	ch, cancel := rt.SubscribeExecutionEvents()
	t.Cleanup(func() {
		cancel()
		_ = rt.Close(context.Background())
	})

	eucloTypes := []string{
		"euclo.recipe.selected",
		"euclo.step.started",
		"euclo.step.completed",
		"euclo.branch.resolved",
		"euclo.intake.complete",
		"euclo.route.selected",
		"euclo.projection.completed",
	}
	for _, typ := range eucloTypes {
		rt.Workspace.Telemetry.Emit(telemetry.Event{
			Type:      telemetry.EventType(typ),
			TaskID:    "test-task",
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]any{"step_id": "s1", "index": 1, "total": 3, "success": true},
		})
	}

	for i := 0; i < len(eucloTypes); i++ {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("subscriber channel closed after %d events", i)
			}
			if !strings.HasPrefix(string(ev.Type), "euclo.") {
				t.Errorf("expected euclo.* event, got %s", ev.Type)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for event %d/%d", i+1, len(eucloTypes))
		}
	}

	t.Logf("received all %d euclo event types via workspace telemetry chain", len(eucloTypes))
}

// TestExecEvents_ExecSinkDirectEmit proves that events emitted directly
// through execSink (as the euclo agent would via EucloTelemetry) are
// delivered to SubscribeExecutionEvents subscribers.
func TestExecEvents_ExecSinkDirectEmit(t *testing.T) {
	rt := &Runtime{
		execSink: telemetry.NewBroadcastSink(),
	}

	ch, cancel := rt.SubscribeExecutionEvents()
	defer cancel()

	rt.execSink.Emit(telemetry.Event{
		Type:      "euclo.step.started",
		TaskID:    "direct-task",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"step_id": "test-step", "index": 1, "total": 3},
	})

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("subscriber channel closed unexpectedly")
		}
		if ev.Type != "euclo.step.started" {
			t.Fatalf("expected euclo.step.started, got %s", ev.Type)
		}
		stepID, _ := ev.Metadata["step_id"].(string)
		if stepID != "test-step" {
			t.Fatalf("expected step_id test-step, got %s", stepID)
		}
		t.Logf("received event: type=%s task=%s step=%s", ev.Type, ev.TaskID, stepID)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestExecEvents_NilSafety verifies SubscribeExecutionEvents handles nil Runtime
// and nil execSink without panic.
func TestExecEvents_NilSafety(t *testing.T) {
	var nilRT *Runtime

	ch, cancel := nilRT.SubscribeExecutionEvents()
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel from nil Runtime")
	}
	cancel()

	rt := &Runtime{}
	ch2, cancel2 := rt.SubscribeExecutionEvents()
	_, ok2 := <-ch2
	if ok2 {
		t.Fatal("expected closed channel when execSink is nil")
	}
	cancel2()
}

// TestExecEvents_CancelCloseOrder verifies cancel then Close is clean.
func TestExecEvents_CancelCloseOrder(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	cfg := ConfigForWorkspace(Config{AgentName: AgentLabelEuclo}, workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}

	ch, cancel := rt.SubscribeExecutionEvents()
	cancel()
	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime after cancel: %v", err)
	}

	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after cancel")
	}
}

// TestExecEvents_SubscribeAfterClose verifies SubscribeExecutionEvents returns
// a closed channel after Runtime.Close.
func TestExecEvents_SubscribeAfterClose(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	cfg := ConfigForWorkspace(Config{AgentName: AgentLabelEuclo}, workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}

	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	ch, cancel := rt.SubscribeExecutionEvents()
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel after Runtime.Close")
	}
	cancel()
}
