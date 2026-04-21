package benchmark

import (
	"context"
	"fmt"
	"testing"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
)

func BenchmarkLearningSyncPatternProposals(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "sync-patterns-"+scale.name)
			corpusScope := "workspace-" + scale.name
			seedPatternRecords(b, fixture.patternStore, corpusScope, scale.name, scale.patternCount)
			svc := archaeolearning.Service{
				Store:        fixture.workflowStore,
				PatternStore: fixture.patternStore,
				CommentStore: fixture.commentStore,
				PlanStore:    fixture.planStore,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-sync-pattern-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "sync patterns")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				if _, err := svc.SyncPatternProposals(context.Background(), workflowID, session.ID, corpusScope, "rev-sync"); err != nil {
					b.Fatalf("sync pattern proposals: %v", err)
				}
			}
		})
	}
}

func BenchmarkLearningResolve(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "resolve-learning-"+scale.name)
			seedPatternRecords(b, fixture.patternStore, "workspace", scale.name, scale.patternCount)
			svc := archaeolearning.Service{
				Store:        fixture.workflowStore,
				PatternStore: fixture.patternStore,
				CommentStore: fixture.commentStore,
				PlanStore:    fixture.planStore,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-resolve-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "resolve learning")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				patternID := fmt.Sprintf("%s-pattern-%03d", scale.name, i%scale.patternCount)
				interaction, err := svc.Create(context.Background(), archaeolearning.CreateInput{
					WorkflowID:    workflowID,
					ExplorationID: session.ID,
					Kind:          archaeolearning.InteractionPatternProposal,
					SubjectType:   archaeolearning.SubjectPattern,
					SubjectID:     patternID,
					Title:         "Confirm pattern",
					Blocking:      true,
				})
				if err != nil {
					b.Fatalf("create interaction: %v", err)
				}
				if _, err := svc.Resolve(context.Background(), archaeolearning.ResolveInput{
					WorkflowID:    workflowID,
					InteractionID: interaction.ID,
					Kind:          archaeolearning.ResolutionConfirm,
					ChoiceID:      "confirm",
					ResolvedBy:    "benchmark",
				}); err != nil {
					b.Fatalf("resolve interaction: %v", err)
				}
			}
		})
	}
}

func BenchmarkLearningSyncAnchorDrifts(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "sync-anchor-drifts-"+scale.name)
			corpusScope := "workspace-" + scale.name
			seedAnchorDrifts(b, fixture.workflowStore.DB(), corpusScope, scale.name, scale.interactionCount)
			svc := archaeolearning.Service{
				Store:     fixture.workflowStore,
				PlanStore: fixture.planStore,
				Retrieval: archaeoretrieval.NewSQLStore(fixture.workflowStore.DB()),
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-sync-drift-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "sync anchor drifts")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				if _, err := svc.SyncAnchorDrifts(context.Background(), workflowID, session.ID, corpusScope, "rev-drift"); err != nil {
					b.Fatalf("sync anchor drifts: %v", err)
				}
			}
		})
	}
}

func BenchmarkLearningSyncTensions(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "sync-tensions-"+scale.name)
			svc := archaeolearning.Service{
				Store:     fixture.workflowStore,
				PlanStore: fixture.planStore,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-sync-tension-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "sync tensions")
				session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedTensions(b, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
				if _, err := svc.SyncTensions(context.Background(), workflowID, session.ID, snapshot.ID, "rev-tension"); err != nil {
					b.Fatalf("sync tensions: %v", err)
				}
			}
		})
	}
}

func BenchmarkComparePlanVersions(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "compare-plan-"+scale.name)
			workflowID := "wf-compare-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "compare plan versions")
			session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
			active := seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, max(2, scale.planVersionCount))
			svc := archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore}
			fromVersion := max(1, active.Version-1)
			toVersion := active.Version
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.CompareVersions(context.Background(), workflowID, fromVersion, toVersion); err != nil {
					b.Fatalf("compare versions: %v", err)
				}
			}
		})
	}
}

func BenchmarkSyncActiveVersionWithExploration(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "sync-active-version-"+scale.name)
			workflowID := "wf-sync-version-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "sync active version with exploration")
			session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
			svc := archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				candidateSnapshot := *snapshot
				candidateSnapshot.CandidatePatternRefs = []string{fmt.Sprintf("pattern-%d", i%max(1, scale.patternCount))}
				candidateSnapshot.CandidateAnchorRefs = []string{fmt.Sprintf("anchor-%d", i%max(1, scale.interactionCount))}
				candidateSnapshot.TensionIDs = []string{fmt.Sprintf("tension-%d", i%max(1, scale.tensionCount))}
				candidateSnapshot.BasedOnRevision = fmt.Sprintf("rev-%s-%d", scale.name, i)
				if _, err := svc.SyncActiveVersionWithExploration(context.Background(), workflowID, &candidateSnapshot); err != nil {
					b.Fatalf("sync active version with exploration: %v", err)
				}
			}
		})
	}
}

func BenchmarkArchaeologyPrepareLivingPlan(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "prepare-living-plan-"+scale.name)
			corpusScope := "workspace-" + scale.name
			seedPatternRecords(b, fixture.patternStore, corpusScope, scale.name, scale.patternCount)
			svc := archaeoarch.Service{
				Store: fixture.workflowStore,
				Plans: archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore},
				Learning: archaeolearning.Service{
					Store:        fixture.workflowStore,
					PatternStore: fixture.patternStore,
					CommentStore: fixture.commentStore,
					PlanStore:    fixture.planStore,
				},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-prepare-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "prepare living plan")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				seedActivePlan(b, fixture, workflowID, session.ID, scale.planStepCount)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "summarize current status", map[string]any{
					"workspace":    fixture.workspace,
					"corpus_scope": corpusScope,
				})
				if out := svc.PrepareLivingPlan(context.Background(), task, state, workflowID); out.Err != nil {
					b.Fatalf("prepare living plan: %v", out.Err)
				}
			}
		})
	}
}

func BenchmarkTensionSummaryByWorkflow(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "tension-summary-"+scale.name)
			workflowID := "wf-tension-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "tension summary")
			session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedTensions(b, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
			svc := archaeotensions.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.SummaryByWorkflow(context.Background(), workflowID); err != nil {
					b.Fatalf("tension summary: %v", err)
				}
			}
		})
	}
}
