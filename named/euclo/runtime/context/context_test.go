package context_test

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	euclocontext "github.com/lexcodex/relurpify/named/euclo/runtime/context"
)

// ApplyEditIntentArtifacts is currently a no-op stub. This test confirms it
// is callable without panicking and satisfies coverage for the statement.
func TestApplyEditIntentArtifacts_NoopDoesNotPanic(t *testing.T) {
	ctx := core.NewContext()
	state := core.NewContext()
	euclocontext.ApplyEditIntentArtifacts(ctx, state)
}

func TestApplyEditIntentArtifacts_NilInputsDoNotPanic(t *testing.T) {
	euclocontext.ApplyEditIntentArtifacts(nil, nil)
}

// Smoke-test that the re-exported BuildContextLifecycleState function is
// callable through this package.
func TestBuildContextLifecycleState_IsCallable(t *testing.T) {
	_ = euclocontext.BuildContextLifecycleState
}
