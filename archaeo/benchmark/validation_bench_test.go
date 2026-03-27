package benchmark

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeointernal "github.com/lexcodex/relurpify/archaeo/internal/storeutil"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoproj "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoprovenance "github.com/lexcodex/relurpify/archaeo/provenance"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type workspaceArtifactLister interface {
	ListWorkflowArtifactsByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error)
}

func BenchmarkListWorkflowArtifactsByKind(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "artifact-kind-"+scale.name)
			workflowID := "wf-artifact-kind-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "list artifacts by kind")
			seedMixedArtifacts(b, fixture, workflowID, fixture.workspace, scale)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := archaeointernal.ListWorkflowArtifactsByKind(context.Background(), fixture.workflowStore, workflowID, "", "bench_target"); err != nil {
					b.Fatalf("list artifacts by kind: %v", err)
				}
			}
		})
	}
}

func BenchmarkListWorkflowArtifactsByKindAndWorkspace(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "artifact-kind-workspace-"+scale.name)
			workflowID := "wf-artifact-kind-workspace-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "list artifacts by kind and workspace")
			seedMixedArtifacts(b, fixture, workflowID, fixture.workspace, scale)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := listArtifactsByKindAndWorkspace(context.Background(), fixture.workflowStore, workflowID, "", "bench_target", fixture.workspace); err != nil {
					b.Fatalf("list artifacts by kind and workspace: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceScopedArtifactFallbackFiltering(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "artifact-fallback-"+scale.name)
			workflowID := "wf-artifact-fallback-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "fallback workspace artifact filtering")
			seedMixedArtifacts(b, fixture, workflowID, fixture.workspace, scale)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				artifacts, err := fixture.workflowStore.ListWorkflowArtifacts(context.Background(), workflowID, "")
				if err != nil {
					b.Fatalf("list workflow artifacts: %v", err)
				}
				count := 0
				for _, artifact := range artifacts {
					if artifact.Kind != "bench_target" {
						continue
					}
					if stringMetadata(artifact.SummaryMetadata, "workspace_id") == fixture.workspace {
						count++
					}
				}
				if count == 0 {
					b.Fatalf("expected filtered artifacts")
				}
			}
		})
	}
}

func BenchmarkProvenanceBuild(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "provenance-build-"+scale.name)
			workflowID := "wf-provenance-build-" + scale.name
			seedProvenanceWorkflow(b, fixture, workflowID, scale, false)
			svc := archaeoprovenance.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.Build(context.Background(), workflowID); err != nil {
					b.Fatalf("build provenance: %v", err)
				}
			}
		})
	}
}

func BenchmarkProvenanceBuildWithWorkspaceRecords(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "provenance-workspace-"+scale.name)
			workflowID := "wf-provenance-workspace-" + scale.name
			seedProvenanceWorkflow(b, fixture, workflowID, scale, true)
			svc := archaeoprovenance.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.Build(context.Background(), workflowID); err != nil {
					b.Fatalf("build provenance with workspace records: %v", err)
				}
			}
		})
	}
}

func BenchmarkArchaeologyRefreshBundleLoad(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "refresh-bundle-"+scale.name)
			workflowID := "wf-refresh-bundle-" + scale.name
			seedRefreshWorkflow(b, fixture, workflowID, scale, false)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := buildRefreshReadContext(context.Background(), fixture, workflowID); err != nil {
					b.Fatalf("build refresh read context: %v", err)
				}
			}
		})
	}
}

func BenchmarkArchaeologyRequestReuseDuringRefresh(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "refresh-request-reuse-"+scale.name)
			workflowID := "wf-refresh-request-reuse-" + scale.name
			state, task, svc := seedRefreshWorkflow(b, fixture, workflowID, scale, true)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if out := svc.PrepareLivingPlan(context.Background(), task, state, workflowID); out.Err != nil {
					b.Fatalf("prepare living plan with request reuse: %v", out.Err)
				}
			}
		})
	}
}

func BenchmarkArchaeologyProviderRequestIssuance(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "provider-request-issuance-"+scale.name)
			now := time.Now().UTC()
			providedPatterns := seedPatternRecords(b, fixture.patternStore, fixture.workspace, "inline-provider", max(2, scale.patternCount/2))
			providedTensions := make([]archaeodomain.Tension, 0, max(1, scale.tensionCount/2))
			for i := 0; i < max(1, scale.tensionCount/2); i++ {
				providedTensions = append(providedTensions, archaeodomain.Tension{
					ID:                 fmt.Sprintf("inline-provider-tension-%03d", i),
					SourceRef:          fmt.Sprintf("inline-provider-gap-%03d", i),
					Kind:               "intent_gap",
					Description:        "provider tension",
					Status:             archaeodomain.TensionUnresolved,
					RelatedPlanStepIDs: []string{"resolve_tensions"},
					BasedOnRevision:    "rev-provider",
				})
			}
			svc := archaeologyServiceForFixture(fixture, providers.Bundle{
				PatternSurfacer:     benchPatternSurfacer{records: providedPatterns},
				TensionAnalyzer:     benchTensionAnalyzer{records: providedTensions},
				ProspectiveAnalyzer: benchProspectiveAnalyzer{records: providedPatterns},
				ConvergenceReviewer: benchConvergenceReviewer{},
			}, now)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-provider-request-issuance-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "provider request issuance")
				seedActivePlan(b, fixture, workflowID, "", scale.planStepCount)
				state := benchmarkState()
				task := benchmarkTask(workflowID, "review structure", map[string]any{
					"workspace":    fixture.workspace,
					"corpus_scope": fixture.workspace,
					"symbol_scope": fixture.sourcePath,
				})
				if out := svc.PrepareLivingPlan(context.Background(), task, state, workflowID); out.Err != nil {
					b.Fatalf("provider request issuance: %v", out.Err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceCurrentConvergenceProjection(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "current-convergence-"+scale.name)
			workflowID := "wf-current-convergence-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "current convergence")
			seedWorkspaceRecords(b, fixture, workflowID, scale)
			svc := archaeoconvergence.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.CurrentByWorkspace(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("current convergence projection: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceConvergenceHistoryScan(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "history-convergence-"+scale.name)
			workflowID := "wf-history-convergence-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "convergence history scan")
			seedWorkspaceRecords(b, fixture, workflowID, scale)
			svc := archaeoconvergence.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.ListByWorkspace(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("convergence history scan: %v", err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceDecisionTrailSummary(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "decision-summary-"+scale.name)
			workflowID := "wf-decision-summary-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "decision summary")
			seedWorkspaceRecords(b, fixture, workflowID, scale)
			svc := archaeodecisions.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				records, err := svc.ListByWorkspace(context.Background(), fixture.workspace)
				if err != nil {
					b.Fatalf("list decision records: %v", err)
				}
				resolved := 0
				open := 0
				for _, record := range records {
					switch record.Status {
					case archaeodomain.DecisionStatusResolved:
						resolved++
					default:
						open++
					}
				}
				if open+resolved == 0 {
					b.Fatalf("expected decision records")
				}
			}
		})
	}
}

func BenchmarkWorkspaceDecisionTrailFullHistory(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "decision-history-"+scale.name)
			workflowID := "wf-decision-history-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "decision history")
			seedWorkspaceRecords(b, fixture, workflowID, scale)
			proj := archaeoproj.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := proj.DecisionTrail(context.Background(), fixture.workspace); err != nil {
					b.Fatalf("decision trail full history: %v", err)
				}
			}
		})
	}
}

func BenchmarkRequestClaimRenewExpire(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-claim-renew-expire-"+scale.name)
			workflowID := "wf-request-claim-renew-expire-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "request claim renew expire")
			now := time.Now().UTC()
			svc := archaeorequests.Service{
				Store: fixture.workflowStore,
				Now:   func() time.Time { return now },
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
					WorkflowID:     workflowID,
					Kind:           archaeodomain.RequestPatternSurfacing,
					Title:          fmt.Sprintf("claim-renew-expire-%d", i),
					RequestedBy:    "benchmark",
					IdempotencyKey: fmt.Sprintf("claim-renew-expire-%d", i),
				})
				if err != nil {
					b.Fatalf("create request: %v", err)
				}
				if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, map[string]any{"mode": "bench"}); err != nil {
					b.Fatalf("dispatch request: %v", err)
				}
				if _, err := svc.Claim(context.Background(), archaeorequests.ClaimInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					ClaimedBy:  "benchmark",
					LeaseTTL:   time.Minute,
				}); err != nil {
					b.Fatalf("claim request: %v", err)
				}
				if _, err := svc.Renew(context.Background(), archaeorequests.RenewInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					LeaseTTL:   time.Minute,
				}); err != nil {
					b.Fatalf("renew request: %v", err)
				}
				now = now.Add(2 * time.Minute)
				if _, err := svc.ExpireClaims(context.Background(), workflowID); err != nil {
					b.Fatalf("expire claims: %v", err)
				}
			}
		})
	}
}

func BenchmarkRequestApplyExternalFulfillment(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-external-fulfillment-"+scale.name)
			workflowID := "wf-request-external-fulfillment-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "request external fulfillment")
			svc := archaeorequests.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
					WorkflowID:      workflowID,
					Kind:            archaeodomain.RequestPatternSurfacing,
					Title:           fmt.Sprintf("external-fulfillment-%d", i),
					RequestedBy:     "benchmark",
					IdempotencyKey:  fmt.Sprintf("external-fulfillment-%d", i),
					Input:           map[string]any{"workspace_id": fixture.workspace},
					BasedOnRevision: "rev-current",
				})
				if err != nil {
					b.Fatalf("create request: %v", err)
				}
				if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, map[string]any{"workspace_id": fixture.workspace}); err != nil {
					b.Fatalf("dispatch request: %v", err)
				}
				if _, err := svc.Claim(context.Background(), archaeorequests.ClaimInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					ClaimedBy:  "benchmark",
					LeaseTTL:   time.Minute,
				}); err != nil {
					b.Fatalf("claim request: %v", err)
				}
				if _, _, err := svc.ApplyFulfillment(context.Background(), archaeorequests.ApplyFulfillmentInput{
					WorkflowID:      workflowID,
					RequestID:       record.ID,
					CurrentRevision: "rev-current",
					Fulfillment: archaeodomain.RequestFulfillment{
						Kind:        "external_result",
						Summary:     "fulfilled",
						ExecutorRef: "bench-executor",
						SessionRef:  fmt.Sprintf("session-%d", i),
					},
				}); err != nil {
					b.Fatalf("apply external fulfillment: %v", err)
				}
			}
		})
	}
}

func BenchmarkRequestApplyStaleFulfillmentDecisionPath(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-stale-fulfillment-"+scale.name)
			workflowID := "wf-request-stale-fulfillment-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "request stale fulfillment")
			svc := archaeorequests.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
					WorkflowID:      workflowID,
					Kind:            archaeodomain.RequestPatternSurfacing,
					Title:           fmt.Sprintf("stale-fulfillment-%d", i),
					RequestedBy:     "benchmark",
					IdempotencyKey:  fmt.Sprintf("stale-fulfillment-%d", i),
					Input:           map[string]any{"workspace_id": fixture.workspace},
					BasedOnRevision: "rev-original",
				})
				if err != nil {
					b.Fatalf("create request: %v", err)
				}
				if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, map[string]any{"workspace_id": fixture.workspace}); err != nil {
					b.Fatalf("dispatch request: %v", err)
				}
				if _, err := svc.Claim(context.Background(), archaeorequests.ClaimInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					ClaimedBy:  "benchmark",
					LeaseTTL:   time.Minute,
				}); err != nil {
					b.Fatalf("claim request: %v", err)
				}
				if _, _, err := svc.ApplyFulfillment(context.Background(), archaeorequests.ApplyFulfillmentInput{
					WorkflowID:      workflowID,
					RequestID:       record.ID,
					CurrentRevision: "rev-new",
					Fulfillment: archaeodomain.RequestFulfillment{
						Kind:        "external_result",
						Summary:     "stale",
						ExecutorRef: "bench-executor",
						SessionRef:  fmt.Sprintf("session-%d", i),
					},
				}); err != nil {
					b.Fatalf("apply stale fulfillment: %v", err)
				}
			}
		})
	}
}

func BenchmarkRequestConcurrentClaimReclaim(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-concurrent-claim-"+scale.name)
			workflowID := "wf-request-concurrent-claim-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "request concurrent claim reclaim")
			var counter atomic.Int64
			svc := archaeorequests.Service{Store: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					idx := counter.Add(1)
					record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
						WorkflowID:     workflowID,
						Kind:           archaeodomain.RequestPatternSurfacing,
						Title:          fmt.Sprintf("claim-%d", idx),
						IdempotencyKey: fmt.Sprintf("claim-%d", idx),
						Input:          map[string]any{"ordinal": idx},
					})
					if err != nil {
						b.Fatalf("create request: %v", err)
					}
					if _, err := svc.Dispatch(context.Background(), workflowID, record.ID, nil); err != nil {
						b.Fatalf("dispatch request: %v", err)
					}
					if _, err := svc.Claim(context.Background(), archaeorequests.ClaimInput{
						WorkflowID: workflowID,
						RequestID:  record.ID,
						ClaimedBy:  "bench-claimer",
						LeaseTTL:   time.Minute,
					}); err != nil {
						b.Fatalf("claim request: %v", err)
					}
					if _, err := svc.Release(context.Background(), workflowID, record.ID); err != nil {
						b.Fatalf("release request: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkRequestConcurrentApplyIndependentFulfillments(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "request-concurrent-apply-"+scale.name)
			workflowID := "wf-request-concurrent-apply-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "request concurrent apply")
			svc := archaeorequests.Service{Store: fixture.workflowStore}
			requestIDs := make([]string, 0, max(8, scale.interactionCount))
			for i := 0; i < max(8, scale.interactionCount); i++ {
				record, err := svc.Create(context.Background(), archaeorequests.CreateInput{
					WorkflowID:      workflowID,
					Kind:            archaeodomain.RequestProspectiveAnalysis,
					Title:           fmt.Sprintf("apply-%03d", i),
					IdempotencyKey:  fmt.Sprintf("apply-%03d", i),
					Input:           map[string]any{"ordinal": i},
					BasedOnRevision: "rev-1",
				})
				if err != nil {
					b.Fatalf("create request: %v", err)
				}
				record, err = svc.Dispatch(context.Background(), workflowID, record.ID, nil)
				if err != nil {
					b.Fatalf("dispatch request: %v", err)
				}
				record, err = svc.Claim(context.Background(), archaeorequests.ClaimInput{
					WorkflowID: workflowID,
					RequestID:  record.ID,
					ClaimedBy:  "bench-executor",
					LeaseTTL:   time.Minute,
				})
				if err != nil {
					b.Fatalf("claim request: %v", err)
				}
				requestIDs = append(requestIDs, record.ID)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				for idx, requestID := range requestIDs {
					wg.Add(1)
					go func(idx int, requestID string) {
						defer wg.Done()
						if _, _, err := svc.ApplyFulfillment(context.Background(), archaeorequests.ApplyFulfillmentInput{
							WorkflowID:      workflowID,
							RequestID:       requestID,
							CurrentRevision: "rev-1",
							Fulfillment: archaeodomain.RequestFulfillment{
								Kind:        "result",
								RefID:       fmt.Sprintf("result-%03d-%03d", i, idx),
								Summary:     "fulfilled",
								ExecutorRef: "bench-executor",
							},
						}); err != nil {
							b.Fatalf("apply fulfillment: %v", err)
						}
					}(idx, requestID)
				}
				wg.Wait()
			}
		})
	}
}

func BenchmarkExecutionCreateDeferredDraftRequest(b *testing.B) {
	benchmarkExecutionDeferredDraft(b, archaeodomain.DispositionRequireReplan, archaeodomain.MutationPlanStaleness, "execution-deferred-request")
}

func BenchmarkExecutionLiveCheckpointWithDeferredDraftCreation(b *testing.B) {
	benchmarkExecutionDeferredDraft(b, archaeodomain.DispositionPauseForGuidance, archaeodomain.MutationBlockingSemantic, "execution-live-checkpoint")
}

func benchmarkExecutionDeferredDraft(b *testing.B, disposition archaeodomain.ExecutionDisposition, category archaeodomain.MutationCategory, prefix string) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, prefix+"-"+scale.name)
			coordinator := archaeoexec.LiveMutationCoordinator{
				Service: archaeoexec.Service{WorkflowStore: fixture.workflowStore},
				Plans:   archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-%s-%s-%d", prefix, scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "execution deferred draft benchmark")
				if err := appendExecutionMutation(context.Background(), fixture, workflowID, disposition, category); err != nil {
					b.Fatalf("append mutation: %v", err)
				}
				plan := &frameworkplan.LivingPlan{
					ID:         "plan-1",
					WorkflowID: workflowID,
					Version:    1,
				}
				step := &frameworkplan.PlanStep{ID: "step-1"}
				state := benchmarkState()
				state.Set("euclo.workspace", fixture.workspace)
				state.Set("euclo.based_on_revision", "rev-bench")
				state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
					WorkflowID:  workflowID,
					PlanID:      plan.ID,
					PlanVersion: plan.Version,
				})
				task := benchmarkTask(workflowID, "execute implementation", map[string]any{"workspace": fixture.workspace})
				if _, err := coordinator.CheckpointExecutionAt(context.Background(), archaeodomain.MutationCheckpointPreVerification, task, state, plan, step); err == nil {
					b.Fatalf("expected execution checkpoint to interrupt for disposition %s", disposition)
				}
			}
		})
	}
}

func seedMixedArtifacts(tb testing.TB, fixture *benchmarkFixture, workflowID, workspaceID string, scale benchScale) {
	tb.Helper()
	now := time.Now().UTC()
	total := max(8, scale.timelineEventCount/4)
	for i := 0; i < total; i++ {
		workspace := workspaceID
		if i%3 == 0 {
			workspace = workspaceID + "-other"
		}
		kind := "bench_other"
		if i%2 == 0 {
			kind = "bench_target"
		}
		if err := fixture.workflowStore.UpsertWorkflowArtifact(context.Background(), memory.WorkflowArtifactRecord{
			ArtifactID:      fmt.Sprintf("artifact-%s-%03d", workflowID, i),
			WorkflowID:      workflowID,
			Kind:            kind,
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     kind,
			SummaryMetadata: map[string]any{"workspace_id": workspace, "ordinal": i},
			InlineRawText:   fmt.Sprintf(`{"kind":%q,"workspace_id":%q,"ordinal":%d}`, kind, workspace, i),
			CreatedAt:       now.Add(time.Duration(i) * time.Millisecond),
		}); err != nil {
			tb.Fatalf("seed mixed artifact: %v", err)
		}
	}
}

func listArtifactsByKindAndWorkspace(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error) {
	if typed, ok := store.(workspaceArtifactLister); ok {
		return typed.ListWorkflowArtifactsByKindAndWorkspace(ctx, workflowID, runID, kind, workspaceID)
	}
	artifacts, err := archaeointernal.ListWorkflowArtifactsByKind(ctx, store, workflowID, runID, kind)
	if err != nil {
		return nil, err
	}
	out := make([]memory.WorkflowArtifactRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		if stringMetadata(artifact.SummaryMetadata, "workspace_id") == workspaceID {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func stringMetadata(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func seedProvenanceWorkflow(tb testing.TB, fixture *benchmarkFixture, workflowID string, scale benchScale, includeWorkspaceRecords bool) {
	tb.Helper()
	mustCreateWorkflow(tb, fixture.workflowStore, workflowID, "seed provenance workflow")
	session, snapshot := seedExploration(tb, fixture, workflowID, fixture.workspace)
	seedPatternRecords(tb, fixture.patternStore, fixture.workspace, "provenance", scale.patternCount)
	seedLearningInteractions(tb, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
	seedTensions(tb, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
	seedPlanVersionHistory(tb, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
	seedRequestHistory(tb, fixture, workflowID, session.ID, snapshot.ID, scale.interactionCount)
	appendMutationBatch(tb, fixture, workflowID, "plan-1", max(1, scale.planVersionCount), "step-1", max(1, scale.interactionCount/2))
	if includeWorkspaceRecords {
		seedWorkspaceRecords(tb, fixture, workflowID, scale)
	}
}

func seedRefreshWorkflow(tb testing.TB, fixture *benchmarkFixture, workflowID string, scale benchScale, withRequests bool) (*core.Context, *core.Task, archaeoarch.Service) {
	tb.Helper()
	mustCreateWorkflow(tb, fixture.workflowStore, workflowID, "seed refresh workflow")
	session, snapshot := seedExploration(tb, fixture, workflowID, fixture.workspace)
	seedPatternRecords(tb, fixture.patternStore, fixture.workspace, "refresh", scale.patternCount)
	seedLearningInteractions(tb, fixture, workflowID, session.ID, scale.name, scale.interactionCount)
	seedTensions(tb, fixture, workflowID, session.ID, snapshot.ID, scale.name, scale.tensionCount)
	seedPlanVersionHistory(tb, fixture, workflowID, session.ID, scale.planStepCount, scale.planVersionCount)
	if withRequests {
		seedRequestHistory(tb, fixture, workflowID, session.ID, snapshot.ID, scale.interactionCount)
	}
	state := benchmarkState()
	state.Set("euclo.workspace", fixture.workspace)
	state.Set("euclo.active_exploration_snapshot_id", snapshot.ID)
	state.Set("euclo.active_exploration_id", session.ID)
	task := benchmarkTask(workflowID, "refresh archaeology", map[string]any{
		"workspace":      fixture.workspace,
		"corpus_scope":   fixture.workspace,
		"symbol_scope":   fixture.sourcePath,
		"exploration_id": session.ID,
	})
	svc := archaeologyServiceForFixture(fixture, providers.Bundle{}, time.Now().UTC())
	return state, task, svc
}

func buildRefreshReadContext(ctx context.Context, fixture *benchmarkFixture, workflowID string) (map[string]any, error) {
	learningSvc := archaeolearning.Service{
		Store:        fixture.workflowStore,
		PatternStore: fixture.patternStore,
		CommentStore: fixture.commentStore,
		PlanStore:    fixture.planStore,
		Retrieval:    archaeoretrieval.NewSQLStore(fixture.retrievalDB),
	}
	lineage, err := (archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore}).LoadLineage(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	view, err := (archaeoarch.Service{Store: fixture.workflowStore, Learning: learningSvc}).LoadExplorationViewByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	pending, err := learningSvc.Pending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	blocking, err := learningSvc.BlockingPending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	tensions, err := (archaeotensions.Service{Store: fixture.workflowStore}).ActiveByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	requests, err := (archaeorequests.Service{Store: fixture.workflowStore}).Pending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	provenance, err := (archaeoprovenance.Service{Store: fixture.workflowStore}).Build(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"view":       view,
		"lineage":    lineage,
		"pending":    pending,
		"blocking":   blocking,
		"tensions":   tensions,
		"requests":   requests,
		"provenance": provenance,
	}, nil
}

func archaeologyServiceForFixture(fixture *benchmarkFixture, bundle providers.Bundle, now time.Time) archaeoarch.Service {
	return archaeoarch.Service{
		Store: fixture.workflowStore,
		Plans: archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore},
		Learning: archaeolearning.Service{
			Store:        fixture.workflowStore,
			PatternStore: fixture.patternStore,
			CommentStore: fixture.commentStore,
			PlanStore:    fixture.planStore,
			Retrieval:    archaeoretrieval.NewSQLStore(fixture.retrievalDB),
		},
		Requests:  archaeorequests.Service{Store: fixture.workflowStore},
		Providers: bundle,
		PersistPhase: func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep) {
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
		Now: func() time.Time { return now },
	}
}

func seedWorkspaceRecords(tb testing.TB, fixture *benchmarkFixture, workflowID string, scale benchScale) {
	tb.Helper()
	deferredSvc := archaeodeferred.Service{Store: fixture.workflowStore}
	convergenceSvc := archaeoconvergence.Service{Store: fixture.workflowStore}
	decisionSvc := archaeodecisions.Service{Store: fixture.workflowStore}
	for i := 0; i < max(1, scale.interactionCount/2); i++ {
		deferred, err := deferredSvc.CreateOrUpdate(context.Background(), archaeodeferred.CreateInput{
			WorkspaceID:  fixture.workspace,
			WorkflowID:   workflowID,
			PlanID:       "plan-1",
			PlanVersion:  intPointer(1),
			AmbiguityKey: fmt.Sprintf("ambiguity-%03d", i),
			Title:        "Deferred draft",
			Metadata:     map[string]any{"ordinal": i},
		})
		if err != nil {
			tb.Fatalf("seed deferred draft: %v", err)
		}
		record, err := convergenceSvc.Create(context.Background(), archaeoconvergence.CreateInput{
			WorkspaceID:        fixture.workspace,
			WorkflowID:         workflowID,
			PlanID:             "plan-1",
			PlanVersion:        intPointer(1),
			Question:           fmt.Sprintf("question-%03d", i),
			DeferredDraftIDs:   []string{deferred.ID},
			RelevantTensionIDs: []string{fmt.Sprintf("tension-%03d", i)},
			PendingLearningIDs: []string{fmt.Sprintf("learning-%03d", i)},
			ProvenanceRefs:     []string{fmt.Sprintf("prov-%03d", i)},
		})
		if err != nil {
			tb.Fatalf("seed convergence: %v", err)
		}
		if i%2 == 0 {
			if _, err := convergenceSvc.Resolve(context.Background(), archaeoconvergence.ResolveInput{
				WorkflowID: workflowID,
				RecordID:   record.ID,
				Resolution: archaeodomain.ConvergenceResolution{
					Status:  archaeodomain.ConvergenceResolutionDeferred,
					Summary: "defer",
				},
			}); err != nil {
				tb.Fatalf("resolve convergence: %v", err)
			}
		}
		decision, err := decisionSvc.Create(context.Background(), archaeodecisions.CreateInput{
			WorkspaceID:            fixture.workspace,
			WorkflowID:             workflowID,
			Kind:                   archaeodomain.DecisionKindStaleResult,
			Title:                  fmt.Sprintf("decision-%03d", i),
			RelatedConvergenceID:   record.ID,
			RelatedDeferredDraftID: deferred.ID,
			CommentRefs:            []string{fmt.Sprintf("comment-%03d", i)},
			Metadata:               map[string]any{"provenance_ref": fmt.Sprintf("prov-%03d", i)},
		})
		if err != nil {
			tb.Fatalf("seed decision: %v", err)
		}
		if i%2 == 0 {
			if _, err := decisionSvc.Resolve(context.Background(), archaeodecisions.ResolveInput{
				WorkflowID: workflowID,
				RecordID:   decision.ID,
				Status:     archaeodomain.DecisionStatusResolved,
			}); err != nil {
				tb.Fatalf("resolve decision: %v", err)
			}
		}
	}
}

func appendExecutionMutation(ctx context.Context, fixture *benchmarkFixture, workflowID string, disposition archaeodomain.ExecutionDisposition, category archaeodomain.MutationCategory) error {
	now := time.Now().UTC()
	version := 1
	return archaeoevents.AppendMutationEvent(ctx, fixture.workflowStore, archaeodomain.MutationEvent{
		ID:              fmt.Sprintf("mutation-%s-%s", workflowID, disposition),
		WorkflowID:      workflowID,
		PlanID:          "plan-1",
		PlanVersion:     &version,
		StepID:          "step-1",
		Category:        category,
		Impact:          archaeodomain.ImpactPlanRecomputeRequired,
		Disposition:     disposition,
		Blocking:        disposition != archaeodomain.DispositionContinueOnStalePlan && disposition != archaeodomain.DispositionContinue,
		Description:     "benchmark execution mutation",
		BlastRadius:     archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusStep, AffectedStepIDs: []string{"step-1"}},
		CreatedAt:       now,
		BasedOnRevision: "rev-bench",
	})
}

func intPointer(value int) *int {
	copy := value
	return &copy
}
