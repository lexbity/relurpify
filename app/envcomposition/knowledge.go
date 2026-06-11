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
type KnowledgeRuntime struct {
	KnowledgeStore  *knowledge.ChunkStore
	KnowledgeEvents *knowledge.EventBus
	Retriever       *retrieval.Retriever
	Compiler        *compiler.Compiler
	StreamTrigger   *contextstream.Trigger
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
	comp := compiler.NewCompiler(retriever, nil, knowledgeStore)
	return &KnowledgeRuntime{
		KnowledgeStore:  knowledgeStore,
		KnowledgeEvents: bkcEvents,
		Retriever:       retriever,
		Compiler:        comp,
		StreamTrigger:   contextstream.NewTrigger(&compilerTriggerAdapter{inner: comp}),
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
		Query:     query,
		MaxTokens: req.BudgetTokens,
	}
	result, _, err := a.inner.Compile(ctx, innerReq)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
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
		},
	}, nil
}


