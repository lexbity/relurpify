package core

import (
	"fmt"
	"testing"
	"time"
)

func benchmarkContextFixture(stateKeys, nestedWidth, interactions int) *Context {
	ctx := NewContext()
	ctx.SetExecutionPhase("benchmark")
	for i := 0; i < stateKeys; i++ {
		nested := make(map[string]interface{}, nestedWidth)
		for j := 0; j < nestedWidth; j++ {
			nested[fmt.Sprintf("child_%d", j)] = map[string]interface{}{
				"value": fmt.Sprintf("state-%d-%d", i, j),
				"nums":  []int{i, j, i + j},
			}
		}
		ctx.Set(fmt.Sprintf("state.%04d", i), nested)
		ctx.SetVariable(fmt.Sprintf("var.%04d", i), i)
		ctx.SetKnowledge(fmt.Sprintf("knowledge.%04d", i), map[string]interface{}{
			"kind":  "fact",
			"value": fmt.Sprintf("knowledge-%d", i),
		})
	}
	for i := 0; i < interactions; i++ {
		ctx.AddInteraction("assistant", fmt.Sprintf("interaction %d %s", i, string(make([]byte, 64))), nil)
	}
	clone := ctx.Clone()
	return clone
}

func BenchmarkContextCloneSmallState(b *testing.B) {
	base := benchmarkContextFixture(24, 2, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base.Clone()
	}
}

func BenchmarkContextCloneLargeState(b *testing.B) {
	base := benchmarkContextFixture(512, 4, 32)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base.Clone()
	}
}

func BenchmarkContextDirtyDeltaSmallMutation(b *testing.B) {
	base := benchmarkContextFixture(256, 3, 16)
	branch := base.Clone()
	branch.Set("state.mutated", map[string]interface{}{"value": "changed"})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = branch.DirtyDelta()
	}
}

func BenchmarkContextDirtyDeltaLargeMutation(b *testing.B) {
	base := benchmarkContextFixture(256, 3, 16)
	branch := base.Clone()
	for i := 0; i < 128; i++ {
		branch.Set(fmt.Sprintf("state.bulk.%03d", i), map[string]interface{}{
			"updated": i,
			"nested":  map[string]interface{}{"when": time.Unix(int64(i), 0).UTC().Format(time.RFC3339)},
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = branch.DirtyDelta()
	}
}

func BenchmarkContextSnapshotRestore(b *testing.B) {
	base := benchmarkContextFixture(256, 3, 24)
	snapshot := base.Snapshot()
	target := NewContext()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := target.Restore(snapshot); err != nil {
			b.Fatal(err)
		}
	}
}
