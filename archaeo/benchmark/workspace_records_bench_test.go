package benchmark

import (
	"context"
	"fmt"
	"testing"

	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoproj "codeburg.org/lexbit/relurpify/archaeo/projections"
)

func BenchmarkWorkspaceDeferredDraftProjection(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workspace-deferred-"+scale.name)
			workflowID := "wf-workspace-deferred-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "workspace deferred drafts")
			svc := archaeodeferred.Service{Store: fixture.workflowStore}
			for i := 0; i < max(1, scale.interactionCount/2); i++ {
				if _, err := svc.CreateOrUpdate(context.Background(), archaeodeferred.CreateInput{
					WorkspaceID:  fixture.workspace,
					WorkflowID:   workflowID,
					PlanID:       "plan-1",
					AmbiguityKey: fmt.Sprintf("step-%d:ambiguity", i),
					Title:        "Deferred draft",
				}); err != nil {
					b.Fatalf("seed deferred draft: %v", err)
				}
			}
			proj := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := proj.DeferredDrafts(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("deferred draft projection: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceConvergenceProjection(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workspace-convergence-"+scale.name)
			workflowID := "wf-workspace-convergence-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "workspace convergence")
			svc := archaeoconvergence.Service{Store: fixture.workflowStore}
			for i := 0; i < max(1, scale.tensionCount/2); i++ {
				record, err := svc.Create(context.Background(), archaeoconvergence.CreateInput{
					WorkspaceID: fixture.workspace,
					WorkflowID:  workflowID,
					Question:    fmt.Sprintf("question-%d", i),
				})
				if err != nil {
					b.Fatalf("seed convergence: %v", err)
				}
				if i%2 == 0 {
					if _, err := svc.Resolve(context.Background(), archaeoconvergence.ResolveInput{
						WorkflowID: workflowID,
						RecordID:   record.ID,
						Resolution: archaeodomain.ConvergenceResolution{Status: archaeodomain.ConvergenceResolutionDeferred, Summary: "defer"},
					}); err != nil {
						b.Fatalf("resolve convergence: %v", err)
					}
				}
			}
			proj := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := proj.ConvergenceHistory(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("convergence projection: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceDecisionTrailProjection(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workspace-decisions-"+scale.name)
			workflowID := "wf-workspace-decisions-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "workspace decisions")
			svc := archaeodecisions.Service{Store: fixture.workflowStore}
			for i := 0; i < max(1, scale.interactionCount/2); i++ {
				record, err := svc.Create(context.Background(), archaeodecisions.CreateInput{
					WorkspaceID: fixture.workspace,
					WorkflowID:  workflowID,
					Kind:        archaeodomain.DecisionKindStaleResult,
					Title:       fmt.Sprintf("decision-%d", i),
				})
				if err != nil {
					b.Fatalf("seed decision: %v", err)
				}
				if i%2 == 0 {
					if _, err := svc.Resolve(context.Background(), archaeodecisions.ResolveInput{
						WorkflowID: workflowID,
						RecordID:   record.ID,
						Status:     archaeodomain.DecisionStatusResolved,
					}); err != nil {
						b.Fatalf("resolve decision: %v", err)
					}
				}
			}
			proj := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := proj.DecisionTrail(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("decision projection: %v", err)
				}
			}
		})
	}
}
