package contextstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type fakeCompiler struct {
	request compiler.CompilationRequest
	result  *compiler.CompilationResult
	record  *compiler.CompilationRecord
	err     error
}

func (f *fakeCompiler) Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	f.request = request
	return f.result, f.record, f.err
}

func TestRequestBlockingAppliesCompilationResult(t *testing.T) {
	comp := &fakeCompiler{
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{
				{ChunkID: "chunk-1", Rank: 1},
			},
			ShortfallTokens: 5,
			Substitutions: []compiler.SummarySubstitution{
				{OriginalChunkID: knowledge.ChunkID("chunk-2"), SummaryChunkID: knowledge.ChunkID("sum-2"), Reason: "budget_pressure", TokenSavings: 7},
			},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "comp-1"},
		},
	}
	trigger := NewTrigger(comp)
	req := Request{
		ID:                    "req-1",
		Query:                 retrieval.RetrievalQuery{Text: "context"},
		MaxTokens:             64,
		EventLogSeq:           7,
		BudgetShortfallPolicy: "emit_partial",
		Mode:                  ModeBlocking,
		RequestedAt:           time.Unix(10, 0).UTC(),
	}
	result, err := trigger.RequestBlocking(context.Background(), req)
	if err != nil {
		t.Fatalf("RequestBlocking returned error: %v", err)
	}
	if result == nil || result.Record == nil {
		t.Fatalf("expected result and record, got %+v", result)
	}
	if !result.Trim.Truncated {
		t.Fatal("expected trimmed result")
	}
	if result.Trim.ShortfallTokens != 5 {
		t.Fatalf("expected shortfall 5, got %d", result.Trim.ShortfallTokens)
	}
	if result.Record.AssemblyMetadata.CompilationID != "comp-1" {
		t.Fatalf("unexpected record metadata: %+v", result.Record.AssemblyMetadata)
	}
	if comp.request.MaxTokens != 64 || comp.request.EventLogSeq != 7 {
		t.Fatalf("unexpected compiler request: %+v", comp.request)
	}
}

func TestRequestBlockingReturnsError(t *testing.T) {
	comp := &fakeCompiler{err: errors.New("boom")}
	trigger := NewTrigger(comp)
	_, err := trigger.RequestBlocking(context.Background(), Request{ID: "req-2"})
	if err == nil {
		t.Fatal("expected error")
	}
}
