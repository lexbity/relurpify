package envcomposition

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

// TestSkipASTIndexGatesIndexing is FR-12: SkipASTIndex=true prevents the AST
// index from starting; the flag finally reaches the composer and changes
// behavior. Rankings degrade deterministically to the remaining rankers.
func TestSkipASTIndexGatesIndexing(t *testing.T) {
	runner, err := testsupport.NewAuthorizedFakeRunner(testsupport.PermitAllPolicy())
	if err != nil {
		t.Fatal(err)
	}

	started := 0
	original := startIndexingFn
	startIndexingFn = func(_ *ast.IndexManager, _ context.Context) error {
		started++
		return nil
	}
	t.Cleanup(func() { startIndexingFn = original })

	ctx := context.Background()
	if _, err := BuildCapabilityRuntime(ctx, t.TempDir(), runner, CapabilityRuntimeOptions{AgentID: "flag-off"}); err != nil {
		t.Fatalf("BuildCapabilityRuntime (indexing on): %v", err)
	}
	if started != 1 {
		t.Fatalf("indexing starts = %d, want 1 (default boots index)", started)
	}

	if _, err := BuildCapabilityRuntime(ctx, t.TempDir(), runner, CapabilityRuntimeOptions{AgentID: "flag-on", SkipASTIndex: true}); err != nil {
		t.Fatalf("BuildCapabilityRuntime (SkipASTIndex): %v", err)
	}
	if started != 1 {
		t.Fatalf("SkipASTIndex must not start the index; starts = %d", started)
	}
}
