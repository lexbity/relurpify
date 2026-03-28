package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
)

func BenchmarkWorkflowProjection(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workflow-projection-"+scale.name)
			workflowID := "wf-projection-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "workflow projection")
			session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
			seedTimelineEvents(b, fixture.workflowStore, workflowID, scale.timelineEventCount)
			planVersion := seedActivePlan(b, fixture, workflowID, session.ID, scale.planStepCount)
			finalizeConvergence(b, fixture, &planVersion.Plan)
			phaseSvc := archaeophases.Service{Store: fixture.workflowStore}
			if _, err := phaseSvc.Transition(context.Background(), workflowID, archaeodomain.PhaseExecution, archaeodomain.PhaseTransition{
				To:                archaeodomain.PhaseExecution,
				ActivePlanID:      planVersion.Plan.ID,
				ActivePlanVersion: &planVersion.Version,
			}); err != nil {
				b.Fatalf("transition phase: %v", err)
			}
			svc := archaeoprojections.Service{Store: fixture.workflowStore}
			b.Run("cold_restore", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.Workflow(context.Background(), workflowID); err != nil {
						b.Fatalf("workflow projection cold_restore: %v", err)
					}
				}
			})
			b.Run("warm_repeat", func(b *testing.B) {
				if _, err := svc.Workflow(context.Background(), workflowID); err != nil {
					b.Fatalf("warmup workflow projection: %v", err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.Workflow(context.Background(), workflowID); err != nil {
						b.Fatalf("workflow projection warm_repeat: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkDedicatedProjections(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "dedicated-projections-"+scale.name)
			workflowID := "wf-dedicated-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "dedicated projections")
			session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
			seedTimelineEvents(b, fixture.workflowStore, workflowID, scale.timelineEventCount)
			seedActivePlan(b, fixture, workflowID, session.ID, scale.planStepCount)
			svc := archaeoprojections.Service{Store: fixture.workflowStore}
			b.Run("exploration", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.Exploration(context.Background(), workflowID); err != nil {
						b.Fatalf("exploration projection: %v", err)
					}
				}
			})
			b.Run("learning_queue", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.LearningQueue(context.Background(), workflowID); err != nil {
						b.Fatalf("learning queue projection: %v", err)
					}
				}
			})
			b.Run("active_plan", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.ActivePlan(context.Background(), workflowID); err != nil {
						b.Fatalf("active plan projection: %v", err)
					}
				}
			})
			b.Run("timeline", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := svc.TimelineProjection(context.Background(), workflowID); err != nil {
						b.Fatalf("timeline projection: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkWorkflowProjectionSubscription(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workflow-subscription-"+scale.name)
			workflowID := "wf-sub-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "projection subscription")
			session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
			svc := &archaeoprojections.Service{Store: fixture.workflowStore, PollInterval: time.Millisecond}
			ch, cancel := svc.SubscribeWorkflow(workflowID, 16)
			defer cancel()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := archaeoevents.AppendWorkflowEvent(context.Background(), fixture.workflowStore, workflowID, archaeoevents.EventExplorationSnapshotUpserted, fmt.Sprintf("tick-%d", i), map[string]any{
					"workflow_id": workflowID,
					"index":       i,
				}, time.Now().UTC()); err != nil {
					b.Fatalf("append event: %v", err)
				}
				b.StartTimer()
				select {
				case <-ch:
				case <-time.After(2 * time.Second):
					b.Fatal("timed out waiting for projection event")
				}
			}
		})
	}
}

func BenchmarkWorkflowSubscriptionFanout(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workflow-fanout-"+scale.name)
			workflowID := "wf-fanout-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "projection fanout")
			svc := &archaeoprojections.Service{Store: fixture.workflowStore, PollInterval: time.Millisecond}
			subscriberCount := 1
			switch scale.name {
			case "medium":
				subscriberCount = 4
			case "large":
				subscriberCount = 8
			}
			channels := make([]<-chan archaeoprojections.ProjectionEvent, 0, subscriberCount)
			cancels := make([]func(), 0, subscriberCount)
			for i := 0; i < subscriberCount; i++ {
				ch, cancel := svc.SubscribeWorkflow(workflowID, 16)
				channels = append(channels, ch)
				cancels = append(cancels, cancel)
			}
			defer func() {
				for _, cancel := range cancels {
					cancel()
				}
			}()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := archaeoevents.AppendWorkflowEvent(context.Background(), fixture.workflowStore, workflowID, archaeoevents.EventLearningInteractionRequested, fmt.Sprintf("fanout-%d", i), map[string]any{
					"workflow_id": workflowID,
					"index":       i,
				}, time.Now().UTC()); err != nil {
					b.Fatalf("append workflow event: %v", err)
				}
				b.StartTimer()
				for _, ch := range channels {
					select {
					case <-ch:
					case <-time.After(2 * time.Second):
						b.Fatal("timed out waiting for fanout projection event")
					}
				}
			}
		})
	}
}
