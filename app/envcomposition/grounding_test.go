package envcomposition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// TestWireGroundingSetsBothFields is D-5: the single composition sets the
// graph's grounder and the barrier's drain from one knowledge runtime.
func TestWireGroundingSetsBothFields(t *testing.T) {
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(ctx)) })
	kr, err := BuildKnowledgeRuntime(KnowledgeRuntimeInput{GraphDB: engine})
	require.NoError(t, err)
	t.Cleanup(kr.Close)

	deps := &paradigm.Deps{}
	wired := WireGrounding(deps, kr)
	if wired != deps {
		t.Fatal("WireGrounding must return the same deps")
	}
	if deps.Grounder == nil {
		t.Fatal("Grounder not wired")
	}
	if deps.EpochDrain == nil {
		t.Fatal("EpochDrain not wired")
	}
}

// TestWireGroundingNilKnowledgeNoOp: callers that legitimately run without
// knowledge pass nil and keep their deps untouched.
func TestWireGroundingNilKnowledgeNoOp(t *testing.T) {
	deps := &paradigm.Deps{}
	if wired := WireGrounding(deps, nil); wired != deps {
		t.Fatal("nil knowledge runtime must return deps unchanged")
	}
	if deps.Grounder != nil || deps.EpochDrain != nil {
		t.Fatalf("nil knowledge runtime must not touch deps: %+v", deps)
	}
	if wired := WireGrounding(nil, &KnowledgeRuntime{}); wired != nil {
		t.Fatal("nil deps must return nil")
	}
}
