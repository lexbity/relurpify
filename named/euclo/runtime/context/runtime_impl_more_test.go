package context

import (
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

func TestRuntimeHelperSelectionAndPreferences(t *testing.T) {
	strategy, name := selectContextStrategy(eucloruntime.ModeResolution{ModeID: "review"}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{Family: eucloruntime.ExecutorFamilyReact}}})
	if strategy == nil || name != "conservative" {
		t.Fatalf("unexpected strategy resolution: %T %q", strategy, name)
	}
	strategy, name = selectContextStrategy(eucloruntime.ModeResolution{ModeID: "debug"}, eucloruntime.UnitOfWork{})
	if strategy == nil || name != "aggressive" {
		t.Fatalf("unexpected debug strategy resolution: %T %q", strategy, name)
	}
	strategy, name = selectContextStrategy(eucloruntime.ModeResolution{ModeID: "code"}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ContextStrategyID: "expand_carefully"}})
	if strategy == nil || name != "expand_carefully" {
		t.Fatalf("unexpected explicit strategy resolution: %T %q", strategy, name)
	}
	if got := strategy.DetermineDetailLevel("file.go", 0.6); got != contextmgr.DetailDetailed {
		t.Fatalf("expected explicit profile to use balanced thresholds, got %v", got)
	}
	if _, name = selectContextStrategy(eucloruntime.ModeResolution{ModeID: "code"}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ContextStrategyID: "unknown-profile"}}); name != "adaptive" {
		t.Fatalf("unexpected fallback strategy name for unknown profile: %q", name)
	}

	prefs := buildContextPolicyPreferences(
		eucloruntime.ModeResolution{ModeID: "planning"},
		eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ResolvedPolicy: eucloruntime.ResolvedExecutionPolicy{ContextPolicy: eucloruntime.ContextPolicySummary{PreferredDetail: "signature_only"}}, ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{Family: eucloruntime.ExecutorFamilyPlanner}}},
	)
	if prefs.PreferredDetailLevel != contextmgr.DetailSignatureOnly || prefs.MinHistorySize != 7 || prefs.CompressionThreshold != 0.75 {
		t.Fatalf("unexpected preferences: %#v", prefs)
	}

	system, tools, output := contextReservationsForWork(eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{Family: eucloruntime.ExecutorFamilyRewoo}}})
	if system != 900 || tools != 1800 || output != 1200 {
		t.Fatalf("unexpected rewoo reservations: %d %d %d", system, tools, output)
	}
	system, tools, output = contextReservationsForWork(eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ModeID: "debug"}})
	if system != 700 || tools != 1600 || output != 1000 {
		t.Fatalf("unexpected debug reservations: %d %d %d", system, tools, output)
	}

	if got := parsePreferredDetail("minimal"); got != contextmgr.DetailMinimal {
		t.Fatalf("unexpected detail parsing: %v", got)
	}
	if got := parsePreferredDetail("signature"); got != contextmgr.DetailSignatureOnly {
		t.Fatalf("unexpected signature detail parsing: %v", got)
	}
	if got := budgetStateLabel(core.BudgetCritical); got != "critical" {
		t.Fatalf("unexpected budget label: %q", got)
	}

	paths := contextProtectedPaths(&core.Task{Context: map[string]any{"workspace": " /tmp/workspace "}}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ContextBundle: eucloruntime.UnitOfWorkContextBundle{WorkspacePaths: []string{" /tmp/workspace ", "/repo"}},
		PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{ActiveStepID: "step-1"}},
	})
	if !reflect.DeepEqual(paths, []string{"/tmp/workspace", "/repo", "step-1"}) {
		t.Fatalf("unexpected sandbox file-scope governance roots: %#v", paths)
	}

	set := stringSliceSet([]string{" a ", "", "b", "a"})
	if len(set) != 2 {
		t.Fatalf("unexpected string slice set: %#v", set)
	}
	if !containsDebugMessage([]string{"compaction complete", "Demoted file context"}, "demoted file context") {
		t.Fatal("expected debug message search to match")
	}
	if got := uniqueStrings([]string{" a ", "a", "", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected unique strings: %#v", got)
	}
}

func TestRuntimeBKCChunkHelpers(t *testing.T) {
	rawChunks := contextChunksFromAny([]any{
		map[string]any{"id": "chunk-1", "content": "alpha", "token_estimate": 12, "metadata": map[string]any{"version": "v1"}},
		map[string]any{"content": "alpha"},
		"skip",
	})
	if len(rawChunks) != 2 {
		t.Fatalf("unexpected chunks parsed from any: %#v", rawChunks)
	}
	if rawChunks[0].Metadata["version"] != "v1" || rawChunks[0].TokenEstimate != 12 {
		t.Fatalf("unexpected chunk metadata: %#v", rawChunks[0])
	}

	chunk, ok := contextChunkFromValue(map[string]any{"id": "chunk-2", "content": "beta", "metadata": map[string]string{"kind": "note"}})
	if !ok || chunk.ID != "chunk-2" || chunk.Metadata["kind"] != "note" {
		t.Fatalf("unexpected chunk value: %#v %v", chunk, ok)
	}

	unique := uniqueContextChunks([]contextmgr.ContextChunk{{ID: "same"}, {ID: "same"}, {Content: "fallback"}, {Content: "fallback"}})
	if len(unique) != 2 {
		t.Fatalf("unexpected unique chunk list: %#v", unique)
	}

	task := &core.Task{Context: map[string]any{"root_chunk_ids": []any{" root-1 ", "root-2", ""}, "tension_refs": "ignored"}}
	chunks := bkcSeedChunks(task, core.NewContext(), eucloruntime.ModeResolution{ModeID: "debug"}, eucloruntime.UnitOfWork{})
	if !reflect.DeepEqual(chunks, []contextmgr.ContextChunk{{ID: "root-1"}, {ID: "root-2"}, {ID: "ignored"}}) {
		t.Fatalf("unexpected debug seed chunks: %#v", chunks)
	}

	chunks = bkcSeedChunks(task, core.NewContext(), eucloruntime.ModeResolution{ModeID: "planning"}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{RootChunkIDs: []string{"plan-root-1", "plan-root-1", "plan-root-2"}}}})
	if !reflect.DeepEqual(chunks, []contextmgr.ContextChunk{{ID: "plan-root-1"}, {ID: "plan-root-2"}}) {
		t.Fatalf("unexpected plan seed chunks: %#v", chunks)
	}

	base := contextmgr.NewAdaptiveStrategy()
	wrapped, ok := wrapBKCStrategy(task, core.NewContext(), ContextRuntimeConfig{BKCBootstrapReady: true}, eucloruntime.ModeResolution{ModeID: "debug"}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{RootChunkIDs: []string{"seed-1"}}}}, base)
	if !ok || wrapped == nil {
		t.Fatal("expected BKC strategy wrapper to activate")
	}
	loaded, err := wrapped.(*bkcContextStrategy).LoadChunks(task, nil)
	if err != nil {
		t.Fatalf("load chunks: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "seed-1" {
		t.Fatalf("unexpected wrapped chunks: %#v", loaded)
	}

	if _, ok := wrapBKCStrategy(task, core.NewContext(), ContextRuntimeConfig{}, eucloruntime.ModeResolution{}, eucloruntime.UnitOfWork{}, base); ok {
		t.Fatal("expected wrapper to stay disabled without bootstrap readiness")
	}
}
