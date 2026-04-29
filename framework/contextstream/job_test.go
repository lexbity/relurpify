package contextstream

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/compiler"
)

func TestRequestBackgroundCompletesJob(t *testing.T) {
	trigger := NewTrigger(&fakeCompiler{
		result: &compiler.CompilationResult{},
		record: &compiler.CompilationRecord{},
	})
	job, err := trigger.RequestBackground(context.Background(), Request{ID: "job-1", Mode: ModeBackground})
	if err != nil {
		t.Fatalf("RequestBackground returned error: %v", err)
	}
	result, err := job.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Request.ID != "job-1" {
		t.Fatalf("unexpected request id: %q", result.Request.ID)
	}
	select {
	case <-job.Done():
	default:
		t.Fatal("expected job to be done")
	}
}
