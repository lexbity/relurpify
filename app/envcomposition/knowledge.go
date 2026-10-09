package envcomposition

import (
	"context"
	"fmt"
	"time"

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
	// Grounding is the synchronous capture-as-bridge write path. Graph runs
	// wire it as their epoch grounder (one bus, one synchronous write boundary).
	Grounding *knowledge.GroundingService
	// Invalidation is the revision-drift pass. It consumes code-revision events
	// from the git watcher on the composition bus and marks affected chunks
	// stale, so the next compile excludes them.
	Invalidation *knowledge.InvalidationPass
	// Drain is the invalidation subscriber's bounded, subscriber-side drain the
	// epoch barrier calls. Nil when no invalidation pass is running.
	Drain func(time.Duration)
	// Health aggregates knowledge-domain degraded signals into one condition.
	Health *knowledge.KnowledgeHealth

	Retriever     *retrieval.Retriever
	Compiler      *compiler.Compiler
	StreamTrigger *contextstream.Trigger

	// CloseRetriever tears down the retriever's event subscription. It is
	// owned by the composition root and consumed by session teardown.
	CloseRetriever func()
}

// Close stops the knowledge runtime's owned lifecycles (the invalidation loop,
// the retriever's event subscription, the compiler's invalidation loop and
// event subscription, and the health aggregator). Safe to call more than once.
func (k *KnowledgeRuntime) Close() {
	if k == nil {
		return
	}
	if k.Invalidation != nil {
		_ = k.Invalidation.Stop()
	}
	if k.CloseRetriever != nil {
		k.CloseRetriever()
	}
	if k.Compiler != nil {
		k.Compiler.Stop()
	}
	if k.Health != nil {
		k.Health.Close()
	}
}

// KnowledgeRuntimeInput carries parameters for BuildKnowledgeRuntime.
type KnowledgeRuntimeInput struct {
	GraphDB *graphdb.Engine
	Index   *ast.IndexManager
	// WorkspaceRoot is carried into revision-drift health payloads.
	WorkspaceRoot string
}

// BuildKnowledgeRuntime assembles knowledge store, grounding, retriever,
// compiler, and the revision-drift invalidation pass over one event bus.
func BuildKnowledgeRuntime(input KnowledgeRuntimeInput) (*KnowledgeRuntime, error) {
	if input.GraphDB == nil {
		return nil, fmt.Errorf("graphdb engine required")
	}
	bkcEvents := &knowledge.EventBus{}
	knowledgeStore := &knowledge.ChunkStore{Graph: input.GraphDB}
	grounding := knowledge.NewGroundingService(knowledgeStore, bkcEvents, nil, nil)
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
	// The dependency-ordered stream path is the compiler's stale-exclusion
	// mechanism: a stale seed is reported as a skipped-stale gap instead of
	// streamed into context.
	comp.SetStreamer(&knowledge.Streamer{Store: knowledgeStore})
	// Record persistence and invalidation events are owned by the
	// composition root: records go through the O(1) graph repository, and
	// chunk events on the knowledge bus drive cache invalidation.
	comp.SetRepository(compiler.NewCompilerRepository(input.GraphDB))
	comp.SetEventBus(bkcEvents)
	// Boot-time contract: every knowledge consumer must subscribe to the one
	// composition-owned bus, or invalidation silently diverges.
	if err := knowledge.AssertSameBus(comp.EventBus(), retriever.EventBus()); err != nil {
		return nil, err
	}
	if err := knowledge.AssertSameBus(comp.EventBus(), bkcEvents); err != nil {
		return nil, err
	}
	// The revision-drift pass closes the loop from git-watcher revision events
	// to stale chunks on the same bus. Its non-lethal loop keeps retrying a
	// temporarily failing store instead of dying on the first error.
	invalidation := &knowledge.InvalidationPass{
		Store:         knowledgeStore,
		Staleness:     &knowledge.StalenessManager{Store: knowledgeStore, Propagate: true, MaxDepth: 3},
		Events:        bkcEvents,
		WorkspaceRoot: input.WorkspaceRoot,
	}
	if err := comp.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("start compiler: %w", err)
	}
	if err := invalidation.Start(context.Background()); err != nil {
		comp.Stop()
		return nil, fmt.Errorf("start invalidation pass: %w", err)
	}
	health := knowledge.NewKnowledgeHealth(bkcEvents)
	rt := &KnowledgeRuntime{
		KnowledgeStore:  knowledgeStore,
		KnowledgeEvents: bkcEvents,
		Grounding:       grounding,
		Invalidation:    invalidation,
		Health:          health,
		Retriever:       retriever,
		Compiler:        comp,
		StreamTrigger:   contextstream.NewTrigger(&compilerTriggerAdapter{inner: comp}),
		CloseRetriever:  closeRetriever,
	}
	rt.Drain = func(d time.Duration) { invalidation.Drain(d) }
	return rt, nil
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
