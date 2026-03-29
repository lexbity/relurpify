package benchmark

import (
	"context"
	"fmt"
	"testing"

	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func BenchmarkEucloWorkloadScenarios(b *testing.B) {
	workloads := []struct {
		name string
		run  func(*testing.B, *benchmarkFixture, int)
	}{
		{
			name: "failing_test_to_fix",
			run: func(b *testing.B, fixture *benchmarkFixture, i int) {
				agent := fixture.newAgent()
				state := benchmarkState()
				task := benchmarkTask(fmt.Sprintf("wf-failing-test-%d", i), "fix the failing test in pkg/foo and verify the repair", map[string]any{
					"mode": "debug",
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute failing_test_to_fix: %v", err)
				}
			},
		},
		{
			name: "multi_step_living_plan",
			run: func(b *testing.B, fixture *benchmarkFixture, i int) {
				agent := fixture.newAgent()
				workflowID := fmt.Sprintf("wf-multi-step-%d", i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "multi-step implementation benchmark")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedPlanVersionHistory(b, fixture, workflowID, session.ID, 12, 3)
				seedLearningInteractions(b, fixture, workflowID, session.ID, "multi-step", 10)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "plan and implement the requested feature across multiple steps", map[string]any{
					"workspace": fixture.workspace,
					"mode":      "planning",
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute multi_step_living_plan: %v", err)
				}
			},
		},
		{
			name: "compatibility_preserving_refactor",
			run: func(b *testing.B, fixture *benchmarkFixture, i int) {
				agent := fixture.newAgent()
				state := benchmarkState()
				task := benchmarkTask(fmt.Sprintf("wf-refactor-%d", i), "refactor the helper into a worker while preserving the public API", map[string]any{
					"workspace": fixture.workspace,
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute compatibility_preserving_refactor: %v", err)
				}
			},
		},
		{
			name: "long_running_migration",
			run: func(b *testing.B, fixture *benchmarkFixture, i int) {
				agent := fixture.newAgent()
				workflowID := fmt.Sprintf("wf-migration-%d", i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "long-running migration benchmark")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedPlanVersionHistory(b, fixture, workflowID, session.ID, 20, 4)
				seedTimelineEvents(b, fixture.workflowStore, workflowID, 30)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "execute the dependency migration to the new SDK", map[string]any{
					"workspace": fixture.workspace,
					"mode":      "planning",
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute long_running_migration: %v", err)
				}
			},
		},
		{
			name: "restore_after_compaction",
			run: func(b *testing.B, fixture *benchmarkFixture, i int) {
				agent := fixture.newAgent()
				workflowID := fmt.Sprintf("wf-restore-%d", i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "restore benchmark")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				active := seedPlanVersionHistory(b, fixture, workflowID, session.ID, 8, 2)
				state := benchmarkState()
				state.Set("euclo.active_plan_version", *active)
				task := benchmarkTask(workflowID, "plan and implement the migration", map[string]any{
					"workspace": fixture.workspace,
					"mode":      "planning",
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("prime restore_after_compaction: %v", err)
				}
				resume := benchmarkState()
				resume.Set("euclo.context_compaction", eucloruntime.ContextLifecycleState{
					WorkflowID:         workflowID,
					RunID:              state.GetString("euclo.run_id"),
					Stage:              eucloruntime.ContextLifecycleStageCompacted,
					RestoreRequired:    true,
					CompactionEligible: true,
				})
				task = benchmarkTask(workflowID, "continue the migration after compaction", map[string]any{
					"workspace":                fixture.workspace,
					"mode":                     "planning",
					"run_id":                   state.GetString("euclo.run_id"),
					"euclo.restore_continuity": true,
				})
				if _, err := agent.Execute(context.Background(), task, resume); err != nil {
					b.Fatalf("resume restore_after_compaction: %v", err)
				}
			},
		},
	}

	for _, workload := range workloads {
		b.Run(workload.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, workload.name)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workload.run(b, fixture, i)
			}
		})
	}
}
