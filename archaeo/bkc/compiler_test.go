package bkc

import (
	"context"
	"math"
	"sort"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	frameworkpatterns "github.com/lexcodex/relurpify/framework/patterns"
	frameworkretrieval "github.com/lexcodex/relurpify/framework/retrieval"
)

func TestCompilerPatternConfirmationProducesDeterministicValidChunk(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	result, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			BasedOnRevision: "rev-1",
			Pattern: frameworkpatterns.PatternRecord{
				ID:          "pattern-1",
				Kind:        frameworkpatterns.PatternKindBoundary,
				Title:       "Boundary pattern",
				Description: "Important boundary",
				Status:      frameworkpatterns.PatternStatusConfirmed,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile pattern: %v", err)
	}
	if len(result.ChunkIDs) != 1 {
		t.Fatalf("expected one chunk id, got %+v", result)
	}
	chunk, ok, err := store.Load(result.ChunkIDs[0])
	if err != nil || !ok {
		t.Fatalf("load chunk: %v ok=%v", err, ok)
	}
	if chunk.Provenance.CompiledBy != CompilerDeterministic || chunk.Freshness != FreshnessValid {
		t.Fatalf("unexpected chunk lifecycle: %+v", chunk)
	}
	if len(chunk.Provenance.Sources) != 1 || chunk.Provenance.Sources[0].Kind != "pattern_confirmation" {
		t.Fatalf("unexpected provenance sources: %+v", chunk.Provenance.Sources)
	}
}

func TestCompilerAnchorConfirmationWritesCodeStateEdge(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	result, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputAnchorConfirmation,
		AnchorConfirmed: &AnchorConfirmationInput{
			WorkspaceID: "ws",
			WorkflowID:  "wf",
			Anchor: frameworkretrieval.AnchorRecord{
				AnchorID:        "anchor-1",
				Term:            "ownership",
				Definition:      "owned here",
				SourceVersionID: "git-sha-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile anchor: %v", err)
	}
	chunkID := result.ChunkIDs[0]
	edges, err := store.LoadEdgesFrom(chunkID, EdgeKindDependsOnCodeState)
	if err != nil {
		t.Fatalf("load code-state edge: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected one code-state edge, got %+v", edges)
	}
	if edges[0].Meta["code_state_ref"] != "git-sha-1" {
		t.Fatalf("unexpected code-state meta: %+v", edges[0])
	}
}

func TestCompilerASTIndexEntryStoresStructuralContent(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	result, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputASTIndexEntry,
		IndexEntryProduced: &ASTIndexEntryInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			EntryID:         "entry-1",
			FilePath:        "service.go",
			SymbolID:        "service.Run",
			Summary:         "service.Run orchestrates execution",
			Kind:            "function",
			BasedOnRevision: "rev-ast",
		},
	})
	if err != nil {
		t.Fatalf("compile ast entry: %v", err)
	}
	chunk, ok, err := store.Load(result.ChunkIDs[0])
	if err != nil || !ok {
		t.Fatalf("load ast chunk: %v ok=%v", err, ok)
	}
	if chunk.Body.Fields["file_path"] != "service.go" || chunk.Body.Fields["symbol_id"] != "service.Run" {
		t.Fatalf("unexpected ast chunk body: %+v", chunk.Body.Fields)
	}
}

func TestCompilerRelatedPatternWritesRequiresContextEdge(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	first, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID: "ws",
			WorkflowID:  "wf",
			Pattern: frameworkpatterns.PatternRecord{
				ID:          "pattern-a",
				Title:       "A",
				Description: "A desc",
				Status:      frameworkpatterns.PatternStatusConfirmed,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile first pattern: %v", err)
	}
	second, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			Pattern:         frameworkpatterns.PatternRecord{ID: "pattern-b", Title: "B", Description: "B desc", Status: frameworkpatterns.PatternStatusConfirmed},
			RelatedChunkIDs: []ChunkID{first.ChunkIDs[0]},
		},
	})
	if err != nil {
		t.Fatalf("compile second pattern: %v", err)
	}
	edges, err := store.LoadEdgesFrom(second.ChunkIDs[0], EdgeKindRequiresContext)
	if err != nil {
		t.Fatalf("load requires_context edges: %v", err)
	}
	if len(edges) != 1 || edges[0].ToChunk != first.ChunkIDs[0] {
		t.Fatalf("unexpected requires_context edges: %+v", edges)
	}
}

func TestCompilerPatternConfirmationCreatesAmplifiesEdges(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}

	first, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID: "ws",
			WorkflowID:  "wf",
			Pattern: frameworkpatterns.PatternRecord{
				ID:          "pattern-amp-1",
				Title:       "A",
				Description: "A desc",
				Status:      frameworkpatterns.PatternStatusConfirmed,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile first pattern: %v", err)
	}
	second, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID: "ws",
			WorkflowID:  "wf",
			Pattern: frameworkpatterns.PatternRecord{
				ID:          "pattern-amp-2",
				Title:       "B",
				Description: "B desc",
				Status:      frameworkpatterns.PatternStatusConfirmed,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile second pattern: %v", err)
	}
	third, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			Pattern:         frameworkpatterns.PatternRecord{ID: "pattern-amp-3", Title: "C", Description: "C desc", Status: frameworkpatterns.PatternStatusConfirmed},
			AmplifyChunkIDs: []ChunkID{first.ChunkIDs[0], second.ChunkIDs[0]},
		},
	})
	if err != nil {
		t.Fatalf("compile third pattern: %v", err)
	}
	edges, err := store.LoadEdgesFrom(third.ChunkIDs[0], EdgeKindAmplifies)
	if err != nil {
		t.Fatalf("load amplifies edges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected two amplifies edges, got %+v", edges)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Weight > edges[j].Weight })
	if edges[0].ToChunk != first.ChunkIDs[0] || edges[1].ToChunk != second.ChunkIDs[0] {
		t.Fatalf("unexpected amplifies targets: %+v", edges)
	}
	if math.Abs(edges[0].Weight-0.9) > 0.0001 || math.Abs(edges[1].Weight-0.8) > 0.0001 {
		t.Fatalf("unexpected amplifies weights: %+v", edges)
	}
}

func TestCompilerRecompileWritesSupersedesEdge(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	input := CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			BasedOnRevision: "rev-1",
			Pattern: frameworkpatterns.PatternRecord{
				ID:          "pattern-supersede",
				Title:       "Versioned",
				Description: "desc",
				Status:      frameworkpatterns.PatternStatusConfirmed,
			},
		},
	}
	first, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	input.PatternConfirmed.BasedOnRevision = "rev-2"
	second, err := compiler.Compile(context.Background(), input)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	chunk, ok, err := store.Load(second.ChunkIDs[0])
	if err != nil || !ok {
		t.Fatalf("load second chunk: %v ok=%v", err, ok)
	}
	if chunk.Version != 2 {
		t.Fatalf("expected version 2, got %d", chunk.Version)
	}
	edges, err := store.LoadEdgesFrom(second.ChunkIDs[0], EdgeKindSupersedes)
	if err != nil {
		t.Fatalf("load supersedes edges: %v", err)
	}
	if len(edges) != 1 || edges[0].ToChunk != first.ChunkIDs[0] {
		t.Fatalf("unexpected supersedes edges: %+v", edges)
	}
}

func TestCompilerAmplifyWeightsClampAtFloor(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}

	amplifies := make([]ChunkID, 0, 12)
	for i := 0; i < 12; i++ {
		result, err := compiler.Compile(context.Background(), CompilerInput{
			Kind: CompilerInputPatternConfirmation,
			PatternConfirmed: &PatternConfirmationInput{
				WorkspaceID: "ws",
				WorkflowID:  "wf",
				Pattern: frameworkpatterns.PatternRecord{
					ID:          "pattern-floor-" + string(rune('a'+i)),
					Title:       "X",
					Description: "desc",
					Status:      frameworkpatterns.PatternStatusConfirmed,
				},
			},
		})
		if err != nil {
			t.Fatalf("compile chunk %d: %v", i, err)
		}
		amplifies = append(amplifies, result.ChunkIDs[0])
	}
	last, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputPatternConfirmation,
		PatternConfirmed: &PatternConfirmationInput{
			WorkspaceID:     "ws",
			WorkflowID:      "wf",
			Pattern:         frameworkpatterns.PatternRecord{ID: "pattern-floor-target", Title: "T", Description: "target", Status: frameworkpatterns.PatternStatusConfirmed},
			AmplifyChunkIDs: amplifies,
		},
	})
	if err != nil {
		t.Fatalf("compile target: %v", err)
	}
	edges, err := store.LoadEdgesFrom(last.ChunkIDs[0], EdgeKindAmplifies)
	if err != nil {
		t.Fatalf("load amplifies edges: %v", err)
	}
	if len(edges) != 12 {
		t.Fatalf("expected 12 amplifies edges, got %d", len(edges))
	}
	foundFloor := false
	for _, edge := range edges {
		if math.Abs(edge.Weight-0.1) <= 0.0001 {
			foundFloor = true
			break
		}
	}
	if !foundFloor {
		t.Fatalf("expected at least one floor weight of 0.1, got %+v", edges)
	}
}

func TestCompilerBusSubscriptionCompilesPatternEvent(t *testing.T) {
	store := newTestChunkStore(t)
	bus := &EventBus{}
	compiler := &Compiler{Store: store, EventBus: bus}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := compiler.Start(ctx); err != nil {
		t.Fatalf("start compiler: %v", err)
	}
	defer compiler.Stop()
	bus.Publish(Event{
		Kind: EventPatternConfirmed,
		Payload: CompilerInput{
			Kind: CompilerInputPatternConfirmation,
			PatternConfirmed: &PatternConfirmationInput{
				WorkspaceID: "ws",
				WorkflowID:  "wf",
				Pattern: frameworkpatterns.PatternRecord{
					ID:          "pattern-bus",
					Title:       "Bus",
					Description: "bus pattern",
					Status:      frameworkpatterns.PatternStatusConfirmed,
				},
			},
		},
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		chunks, err := store.FindByWorkspace("ws")
		if err != nil {
			t.Fatalf("find chunks: %v", err)
		}
		if len(chunks) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected pattern event to be compiled")
}

func TestCompilerUnknownInputKindErrorsWithoutPartialState(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	if _, err := compiler.Compile(context.Background(), CompilerInput{Kind: "mystery"}); err == nil {
		t.Fatal("expected error for unknown compiler input")
	}
	chunks, err := store.FindByWorkspace("ws")
	if err != nil {
		t.Fatalf("find chunks: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected no partial state, got %+v", chunks)
	}
}

func TestCompilerUserStatementDeterministicChunk(t *testing.T) {
	store := newTestChunkStore(t)
	compiler := &Compiler{Store: store}
	result, err := compiler.Compile(context.Background(), CompilerInput{
		Kind: CompilerInputUserStatement,
		UserStatement: &UserStatementInput{
			WorkspaceID: "ws",
			WorkflowID:  "wf",
			Statement:   "the module boundary is intentional",
			Interaction: archaeolearning.Interaction{
				ID:          "interaction-1",
				Title:       "Intent confirmed",
				SubjectType: archaeolearning.SubjectExploration,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile user statement: %v", err)
	}
	chunk, ok, err := store.Load(result.ChunkIDs[0])
	if err != nil || !ok {
		t.Fatalf("load user statement chunk: %v ok=%v", err, ok)
	}
	if chunk.Provenance.Sources[0].Kind != "user_statement" {
		t.Fatalf("unexpected provenance: %+v", chunk.Provenance)
	}
}
