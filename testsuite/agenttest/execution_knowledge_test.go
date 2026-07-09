package agenttest

import (
	"context"
	"io"
	"testing"
)

func TestExecutorBuildsKnowledgeRuntime(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.knowledge == nil {
		t.Fatal("knowledge runtime is nil after Execute")
	}
	if exec.knowledge.KnowledgeStore == nil {
		t.Fatal("knowledge.KnowledgeStore is nil")
	}
	if exec.knowledge.KnowledgeEvents == nil {
		t.Fatal("knowledge.KnowledgeEvents is nil")
	}
	if exec.knowledge.Retriever == nil {
		t.Fatal("knowledge.Retriever is nil")
	}
	if exec.knowledge.Compiler == nil {
		t.Fatal("knowledge.Compiler is nil")
	}
	if exec.knowledge.StreamTrigger == nil {
		t.Fatal("knowledge.StreamTrigger is nil")
	}
}

func TestExecutorKnowledgeReusesCapabilityGraphDB(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	capDB := exec.capability.IndexManager.GraphDB
	knowDB := exec.knowledge.KnowledgeStore.Graph

	if capDB == nil {
		t.Fatal("capability.IndexManager.GraphDB is nil")
	}
	if knowDB == nil {
		t.Fatal("knowledge.KnowledgeStore.Graph is nil")
	}
	if capDB != knowDB {
		t.Fatal("capability and knowledge GraphDB pointers differ — expected single shared engine")
	}
}

func TestExecutorCleanupAfterKnowledgeDoesNotDoubleClose(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	exec.cleanup()

	if exec.capability != nil && exec.capability.IndexManager != nil {
		err := exec.capability.IndexManager.Close(context.Background())
		_ = err
	}
}
