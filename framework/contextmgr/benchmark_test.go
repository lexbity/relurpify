package contextmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type benchmarkContextItem struct {
	tokens    int
	relevance float64
	priority  int
	age       time.Duration
}

func (f *benchmarkContextItem) TokenCount() int         { return f.tokens }
func (f *benchmarkContextItem) RelevanceScore() float64 { return f.relevance }
func (f *benchmarkContextItem) Priority() int           { return f.priority }
func (f *benchmarkContextItem) Type() core.ContextItemType {
	return core.ContextTypeMemory
}
func (f *benchmarkContextItem) Age() time.Duration { return f.age }
func (f *benchmarkContextItem) Compress() (core.ContextItem, error) {
	compressed := f.tokens / 2
	if compressed == 0 {
		compressed = 1
	}
	return &benchmarkContextItem{
		tokens:    compressed,
		relevance: f.relevance * 0.9,
		priority:  f.priority + 1,
		age:       f.age,
	}, nil
}

func BenchmarkContextManagerAddItem(b *testing.B) {
	budget := core.NewContextBudget(64000)
	budget.SetReservations(0, 0, 0)
	manager := NewContextManager(budget)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%256 == 0 {
			manager.Clear()
		}
		if err := manager.AddItem(&benchmarkContextItem{
			tokens:    64,
			relevance: 0.8,
			priority:  1,
			age:       time.Minute,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextManagerUpsertFileItem(b *testing.B) {
	budget := core.NewContextBudget(64000)
	budget.SetReservations(0, 0, 0)
	manager := NewContextManager(budget)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := manager.UpsertFileItem(&core.FileContextItem{
			Path:         fmt.Sprintf("file-%03d.go", i%64),
			Content:      strings.Repeat("func bench() {}\n", 12),
			Summary:      "benchmark file",
			LastAccessed: time.Now().UTC(),
			Relevance:    0.9,
			PriorityVal:  1,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextManagerMakeSpaceCompression(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		budget := core.NewContextBudget(4000)
		budget.SetReservations(0, 0, 0)
		manager := NewContextManager(budget)
		for j := 0; j < 16; j++ {
			if err := manager.AddItem(&benchmarkContextItem{
				tokens:    200,
				relevance: 0.1,
				priority:  j%4 + 1,
				age:       time.Duration(j+1) * time.Hour,
			}); err != nil {
				b.Fatal(err)
			}
		}
		usage := budget.GetCurrentUsage()
		usage.ContextUsagePercent = 0.90
		budget.SetCurrentUsage(usage)
		b.StartTimer()
		if err := manager.MakeSpace(300); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextManagerMakeSpacePrune(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		budget := core.NewContextBudget(4000)
		budget.SetReservations(0, 0, 0)
		manager := NewContextManager(budget)
		for j := 0; j < 16; j++ {
			if err := manager.AddItem(&benchmarkContextItem{
				tokens:    200,
				relevance: 0.0,
				priority:  j%4 + 5,
				age:       time.Duration(j+24) * time.Hour,
			}); err != nil {
				b.Fatal(err)
			}
		}
		usage := budget.GetCurrentUsage()
		usage.ContextUsagePercent = 0.98
		budget.SetCurrentUsage(usage)
		b.StartTimer()
		if err := manager.MakeSpace(500); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProgressiveLoaderDemoteToFree(b *testing.B) {
	dir := b.TempDir()
	paths := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		path := filepath.Join(dir, fmt.Sprintf("bench-%d.go", i))
		if err := os.WriteFile(path, []byte(strings.Repeat("func benchmark() {}\n", 256)), 0o644); err != nil {
			b.Fatal(err)
		}
		paths = append(paths, path)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		budget := core.NewContextBudget(64000)
		budget.SetReservations(0, 0, 0)
		manager := NewContextManager(budget)
		loader := NewProgressiveLoader(manager, nil, nil, nil, budget, &core.SimpleSummarizer{})
		now := time.Now().UTC()
		for idx, path := range paths {
			item := &core.FileContextItem{
				Path:         path,
				Content:      strings.Repeat("func benchmark() {}\n", 256),
				LastAccessed: now.Add(-time.Duration(idx+1) * time.Hour),
				Relevance:    0.3,
				PriorityVal:  idx + 1,
			}
			if err := manager.UpsertFileItem(item); err != nil {
				b.Fatal(err)
			}
			loader.loadedFiles[path] = DetailFull
		}
		b.StartTimer()
		if _, err := loader.DemoteToFree(512, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextPolicyRecordGraphMemoryPublications(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		policy := NewContextPolicy(ContextPolicyConfig{}, nil)
		state := core.NewContext()
		results := make([]map[string]any, 0, 24)
		for j := 0; j < 24; j++ {
			results = append(results, map[string]any{
				"summary":   fmt.Sprintf("retrieved memory %03d", j),
				"text":      fmt.Sprintf("retrieved memory text %03d", j),
				"source":    "retrieval",
				"record_id": fmt.Sprintf("doc:%03d", j),
				"kind":      "document",
				"reference": map[string]any{
					"kind": string(core.ContextReferenceRetrievalEvidence),
					"uri":  fmt.Sprintf("memory://runtime/doc:%03d", j),
				},
			})
		}
		state.Set("graph.declarative_memory_payload", map[string]any{"results": results})
		b.StartTimer()
		policy.RecordGraphMemoryPublications(state, nil)
	}
}

func BenchmarkContextPolicyRecordGraphMemoryPublicationsDeduped(b *testing.B) {
	policy := NewContextPolicy(ContextPolicyConfig{}, nil)
	state := core.NewContext()
	results := make([]map[string]any, 0, 24)
	for j := 0; j < 24; j++ {
		results = append(results, map[string]any{
			"summary":   fmt.Sprintf("retrieved memory %03d", j),
			"record_id": fmt.Sprintf("doc:%03d", j),
			"source":    "retrieval",
			"reference": map[string]any{
				"kind": string(core.ContextReferenceRetrievalEvidence),
				"uri":  fmt.Sprintf("memory://runtime/doc:%03d", j),
			},
		})
	}
	state.Set("graph.declarative_memory_payload", map[string]any{"results": results})
	policy.RecordGraphMemoryPublications(state, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.RecordGraphMemoryPublications(state, nil)
	}
}
