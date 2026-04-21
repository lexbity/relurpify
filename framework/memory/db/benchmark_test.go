package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

func seedRuntimeMemoryBenchmarkStore(b *testing.B, store *SQLiteRuntimeMemoryStore, count int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		if err := store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
			RecordID: fmt.Sprintf("fact-%03d", i),
			Scope:    memory.MemoryScopeProject,
			Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
			Title:    fmt.Sprintf("Declarative fact %03d", i),
			Summary:  fmt.Sprintf("Summary for declarative fact %03d", i),
			Content:  fmt.Sprintf("alpha retrieval memory content %03d", i),
			Tags:     []string{"bench", "retrieval"},
			Verified: i%2 == 0,
		}); err != nil {
			b.Fatal(err)
		}
		if err := store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
			RoutineID:   fmt.Sprintf("routine-%03d", i),
			Scope:       memory.MemoryScopeProject,
			Kind:        memory.ProceduralMemoryKindCapabilityComposition,
			Name:        fmt.Sprintf("Procedure %03d", i),
			Description: fmt.Sprintf("checkpoint recovery routine %03d", i),
			Summary:     fmt.Sprintf("checkpoint routine %03d", i),
			InlineBody:  fmt.Sprintf("checkpoint summarize verify %03d", i),
			CapabilityDependencies: []core.CapabilitySelector{{
				Kind: core.CapabilityKindTool,
				Name: "checkpoint",
			}},
			Verified:   i%2 == 0,
			ReuseCount: i % 10,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeMemorySearchDeclarative(b *testing.B) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(b.TempDir(), "runtime_memory.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	seedRuntimeMemoryBenchmarkStore(b, store, 128)

	ctx := context.Background()
	query := memory.DeclarativeMemoryQuery{
		Query: "retrieval memory",
		Scope: memory.MemoryScopeProject,
		Limit: 10,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.SearchDeclarative(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeMemorySearchProcedural(b *testing.B) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(b.TempDir(), "runtime_memory.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	seedRuntimeMemoryBenchmarkStore(b, store, 128)

	ctx := context.Background()
	query := memory.ProceduralMemoryQuery{
		Query:          "checkpoint",
		Scope:          memory.MemoryScopeProject,
		CapabilityName: "checkpoint",
		Limit:          10,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.SearchProcedural(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeMemoryPutDeclarative(b *testing.B) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(b.TempDir(), "runtime_memory.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
			RecordID: fmt.Sprintf("fact-write-%06d", i),
			Scope:    memory.MemoryScopeProject,
			Kind:     memory.DeclarativeMemoryKindProjectKnowledge,
			Title:    fmt.Sprintf("Fact write %d", i),
			Summary:  "benchmark declarative write",
			Content:  fmt.Sprintf("alpha write payload %d", i),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeMemoryPutProcedural(b *testing.B) {
	store, err := NewSQLiteRuntimeMemoryStore(filepath.Join(b.TempDir(), "runtime_memory.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.PutProcedural(ctx, memory.ProceduralMemoryRecord{
			RoutineID:   fmt.Sprintf("routine-write-%06d", i),
			Scope:       memory.MemoryScopeProject,
			Kind:        memory.ProceduralMemoryKindCapabilityComposition,
			Name:        fmt.Sprintf("Procedure write %d", i),
			Description: "benchmark procedural write",
			Summary:     "checkpoint routine",
			InlineBody:  fmt.Sprintf("checkpoint summarize %d", i),
			CapabilityDependencies: []core.CapabilitySelector{{
				Kind: core.CapabilityKindTool,
				Name: "checkpoint",
			}},
		}); err != nil {
			b.Fatal(err)
		}
	}
}
