package blackboard

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type blackboardStructuredMemoryStub struct {
	declarativeSearchCalls int
	proceduralSearchCalls  int
}

func (s *blackboardStructuredMemoryStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}

func (s *blackboardStructuredMemoryStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *blackboardStructuredMemoryStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	return nil, nil
}

func (s *blackboardStructuredMemoryStub) Forget(context.Context, string, memory.MemoryScope) error {
	return nil
}

func (s *blackboardStructuredMemoryStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func (s *blackboardStructuredMemoryStub) PutDeclarative(context.Context, memory.DeclarativeMemoryRecord) error {
	return nil
}

func (s *blackboardStructuredMemoryStub) GetDeclarative(context.Context, string) (*memory.DeclarativeMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *blackboardStructuredMemoryStub) SearchDeclarative(context.Context, memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	s.declarativeSearchCalls++
	return []memory.DeclarativeMemoryRecord{{
		RecordID: "fact-1",
		Scope:    memory.MemoryScopeProject,
		Summary:  "structured declarative result",
	}}, nil
}

func (s *blackboardStructuredMemoryStub) PutProcedural(context.Context, memory.ProceduralMemoryRecord) error {
	return nil
}

func (s *blackboardStructuredMemoryStub) GetProcedural(context.Context, string) (*memory.ProceduralMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *blackboardStructuredMemoryStub) SearchProcedural(context.Context, memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	s.proceduralSearchCalls++
	return []memory.ProceduralMemoryRecord{{
		RoutineID: "routine-1",
		Scope:     memory.MemoryScopeProject,
		Summary:   "structured procedural result",
	}}, nil
}

type blackboardRetrievalBackedMemoryStub struct {
	blackboardStructuredMemoryStub
}

func (blackboardRetrievalBackedMemoryStub) RetrievalService() retrieval.RetrieverService {
	return blackboardRetrievalServiceStub{}
}

type blackboardRetrievalServiceStub struct{}

func (blackboardRetrievalServiceStub) Retrieve(context.Context, retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return []core.ContentBlock{
		core.StructuredContentBlock{
			Data: map[string]any{
				"type": "retrieval_evidence",
				"id":   "doc:blackboard:1",
				"text": "retrieval backed declarative memory",
				"citations": []retrieval.PackedCitation{{
					DocID: "doc:blackboard:1",
				}},
			},
		},
	}, retrieval.RetrievalEvent{QueryID: "rq:blackboard:1", Timestamp: time.Now().UTC()}, nil
}

func TestBlackboardScopedMemoryRetrieverPrefersRetrievalServiceForDeclarativeQueries(t *testing.T) {
	store := &blackboardRetrievalBackedMemoryStub{}

	results, err := (blackboardScopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassDeclarative,
	}).Retrieve(context.Background(), "retrieval", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two mixed results, got %d", len(results))
	}
	if results[0].Key == "" || results[0].Key == "<nil>" {
		t.Fatalf("expected retrieval-backed key to be populated, got %#v", results[0])
	}
	if results[0].Summary != "retrieval backed declarative memory" {
		t.Fatalf("unexpected retrieval-backed summary: %#v", results[0])
	}
	if results[1].Key != "fact-1" || results[1].Summary != "structured declarative result" {
		t.Fatalf("expected structured declarative result to be merged, got %#v", results[1])
	}
	if store.declarativeSearchCalls != 1 {
		t.Fatalf("expected mixed path to query structured store once, got %d calls", store.declarativeSearchCalls)
	}
}

func TestBlackboardScopedMemoryRetrieverComposesRetrievalAndProceduralStore(t *testing.T) {
	store := &blackboardRetrievalBackedMemoryStub{}

	results, err := (blackboardScopedMemoryRetriever{
		store:       store,
		scope:       memory.MemoryScopeProject,
		memoryClass: core.MemoryClassProcedural,
	}).Retrieve(context.Background(), "checkpoint", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two mixed results, got %d", len(results))
	}
	if results[0].Key == "" || results[0].Key == "<nil>" {
		t.Fatalf("expected retrieval-backed key to be populated, got %#v", results[0])
	}
	if results[1].Key != "routine-1" || results[1].Summary != "structured procedural result" {
		t.Fatalf("expected structured procedural result to be merged, got %#v", results[1])
	}
	if store.proceduralSearchCalls != 1 {
		t.Fatalf("expected procedural store to be queried once, got %d calls", store.proceduralSearchCalls)
	}
}
