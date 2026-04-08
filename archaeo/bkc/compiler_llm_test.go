package bkc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
)

type staticModel struct {
	text string
}

func (m staticModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: m.text}, nil
}
func (m staticModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (m staticModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: m.text}, nil
}
func (m staticModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: m.text}, nil
}

func TestLLMCompilerProposeAndConfirm(t *testing.T) {
	store := newTestChunkStore(t)
	mem, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	ctx := context.Background()
	if err := mem.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypePlanning,
		Instruction: "phase 7 test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	compiler := &LLMCompiler{
		Store:         store,
		WorkflowStore: mem,
		Model:         staticModel{text: `{"title":"Ownership seam","summary":"Capture the seam","body":{"raw":"euclo owns execution; archaeo owns provenance","fields":{"kind":"decision"}},"views":[{"kind":"decision","data":{"summary":"split ownership"}}]}`},
		Now:           func() time.Time { return now },
		NewID:         func(prefix string) string { return prefix + "-1" },
		Learning:      archaeolearning.Service{Store: mem, Now: func() time.Time { return now }, NewID: func(prefix string) string { return prefix + "-1" }},
		Deferred:      archaeodeferred.Service{Store: mem, Now: func() time.Time { return now }, NewID: func(prefix string) string { return prefix + "-1" }},
	}
	proposed, err := compiler.Propose(ctx, LLMCompileInput{
		WorkspaceID:   "ws-1",
		WorkflowID:    "wf-1",
		ExplorationID: "exp-1",
		SubjectRef:    "ownership-seam",
		Prompt:        "Compile the approved ownership seam into a BKC chunk.",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if proposed.Candidate.Status != ChunkCandidatePending {
		t.Fatalf("expected pending candidate, got %+v", proposed.Candidate)
	}
	if proposed.Interaction.SubjectID != proposed.Candidate.ID {
		t.Fatalf("expected interaction subject id %q, got %q", proposed.Candidate.ID, proposed.Interaction.SubjectID)
	}
	candidate, result, err := compiler.ResolveCandidate(ctx, ResolveCandidateInput{
		WorkflowID:    "wf-1",
		InteractionID: proposed.Interaction.ID,
		Kind:          archaeolearning.ResolutionConfirm,
		ResolvedBy:    "operator",
	})
	if err != nil {
		t.Fatalf("resolve confirm: %v", err)
	}
	if candidate.Status != ChunkCandidateConfirmed {
		t.Fatalf("expected confirmed candidate, got %+v", candidate)
	}
	if len(result.ChunkIDs) != 1 {
		t.Fatalf("expected one compiled chunk, got %+v", result)
	}
	chunk, ok, err := store.Load(result.ChunkIDs[0])
	if err != nil || !ok || chunk == nil {
		t.Fatalf("load chunk: %v ok=%v", err, ok)
	}
	if chunk.Freshness != FreshnessValid || chunk.Provenance.CompiledBy != CompilerLLMAssisted {
		t.Fatalf("unexpected chunk lifecycle: %+v", chunk)
	}
}

func TestLLMCompilerRejectCreatesDeferredDraft(t *testing.T) {
	store := newTestChunkStore(t)
	mem, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	ctx := context.Background()
	if err := mem.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypePlanning,
		Instruction: "phase 7 test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	compiler := &LLMCompiler{
		Store:         store,
		WorkflowStore: mem,
		Model:         staticModel{text: `{"title":"Chunk","summary":"candidate","body":{"raw":"candidate raw"}}`},
		Now:           func() time.Time { return now },
		NewID:         func(prefix string) string { return prefix + "-2" },
		Learning:      archaeolearning.Service{Store: mem, Now: func() time.Time { return now }, NewID: func(prefix string) string { return prefix + "-2" }},
		Deferred:      archaeodeferred.Service{Store: mem, Now: func() time.Time { return now }, NewID: func(prefix string) string { return prefix + "-2" }},
	}
	proposed, err := compiler.Propose(ctx, LLMCompileInput{
		WorkspaceID:   "ws-2",
		WorkflowID:    "wf-2",
		ExplorationID: "exp-2",
		Prompt:        "Compile",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	candidate, _, err := compiler.ResolveCandidate(ctx, ResolveCandidateInput{
		WorkflowID:    "wf-2",
		InteractionID: proposed.Interaction.ID,
		Kind:          archaeolearning.ResolutionReject,
		ResolvedBy:    "operator",
	})
	if err != nil {
		t.Fatalf("resolve reject: %v", err)
	}
	if candidate.Status != ChunkCandidateRejected {
		t.Fatalf("expected rejected candidate, got %+v", candidate)
	}
	records, err := (archaeodeferred.Service{Store: mem}).ListByWorkspace(ctx, "ws-2")
	if err != nil {
		t.Fatalf("list deferred: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one deferred record, got %d", len(records))
	}
}
