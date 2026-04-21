package react

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

type structuredMemoryStub struct {
	genericSearchCalls     int
	declarativeSearchCalls int
	proceduralSearchCalls  int
}

func (s *structuredMemoryStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}

func (s *structuredMemoryStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *structuredMemoryStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	s.genericSearchCalls++
	return []memory.MemoryRecord{{
		Key:   "generic",
		Scope: memory.MemoryScopeProject,
		Value: map[string]interface{}{"summary": "generic fallback"},
	}}, nil
}

func (s *structuredMemoryStub) Forget(context.Context, string, memory.MemoryScope) error {
	return nil
}

func (s *structuredMemoryStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func (s *structuredMemoryStub) PutDeclarative(context.Context, memory.DeclarativeMemoryRecord) error {
	return nil
}

func (s *structuredMemoryStub) GetDeclarative(context.Context, string) (*memory.DeclarativeMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *structuredMemoryStub) SearchDeclarative(context.Context, memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	s.declarativeSearchCalls++
	return []memory.DeclarativeMemoryRecord{{
		RecordID: "fact-1",
		Scope:    memory.MemoryScopeProject,
		Summary:  "structured declarative result",
	}}, nil
}

func (s *structuredMemoryStub) PutProcedural(context.Context, memory.ProceduralMemoryRecord) error {
	return nil
}

func (s *structuredMemoryStub) GetProcedural(context.Context, string) (*memory.ProceduralMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *structuredMemoryStub) SearchProcedural(context.Context, memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	s.proceduralSearchCalls++
	return []memory.ProceduralMemoryRecord{{
		RoutineID: "routine-1",
		Scope:     memory.MemoryScopeProject,
		Summary:   "structured procedural result",
	}}, nil
}

func TestScopedMemoryRetrieverPrefersDeclarativeStore(t *testing.T) {
	store := &structuredMemoryStub{}

	results, err := (scopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassDeclarative,
	}).Retrieve(context.Background(), "sqlite", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "fact-1", results[0].Key)
	require.Equal(t, core.MemoryClassDeclarative, results[0].MemoryClass)
	require.Equal(t, 1, store.declarativeSearchCalls)
	require.Equal(t, 0, store.genericSearchCalls)
}

func TestScopedMemoryRetrieverPrefersProceduralStore(t *testing.T) {
	store := &structuredMemoryStub{}

	results, err := (scopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassProcedural,
	}).Retrieve(context.Background(), "checkpoint", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "routine-1", results[0].Key)
	require.Equal(t, core.MemoryClassProcedural, results[0].MemoryClass)
	require.Equal(t, 1, store.proceduralSearchCalls)
	require.Equal(t, 0, store.genericSearchCalls)
}

type genericMemoryOnlyStub struct {
	searchCalls int
}

func (s *genericMemoryOnlyStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}

func (s *genericMemoryOnlyStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *genericMemoryOnlyStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	s.searchCalls++
	return []memory.MemoryRecord{{
		Key:   "generic-1",
		Scope: memory.MemoryScopeProject,
		Value: map[string]interface{}{"summary": "generic fallback", "memory_class": "declarative"},
	}}, nil
}

func (s *genericMemoryOnlyStub) Forget(context.Context, string, memory.MemoryScope) error { return nil }
func (s *genericMemoryOnlyStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func TestScopedMemoryRetrieverFallsBackToGenericMemoryStore(t *testing.T) {
	store := &genericMemoryOnlyStub{}

	results, err := (scopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassDeclarative,
	}).Retrieve(context.Background(), "fallback", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "generic-1", results[0].Key)
	require.Equal(t, core.MemoryClassDeclarative, results[0].MemoryClass)
	require.Equal(t, 1, store.searchCalls)
}

type retrievalBackedMemoryStub struct {
	structuredMemoryStub
}

func (retrievalBackedMemoryStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}

func (retrievalBackedMemoryStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (retrievalBackedMemoryStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	return nil, nil
}

func (retrievalBackedMemoryStub) Forget(context.Context, string, memory.MemoryScope) error {
	return nil
}
func (retrievalBackedMemoryStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func (retrievalBackedMemoryStub) RetrievalService() retrieval.RetrieverService {
	return retrievalServiceStub{}
}

type retrievalServiceStub struct{}

func (retrievalServiceStub) Retrieve(context.Context, retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return []core.ContentBlock{
		core.StructuredContentBlock{
			Data: map[string]any{
				"type": "retrieval_evidence",
				"text": "retrieval backed declarative memory",
				"citations": []retrieval.PackedCitation{{
					DocID: "doc:1",
				}},
			},
		},
	}, retrieval.RetrievalEvent{QueryID: "rq:1", Timestamp: time.Now().UTC()}, nil
}

func TestScopedMemoryRetrieverPrefersRetrievalServiceForDeclarativeQueries(t *testing.T) {
	store := &retrievalBackedMemoryStub{}

	results, err := (scopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassDeclarative,
	}).Retrieve(context.Background(), "retrieval", 3)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "doc:1", results[0].Key)
	require.Equal(t, core.MemoryClassDeclarative, results[0].MemoryClass)
	require.Contains(t, results[0].Summary, "retrieval backed declarative memory")
	require.Equal(t, "fact-1", results[1].Key)
	require.Equal(t, 1, store.declarativeSearchCalls)
}

func TestScopedMemoryRetrieverComposesRetrievalAndProceduralStore(t *testing.T) {
	store := &retrievalBackedMemoryStub{}

	results, err := (scopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassProcedural,
	}).Retrieve(context.Background(), "checkpoint", 3)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "doc:1", results[0].Key)
	require.Equal(t, core.MemoryClassProcedural, results[0].MemoryClass)
	require.Equal(t, "routine-1", results[1].Key)
	require.Equal(t, 1, store.proceduralSearchCalls)
}
