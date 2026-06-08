package contextstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

type fakeCompiler struct {
	request contextports.CompilationRequest
	result  *contextports.CompilationResult
	err     error
}

func (f *fakeCompiler) Compile(ctx context.Context, request contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	f.request = request
	return f.result, f.err
}

func TestRequestBlockingAppliesCompilationResult(t *testing.T) {
	comp := &fakeCompiler{
		result: &contextports.CompilationResult{
			StreamedRefs:    []string{"chunk-1"},
			ShortfallTokens: 5,
			Substitutions: []contextports.SummarySubstitution{
				{Original: "chunk-2", Replaced: "sum-2", ChunkID: "chunk-2"},
			},
			Record: contextports.CompilationRecord{
				OriginalBudget: 64,
			},
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
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
	if result.Trim.ShortfallTokens <= 0 {
		t.Fatal("expected trimmed result (shortfall)")
	}
	if result.Trim.ShortfallTokens != 5 {
		t.Fatalf("expected shortfall 5, got %d", result.Trim.ShortfallTokens)
	}
	if comp.request.BudgetTokens != 64 {
		t.Fatalf("unexpected compiler request budget: %d", comp.request.BudgetTokens)
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
