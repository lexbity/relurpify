package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/archaeo/providers"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestPatternSurfacingFuncDelegates(t *testing.T) {
	called := false
	svc := PatternSurfacingFunc(func(_ context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
		called = true
		if req.WorkflowID != "wf-1" {
			t.Fatalf("unexpected request: %#v", req)
		}
		return []patterns.PatternRecord{{ID: "pattern-1"}}, nil
	})
	records, err := svc.SurfacePatterns(context.Background(), providers.PatternSurfacingRequest{WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called || len(records) != 1 || records[0].ID != "pattern-1" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestConvergenceReviewFuncPropagatesError(t *testing.T) {
	svc := ConvergenceReviewFunc(func(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
		return nil, errors.New("boom")
	})
	_, err := svc.ReviewConvergence(context.Background(), providers.ConvergenceReviewRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}
