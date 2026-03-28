package benchmark

import (
	"context"
	"fmt"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func BenchmarkAgentExecute(b *testing.B) {
	b.Run("simple_status", func(b *testing.B) {
		fixture := newBenchmarkFixture(b, "agent-simple")
		agent := fixture.newAgent()
		task := &core.Task{
			ID:          "task-simple",
			Instruction: "summarize current status",
			Context:     map[string]any{"workspace": fixture.workspace},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			state := benchmarkState()
			if _, err := agent.Execute(context.Background(), task, state); err != nil {
				b.Fatalf("execute: %v", err)
			}
		}
	})

	for _, scale := range benchScales {
		b.Run("with_living_plan/"+scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "agent-plan-"+scale.name)
			agent := fixture.newAgent()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-agent-plan-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "benchmark archaeology workflow")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
				state := benchmarkState()
				task := benchmarkTask(workflowID, fmt.Sprintf("summarize current status %d", i), map[string]any{
					"workspace": fixture.workspace,
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
		b.Run("with_learning_queue/"+scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "agent-learning-"+scale.name)
			agent := fixture.newAgent()
			seedPatternRecords(b, fixture.patternStore, "workspace", scale.name+"-agent", scale.patternCount)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-agent-learning-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "benchmark learning workflow")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "summarize current status with learning", map[string]any{
					"workspace":    fixture.workspace,
					"corpus_scope": "workspace",
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
		b.Run("projection_heavy/"+scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "agent-projection-"+scale.name)
			agent := fixture.newAgent()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-agent-proj-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "projection heavy workflow")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
				seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
				seedTimelineEvents(b, fixture.workflowStore, workflowID, scale.timelineEventCount)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "summarize projection-heavy workflow", map[string]any{
					"workspace": fixture.workspace,
				})
				if _, err := agent.Execute(context.Background(), task, state); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
	}
}
