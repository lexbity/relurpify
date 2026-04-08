package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	relurpicadapters "github.com/lexcodex/relurpify/archaeo/adapters/relurpic"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

func BenchmarkEvaluateMutations(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "evaluate-mutations-"+scale.name)
			workflowID := "wf-evaluate-mutations-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "evaluate execution-time mutations")
			plan := benchmarkExecutionPlan(workflowID)
			step := plan.Steps["step-1"]
			handoff := &archaeodomain.ExecutionHandoff{
				WorkflowID:  workflowID,
				PlanID:      plan.ID,
				PlanVersion: plan.Version,
				CreatedAt:   time.Now().UTC().Add(-time.Minute),
			}
			appendMutationBatch(b, fixture, workflowID, plan.ID, plan.Version, step.ID, scale.interactionCount)
			svc := archaeoexec.Service{WorkflowStore: fixture.workflowStore}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				eval, err := svc.EvaluateMutations(context.Background(), workflowID, handoff, plan, step)
				if err != nil {
					b.Fatalf("evaluate mutations: %v", err)
				}
				if eval == nil {
					b.Fatalf("expected mutation evaluation for %s", scale.name)
				}
			}
		})
	}
}

func BenchmarkCheckpointExecution(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "checkpoint-execution-"+scale.name)
			workflowID := "wf-checkpoint-" + scale.name
			mustCreateWorkflow(b, fixture.workflowStore, workflowID, "checkpoint execution")
			plan := benchmarkExecutionPlan(workflowID)
			step := plan.Steps["step-1"]
			appendMutationBatch(b, fixture, workflowID, plan.ID, plan.Version, step.ID, scale.interactionCount)
			coord := archaeoexec.LiveMutationCoordinator{
				Service: archaeoexec.Service{
					WorkflowStore:  fixture.workflowStore,
					MutationPolicy: archaeoexec.MutationPolicy{ContinueOnStalePlan: true},
				},
				Plans: archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore},
				RequestGuidance: func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision {
					return guidance.GuidanceDecision{ChoiceID: "proceed", DecidedBy: "benchmark"}
				},
			}
			task := benchmarkTask(workflowID, "execute active step", nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state := core.NewContext()
				state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
					WorkflowID:  workflowID,
					PlanID:      plan.ID,
					PlanVersion: plan.Version,
					CreatedAt:   time.Now().UTC().Add(-time.Minute),
				})
				eval, err := coord.CheckpointExecution(context.Background(), task, state, clonePlan(plan), clonePlan(plan).Steps["step-1"])
				if err != nil {
					b.Fatalf("checkpoint execution: %v", err)
				}
				if eval == nil {
					b.Fatalf("expected checkpoint evaluation for %s", scale.name)
				}
			}
		})
	}
}

func BenchmarkRelurpicPatternSurfacingAndSyncLearning(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "relurpic-surface-patterns-"+scale.name)
			service := relurpicadapters.Service{
				Providers: relurpicadapters.Runtime{
					Config:       &core.Config{Name: "coding", Model: "stub"},
					Registry:     fixture.registry,
					IndexManager: fixture.indexManager,
					GraphDB:      fixture.graph,
					PatternStore: fixture.patternStore,
					Retrieval:    archaeoretrieval.NewSQLStore(fixture.retrievalDB),
				}.Bundle(),
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
				workflowID := fmt.Sprintf("wf-relurpic-patterns-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "surface patterns")
				session, _ := seedExploration(b, fixture, workflowID, fixture.workspace)
				service.Providers = relurpicadapters.Runtime{
					Model:         &benchProviderModel{patternCount: scale.patternCount},
					Config:        &core.Config{Name: "coding", Model: "stub"},
					Registry:      fixture.registry,
					IndexManager:  fixture.indexManager,
					GraphDB:       fixture.graph,
					PatternStore:  fixture.patternStore,
					CommentStore:  fixture.commentStore,
					Retrieval:     archaeoretrieval.NewSQLStore(fixture.retrievalDB),
					PlanStore:     fixture.planStore,
					WorkflowStore: fixture.workflowStore,
				}.Bundle()
				records, interactions, err := service.SurfacePatternsAndSyncLearning(context.Background(), providers.PatternSurfacingRequest{
					WorkflowID:      workflowID,
					ExplorationID:   session.ID,
					WorkspaceID:     fixture.workspace,
					SymbolScope:     fixture.sourcePath,
					CorpusScope:     fixture.workspace,
					MaxProposals:    scale.patternCount,
					BasedOnRevision: fmt.Sprintf("rev-patterns-%d", i),
				})
				if err != nil {
					b.Fatalf("surface patterns and sync learning: %v", err)
				}
				if len(records) == 0 || len(interactions) == 0 {
					b.Fatalf("expected relurpic pattern output for %s", scale.name)
				}
			}
		})
	}
}

func BenchmarkRelurpicTensionAnalysisAndPersist(b *testing.B) {
	for _, scale := range benchScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newBenchmarkFixture(b, "relurpic-tensions-"+scale.name)
			anchor, err := retrieval.DeclareAnchor(context.Background(), fixture.retrievalDB, retrieval.AnchorDeclaration{
				Term:       "error wrapping",
				Definition: "Boundary functions wrap returned errors.",
				Class:      "commitment",
			}, fixture.workspace, string(patterns.TrustClassBuiltinTrusted))
			if err != nil {
				b.Fatalf("declare benchmark anchor: %v", err)
			}
			service := relurpicadapters.Service{
				Learning: archaeolearning.Service{
					Store:     fixture.workflowStore,
					PlanStore: fixture.planStore,
				},
				Tensions: archaeotensions.Service{Store: fixture.workflowStore},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				workflowID := fmt.Sprintf("wf-relurpic-tensions-%s-%d", scale.name, i)
				mustCreateWorkflow(b, fixture.workflowStore, workflowID, "analyze tensions")
				session, snapshot := seedExploration(b, fixture, workflowID, fixture.workspace)
				service.Providers = relurpicadapters.Runtime{
					Model:         &benchProviderModel{tensionCount: scale.tensionCount},
					Config:        &core.Config{Name: "coding", Model: "stub"},
					Registry:      fixture.registry,
					IndexManager:  fixture.indexManager,
					GraphDB:       fixture.graph,
					Retrieval:     archaeoretrieval.NewSQLStore(fixture.retrievalDB),
					PlanStore:     fixture.planStore,
					WorkflowStore: fixture.workflowStore,
				}.Bundle()
				records, interactions, err := service.AnalyzeAndPersistTensions(context.Background(), providers.TensionAnalysisRequest{
					WorkflowID:      workflowID,
					ExplorationID:   session.ID,
					SnapshotID:      snapshot.ID,
					WorkspaceID:     fixture.workspace,
					FilePath:        fixture.sourcePath,
					AnchorIDs:       []string{anchor.AnchorID},
					BasedOnRevision: fmt.Sprintf("rev-tensions-%d", i),
				})
				if err != nil {
					b.Fatalf("analyze tensions and persist: %v", err)
				}
				if len(records) == 0 || len(interactions) == 0 {
					b.Fatalf("expected relurpic tension output for %s", scale.name)
				}
			}
		})
	}
}

type benchProviderModel struct {
	patternCount int
	tensionCount int
}

func (m *benchProviderModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	switch {
	case m.patternCount > 0:
		return &core.LLMResponse{Text: patternProposalJSON(m.patternCount)}, nil
	case m.tensionCount > 0:
		return &core.LLMResponse{Text: tensionResultsJSON(m.tensionCount)}, nil
	default:
		return &core.LLMResponse{Text: `{"proposals":[]}`}, nil
	}
}

func (m *benchProviderModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *benchProviderModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return m.Generate(context.Background(), "", nil)
}

func (m *benchProviderModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return m.Generate(context.Background(), "", nil)
}

func patternProposalJSON(count int) string {
	if count <= 0 {
		return `{"proposals":[]}`
	}
	text := `{"proposals":[`
	for i := 0; i < count; i++ {
		if i > 0 {
			text += ","
		}
		text += fmt.Sprintf(`{"kind":"boundary","title":"Pattern %03d","description":"Pattern %03d benchmark description.","instances":[{"file_path":"","start_line":1,"end_line":7,"excerpt":"func Wrap(err error) error {\n\tif err == nil {\n\t\treturn nil\n\t}\n\treturn err\n}"}],"confidence":0.87}`, i, i)
	}
	return text + `]}`
}

func tensionResultsJSON(count int) string {
	if count <= 0 {
		return `{"results":[]}`
	}
	text := `{"results":[`
	for i := 0; i < count; i++ {
		if i > 0 {
			text += ","
		}
		text += fmt.Sprintf(`{"severity":"significant","description":"Benchmark contradiction %03d.","evidence_lines":[5]}`, i)
	}
	return text + `]}`
}

func benchmarkExecutionPlan(workflowID string) *frameworkplan.LivingPlan {
	now := time.Now().UTC()
	return &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: workflowID,
		Version:    1,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:              "step-1",
				Description:     "active benchmark step",
				Status:          frameworkplan.PlanStepPending,
				ConfidenceScore: 0.7,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func clonePlan(plan *frameworkplan.LivingPlan) *frameworkplan.LivingPlan {
	cloned := *plan
	cloned.Steps = make(map[string]*frameworkplan.PlanStep, len(plan.Steps))
	for id, step := range plan.Steps {
		copyStep := *step
		cloned.Steps[id] = &copyStep
	}
	cloned.StepOrder = append([]string(nil), plan.StepOrder...)
	return &cloned
}

func appendMutationBatch(tb testing.TB, fixture *benchmarkFixture, workflowID, planID string, planVersion int, stepID string, count int) {
	tb.Helper()
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		mutation := archaeodomain.MutationEvent{
			WorkflowID:  workflowID,
			PlanID:      planID,
			PlanVersion: intPtr(planVersion),
			Category:    archaeodomain.MutationObservation,
			SourceKind:  "learning_interaction",
			SourceRef:   fmt.Sprintf("mutation-source-%03d", i),
			Description: fmt.Sprintf("supplemental mutation %03d", i),
			BlastRadius: archaeodomain.BlastRadius{
				Scope:           archaeodomain.BlastRadiusStep,
				AffectedStepIDs: []string{stepID},
				EstimatedCount:  1,
			},
			Impact:      archaeodomain.ImpactInformational,
			Disposition: archaeodomain.DispositionContinue,
			CreatedAt:   now.Add(time.Duration(i) * time.Millisecond),
		}
		if i == count-1 {
			mutation.Category = archaeodomain.MutationPlanStaleness
			mutation.SourceKind = "plan_version"
			mutation.Impact = archaeodomain.ImpactPlanRecomputeRequired
			mutation.Disposition = archaeodomain.DispositionContinueOnStalePlan
		}
		if err := archaeoevents.AppendMutationEvent(context.Background(), fixture.workflowStore, mutation); err != nil {
			tb.Fatalf("append mutation event: %v", err)
		}
	}
}

func intPtr(v int) *int { return &v }
