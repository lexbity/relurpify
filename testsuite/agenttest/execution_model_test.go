package agenttest

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestExecutorBuildsModelRuntimeOff(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.model == nil {
		t.Fatal("model runtime is nil after Execute")
	}
	if exec.model.Backend == nil {
		t.Fatal("model.Backend is nil")
	}
	if exec.model.ModelFactory == nil {
		t.Fatal("model.ModelFactory is nil")
	}
}

func TestExecutorBuildsModelRuntimeReplay(t *testing.T) {
	ws := t.TempDir()
	tapeDir := t.TempDir()
	tapePath := filepath.Join(tapeDir, "tape.jsonl")
	tapeContent := `{"timestamp":"2026-01-01T00:00:00Z","kind":"_header","request":{"header":{"provider_id":"tape","model_name":"gemma4:e4b"}}}`
	if err := fs.WriteFileSecure(tapePath, []byte(tapeContent)); err != nil {
		t.Fatal(err)
	}

	desc := validDescriptorWithWorkspace(t, ws)
	desc.RecordingMode = "replay"
	desc.TapePath = tapePath
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.model == nil {
		t.Fatal("model runtime is nil after Execute")
	}
	if exec.model.Backend == nil {
		t.Fatal("model.Backend is nil")
	}
	if exec.model.ModelFactory == nil {
		t.Fatal("model.ModelFactory is nil")
	}
}

func TestExecutorModelTapePathEmptyForOff(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	desc.RecordingMode = "off"
	desc.TapePath = ""
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.model == nil {
		t.Fatal("model runtime is nil after Execute")
	}
	if exec.model.Backend == nil {
		t.Fatal("model.Backend is nil")
	}
}

func TestExecutorCleanupClosesBackend(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	exec.cleanup()

	if exec.model != nil && exec.model.Backend != nil {
		err := exec.model.Backend.Close()
		_ = err
	}
}
