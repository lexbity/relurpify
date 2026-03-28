package benchmark

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func BenchmarkClassifyTaskScored(b *testing.B) {
	envelopes := map[string]eucloruntime.TaskEnvelope{
		"code":    runtimeEnvelope("implement the requested change and update tests"),
		"debug":   runtimeEnvelope("debug why the benchmark occasionally fails"),
		"review":  runtimeEnvelope("review this patch for regressions"),
		"plan":    runtimeEnvelope("produce a multi-step implementation plan"),
		"summary": runtimeEnvelope("summarize current status"),
	}
	for name, envelope := range envelopes {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = eucloruntime.ClassifyTaskScored(envelope)
			}
		})
	}
}

func BenchmarkResolveModeAndProfile(b *testing.B) {
	modeRegistry := euclotypes.DefaultModeRegistry()
	profileRegistry := euclotypes.DefaultExecutionProfileRegistry()
	envelope := runtimeEnvelope("implement the requested change and verify it")
	classification := eucloruntime.ClassifyTaskScored(envelope).TaskClassification
	b.Run("resolve_mode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = eucloruntime.ResolveMode(envelope, classification, modeRegistry)
		}
	})
	mode := eucloruntime.ResolveMode(envelope, classification, modeRegistry)
	b.Run("select_profile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = eucloruntime.SelectExecutionProfile(envelope, classification, mode, profileRegistry)
		}
	})
}

func BenchmarkExpandContext(b *testing.B) {
	fixture := newBenchmarkFixture(b, "runtime-expand")
	workflowID := "wf-runtime-expand"
	mustCreateWorkflow(b, fixture.workflowStore, workflowID, "expand context benchmark")
	seedWorkflowKnowledge(b, fixture.workflowStore, workflowID, 16)
	task := &core.Task{
		ID:          "task-expand",
		Instruction: "review workflow context",
		Context: map[string]any{
			"workspace": fixture.workspace,
		},
	}
	state := core.NewContext()
	mode := euclotypes.ModeResolution{ModeID: "review"}
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"}
	policy := eucloruntime.ResolveRetrievalPolicy(mode, profile)

	b.Run("local_only", func(b *testing.B) {
		noopPolicy := policy
		noopPolicy.WidenToWorkflow = false
		noopPolicy.WidenWhenNoLocal = false
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := eucloruntime.ExpandContext(context.Background(), fixture.workflowStore, workflowID, task, state, noopPolicy); err != nil {
				b.Fatalf("expand context local_only: %v", err)
			}
		}
	})
	b.Run("workflow_widened", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := eucloruntime.ExpandContext(context.Background(), fixture.workflowStore, workflowID, task, state, policy); err != nil {
				b.Fatalf("expand context workflow_widened: %v", err)
			}
		}
	})
}
