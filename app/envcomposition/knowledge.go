package envcomposition

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/execution/compiler"
)

// KnowledgeRuntime bundles the knowledge, retrieval, and compilation products.
// The composition root owns the compiler's lifecycle: Close stops it.
type KnowledgeRuntime struct {
	KnowledgeStore  *knowledge.ChunkStore
	KnowledgeEvents *knowledge.EventBus
	Retriever       *retrieval.Retriever
	Compiler        *compiler.Compiler
	StreamTrigger   *contextstream.Trigger

	closeRetriever func()
}

// Close stops the knowledge runtime's owned lifecycles (the compiler's
// invalidation loop and event subscription). Safe to call more than once.
func (k *KnowledgeRuntime) Close() {
	if k == nil {
		return
	}
	if k.closeRetriever != nil {
		k.closeRetriever()
	}
	if k.Compiler != nil {
		k.Compiler.Stop()
	}
}

// KnowledgeRuntimeInput carries parameters for BuildKnowledgeRuntime.
type KnowledgeRuntimeInput struct {
	GraphDB *graphdb.Engine
	Index   *ast.IndexManager
}

// BuildKnowledgeRuntime assembles knowledge store, retriever, and compiler.
func BuildKnowledgeRuntime(input KnowledgeRuntimeInput) (*KnowledgeRuntime, error) {
	if input.GraphDB == nil {
		return nil, fmt.Errorf("graphdb engine required")
	}
	bkcEvents := &knowledge.EventBus{}
	knowledgeStore := &knowledge.ChunkStore{Graph: input.GraphDB}
	rankerRegistry := retrieval.NewRankerRegistry()
	rankerRegistry.Register(&retrieval.KeywordRanker{K1: 1.2, B: 0.75})
	rankerRegistry.Register(&retrieval.RecencyRanker{HalfLifeHours: 24.0})
	if input.Index != nil {
		rankerRegistry.Register(&retrieval.ASTProximityRanker{Index: input.Index})
	}
	rankerRegistry.Register(&retrieval.TrustRanker{})
	retriever := retrieval.NewRetriever(rankerRegistry, knowledgeStore)
	// The retriever's corpus snapshot invalidates on chunk lifecycle events;
	// the subscription is owned by this composition root and torn down on Close.
	closeRetriever := retriever.SetEventBus(bkcEvents)
	comp := compiler.NewCompiler(retriever, nil, knowledgeStore)
	// Record persistence and invalidation events are owned by the
	// composition root: records go through the O(1) graph repository, and
	// chunk events on the knowledge bus drive cache invalidation.
	comp.SetRepository(compiler.NewCompilerRepository(input.GraphDB))
	comp.SetEventBus(bkcEvents)
	// Boot-time contract: every knowledge consumer must subscribe to the one
	// composition-owned bus, or invalidation silently diverges.
	if comp.EventBus() != retriever.EventBus() {
		return nil, fmt.Errorf("knowledge composition: compiler and retriever must share one event bus")
	}
	if err := comp.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("start compiler: %w", err)
	}
	return &KnowledgeRuntime{
		KnowledgeStore:  knowledgeStore,
		KnowledgeEvents: bkcEvents,
		Retriever:       retriever,
		Compiler:        comp,
		StreamTrigger:   contextstream.NewTrigger(&compilerTriggerAdapter{inner: comp}),
		closeRetriever:  closeRetriever,
	}, nil
}

// compilerTriggerAdapter adapts *compiler.Compiler to implement contextstream.CompilerInvoker.
type compilerTriggerAdapter struct {
	inner *compiler.Compiler
}

func (a *compilerTriggerAdapter) Compile(ctx context.Context, req contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	query := retrieval.RetrievalQuery{
		Text: req.BaseContext,
	}
	innerReq := compiler.CompilationRequest{
		Query:       query,
		MaxTokens:   req.BudgetTokens,
		EventLogSeq: req.EventLogSeq,
		Metadata:    req.Metadata,
	}
	result, record, err := a.inner.Compile(ctx, innerReq)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("compilation result is nil")
	}
	streamedRefs := make([]string, 0, len(result.StreamedRefs))
	for _, ref := range result.StreamedRefs {
		streamedRefs = append(streamedRefs, string(ref.ChunkID))
	}
	skipped := make([]string, 0, len(result.SkippedStaleChunks))
	for _, id := range result.SkippedStaleChunks {
		skipped = append(skipped, string(id))
	}
	subs := make([]contextports.SummarySubstitution, 0, len(result.Substitutions))
	for _, s := range result.Substitutions {
		subs = append(subs, contextports.SummarySubstitution{
			Original: string(s.OriginalChunkID),
			Replaced: string(s.SummaryChunkID),
			ChunkID:  string(s.OriginalChunkID),
		})
	}
	return &contextports.CompilationResult{
		ShortfallTokens:    result.ShortfallTokens,
		StreamedRefs:       streamedRefs,
		SkippedStaleChunks: skipped,
		Substitutions:      subs,
		Record: contextports.CompilationRecord{
			FinalTokens:    result.TotalTokens,
			OriginalBudget: req.BudgetTokens,
			CacheHit:       record != nil && record.CacheHit,
		},
	}, nil
}
