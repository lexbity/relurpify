package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/execution"
	tsevent "codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestBootTurnEventLogReceivesCausalRecord verifies the composition-root
// wiring end to end (FR-4): a booted runtime's causal record at
// .relurpify_state/events.db receives canonical framework `.v1` events —
// agent.run.started, policy.evaluated — recorded with sequences and the
// workspace partition, reusable after Close (reopen reads them back).
func TestBootTurnEventLogReceivesCausalRecord(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})
	testhelper.InitGitRepo(t, workspace)

	eventsPath := filepath.Join(workspace, ".relurpify_state", "events.db")

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = rt.SubmitTurn(ctx, "review", execution.TaskTypeAnalysis, nil, nil)
	if err != nil {
		t.Fatalf("submit turn: %v", err)
	}
	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	// Reopen the store the composition root created and assert the causal
	// record actually has the lifecycle-evidence events in it.
	log, err := tsevent.NewBadgerLog(eventsPath)
	if err != nil {
		t.Fatalf("reopen events.db: %v", err)
	}
	defer func() {
		if err := log.Close(); err != nil {
			t.Fatalf("close reopened event log: %v", err)
		}
	}()

	gotStarted, err := log.ReadByType(context.Background(), "local", "agent.run.started", 0, 10)
	if err != nil {
		t.Fatalf("read agent.run.started: %v", err)
	}
	if len(gotStarted) == 0 {
		t.Fatalf("event log must contain agent.run.started.v1 after a boot; got 0 events")
	}
	for _, ev := range gotStarted {
		if ev.Seq == 0 {
			t.Fatalf("agent.run.started attributed sequence 0")
		}
		if ev.Partition != "local" {
			t.Fatalf("agent.run.started recorded in unexpected partition %q", ev.Partition)
		}
	}

	policyEvents, err := log.ReadByType(context.Background(), "local", "policy.evaluated", 0, 0)
	if err != nil {
		t.Fatalf("read policy.evaluated: %v", err)
	}
	if len(policyEvents) == 0 {
		t.Fatalf("event log must contain policy.evaluated.v1; got 0 events")
	}
}
