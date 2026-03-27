package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoproj "github.com/lexcodex/relurpify/archaeo/projections"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type benchPatternSurfacer struct {
	records []patterns.PatternRecord
}

func (b benchPatternSurfacer) SurfacePatterns(context.Context, providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	return append([]patterns.PatternRecord(nil), b.records...), nil
}

type benchTensionAnalyzer struct {
	records []archaeodomain.Tension
}

func (b benchTensionAnalyzer) AnalyzeTensions(context.Context, providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	return append([]archaeodomain.Tension(nil), b.records...), nil
}

type benchProspectiveAnalyzer struct {
	records []patterns.PatternRecord
}

func (b benchProspectiveAnalyzer) AnalyzeProspective(context.Context, providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	return append([]patterns.PatternRecord(nil), b.records...), nil
}

type benchConvergenceReviewer struct{}

func (benchConvergenceReviewer) ReviewConvergence(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	return nil, nil
}

func BenchmarkRequestLifecycle(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-lifecycle-"+scale.name)
			workflowID := "wf-request-lifecycle-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "process archaeology requests")
			session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
			svc := archaeorequests.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
					WorkflowID:      workflowID,
					ExplorationID:   session.ID,
					SnapshotID:      snapshot.ID,
					Kind:            archaeodomain.RequestPatternSurfacing,
					Title:           fmt.Sprintf("Request %03d", i),
					Description:     "benchmark request lifecycle",
					RequestedBy:     "benchmark",
					SubjectRefs:     []string{fmt.Sprintf("file-%03d.go", i)},
					BasedOnRevision: "rev-bench",
				})
				if err != nil {
					b.Fatalf("create request: %v", err)
				}
				if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, map[string]any{"mode": "bench"}); err != nil {
					b.Fatalf("dispatch request: %v", err)
				}
				if _, err := svc.Start(context.Background(), workflowID, record.ID, map[string]any{"mode": "bench"}); err != nil {
					b.Fatalf("start request: %v", err)
				}
				if _, err := svc.Complete(context.Background(), archaeorequests.CompleteInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					Result: archaeodomain.RequestResult{
						Kind:    "bench_result",
						Summary: "completed",
					},
				}); err != nil {
					b.Fatalf("complete request: %v", err)
				}
			}
		})
	}
}

func BenchmarkArchaeologyProviderLifecycle(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "archaeology-provider-lifecycle-"+scale.name)
			now := time.Now().UTC()
			providedPatterns := seedPatternRecords(b, fixture.patternStore, fixture.workspace, "provider", max(2, scale.patternCount/2))
			prospectivePatterns := seedPatternRecords(b, fixture.patternStore, fixture.workspace, "prospective", max(2, scale.patternCount/3))
			providedTensions := make([]archaeodomain.Tension, 0, max(1, scale.tensionCount/2))
			for i := 0; i < max(1, scale.tensionCount/2); i++ {
				providedTensions = append(providedTensions, archaeodomain.Tension{
					ID:                 fmt.Sprintf("provider-tension-%03d", i),
					SourceRef:          fmt.Sprintf("provider-gap-%03d", i),
					Kind:               "intent_gap",
					Description:        "provider tension",
					Status:             archaeodomain.TensionUnresolved,
					RelatedPlanStepIDs: []string{"resolve_tensions"},
					BasedOnRevision:    "rev-provider",
				})
			}
			svc := archaeoarch.Service{
				Store: fixture.workflowStore,
				Plans: archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore},
				Learning: archaeolearning.Service{
					Store:        fixture.workflowStore,
					PatternStore: fixture.patternStore,
					CommentStore: fixture.commentStore,
					PlanStore:    fixture.planStore,
					Retrieval:    archaeoretrieval.NewSQLStore(fixture.retrievalDB),
				},
				Requests: archaeorequests.Service{Store: fixture.workflowStore},
				Providers: providers.Bundle{
					PatternSurfacer:     benchPatternSurfacer{records: providedPatterns},
					TensionAnalyzer:     benchTensionAnalyzer{records: providedTensions},
					ProspectiveAnalyzer: benchProspectiveAnalyzer{records: prospectivePatterns},
					ConvergenceReviewer: benchConvergenceReviewer{},
				},
				PersistPhase: func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep) {
				},
				EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
					return archaeoexec.PreflightOutcome{}, nil
				},
				Now: func() time.Time { return now },
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-provider-lifecycle-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "refresh archaeology with providers")
				state := benchmarkState()
				task := benchmarkTask(workflowID, "review structure", map[string]any{
					"workspace":    fixture.workspace,
					"corpus_scope": fixture.workspace,
					"symbol_scope": fixture.sourcePath,
				})
				seedActivePlan(b, fixture, workflowID, "", scale.planStepCount)
				out := svc.PrepareLivingPlan(context.Background(), task, state, workflowID)
				if out.Err != nil {
					b.Fatalf("prepare living plan with providers: %v", out.Err)
				}
			}
		})
	}
}

func BenchmarkHistoryAndCoherenceProjections(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "history-coherence-"+scale.name)
			workflowID := "wf-history-coherence-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "build history and coherence views")
			session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedPatternRecords(b, fixture.patternStore, fixture.workspace, scale.name, scale.patternCount)
			seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
			seedTensions(b, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
			seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
			seedTimelineEvents(b, fixture.workflowStore, workflowID, scale.timelineEventCount)
			appendMutationBatch(b, fixture, workflowID, "plan-1", max(1, scale.planVersionCount), "step-1", scale.interactionCount)
			seedRequestHistory(b, fixture, workflowID, session.ID, snapshot.ID, scale.interactionCount)
			svc := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.MutationHistory(context.Background(), workflowID); err != nil {
					b.Fatalf("mutation history: %v", err)
				}
				if _, err := svc.RequestHistory(context.Background(), workflowID); err != nil {
					b.Fatalf("request history: %v", err)
				}
				if _, err := svc.PlanLineage(context.Background(), workflowID); err != nil {
					b.Fatalf("plan lineage: %v", err)
				}
				if _, err := svc.ExplorationActivity(context.Background(), workflowID); err != nil {
					b.Fatalf("exploration activity: %v", err)
				}
				if _, err := svc.Provenance(context.Background(), workflowID); err != nil {
					b.Fatalf("provenance: %v", err)
				}
				if _, err := svc.Coherence(context.Background(), workflowID); err != nil {
					b.Fatalf("coherence: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkflowProjectionWithMutationDenseHistory(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "workflow-mutation-dense-"+scale.name)
			workflowID := "wf-mutation-dense-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "mutation dense projection rebuild")
			session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
			seedLearningInteractions(b, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
			seedTensions(b, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
			seedPlanVersionHistory(b, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
			seedTimelineEvents(b, fixture.workflowStore, workflowID, scale.timelineEventCount)
			appendMutationBatch(b, fixture, workflowID, "plan-1", max(1, scale.planVersionCount), "step-1", scale.timelineEventCount)
			seedRequestHistory(b, fixture, workflowID, session.ID, snapshot.ID, scale.interactionCount)
			svc := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.Workflow(context.Background(), workflowID); err != nil {
					b.Fatalf("workflow projection: %v", err)
				}
			}
		})
	}
}

func seedRequestHistory(tb testing.TB, fixture *benchmarkFixture, workflowID, explorationID, snapshotID string, count int) {
	tb.Helper()
	svc := archaeorequests.Service{Store: fixture.workflowStore}
	for i := 0; i < count; i++ {
		record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			SnapshotID:      snapshotID,
			Kind:            archaeodomain.RequestPatternSurfacing,
			Title:           fmt.Sprintf("Request %03d", i),
			Description:     "seeded request",
			RequestedBy:     "benchmark",
			SubjectRefs:     []string{fmt.Sprintf("subject-%03d", i)},
			BasedOnRevision: "rev-bench",
		})
		if err != nil {
			tb.Fatalf("seed request: %v", err)
		}
		if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, map[string]any{"mode": "seed"}); err != nil {
			tb.Fatalf("dispatch request: %v", err)
		}
		if i%3 == 0 {
			if _, err := svc.Start(context.Background(), workflowID, record.ID, map[string]any{"mode": "seed"}); err != nil {
				tb.Fatalf("start request: %v", err)
			}
		}
		if i%2 == 0 {
			if _, err := svc.Complete(context.Background(), archaeorequests.CompleteInput{
				WorkflowID: workflowID,
				RequestID:  record.ID,
				Result:     archaeodomain.RequestResult{Kind: "seed", Summary: "done"},
			}); err != nil {
				tb.Fatalf("complete request: %v", err)
			}
		}
	}
}
