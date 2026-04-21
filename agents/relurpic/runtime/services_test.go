package runtime

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/archaeo/providers"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
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

func TestPatternSurfacingFuncReturnsError(t *testing.T) {
	expectedErr := errors.New("pattern error")
	svc := PatternSurfacingFunc(func(_ context.Context, _ providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
		return nil, expectedErr
	})
	records, err := svc.SurfacePatterns(context.Background(), providers.PatternSurfacingRequest{})
	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if records != nil && len(records) > 0 {
		t.Fatalf("expected no records, got %#v", records)
	}
}

func TestConvergenceReviewFuncReturnsFailure(t *testing.T) {
	failure := &frameworkplan.ConvergenceFailure{}
	svc := ConvergenceReviewFunc(func(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
		return failure, nil
	})
	result, err := svc.ReviewConvergence(context.Background(), providers.ConvergenceReviewRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != failure {
		t.Fatalf("expected returned failure %v, got %v", failure, result)
	}
}
