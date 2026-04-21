package context_test

import (
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclocontext "codeburg.org/lexbit/relurpify/named/euclo/runtime/context"
)

func TestApplyEditIntentArtifacts_NilInputsDoNotPanic(t *testing.T) {
	euclocontext.ApplyEditIntentArtifacts(nil, nil)
}

func TestApplyEditIntentArtifacts_CopiesEditArtifacts(t *testing.T) {
	src := core.NewContext()
	dst := core.NewContext()

	editExecution := eucloruntime.EditExecutionRecord{
		Summary: "requested=1 executed=1",
	}
	pipelineCode := map[string]any{
		"edits": []any{
			map[string]any{"path": "main.go", "action": "update", "content": "package main"},
		},
	}
	src.Set("pipeline.code", pipelineCode)
	src.Set("euclo.edit_execution", editExecution)

	euclocontext.ApplyEditIntentArtifacts(src, dst)

	if got, ok := dst.Get("pipeline.code"); !ok || !reflect.DeepEqual(got, pipelineCode) {
		t.Fatalf("expected pipeline.code to be copied, got %#v (ok=%v)", got, ok)
	}
	if got, ok := dst.Get("euclo.edit_execution"); !ok || !reflect.DeepEqual(got, editExecution) {
		t.Fatalf("expected euclo.edit_execution to be copied, got %#v (ok=%v)", got, ok)
	}
}

func TestBuildContextLifecycleState_IsCallable(t *testing.T) {
	if euclocontext.BuildContextLifecycleState == nil {
		t.Fatal("expected BuildContextLifecycleState to be re-exported")
	}
}
