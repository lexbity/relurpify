package runtime

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/stretchr/testify/require"
)

type fakeRetrieverService struct {
	result []core.ContentBlock
	event  retrieval.RetrievalEvent
}

func (f fakeRetrieverService) Retrieve(_ context.Context, query retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return f.result, f.event, nil
}

func TestRetrievalAndSemanticHelpers(t *testing.T) {
	mode := euclotypes.ModeResolution{ModeID: "planning"}
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "profile"}
	policy := ResolveRetrievalPolicy(mode, profile)
	require.True(t, policy.WidenToWorkflow)
	require.Equal(t, 6, policy.WorkflowLimit)

	task := &core.Task{
		Instruction: "implement feature",
		Metadata:    map[string]string{"primary": "  /tmp/a.go", "duplicate": "/tmp/a.go"},
		Context: map[string]any{
			"path":         " /tmp/b.go ",
			"verification": "go test ./...",
		},
	}
	expansion := ContextExpansion{LocalPaths: []string{"/tmp/a.go"}, WidenedToWorkflow: true, WorkflowRetrieval: map[string]any{"texts": []string{"match"}}, ExpansionStrategy: "local_then_workflow"}
	cloned := ApplyContextExpansion(nil, task, expansion)
	require.NotNil(t, cloned)
	require.Equal(t, expansion, cloned.Context["euclo.context_expansion"])
	require.Contains(t, cloned.Context["euclo.local_paths"], "/tmp/a.go")
	require.Equal(t, []string{"match"}, cloned.Context["workflow_retrieval"].(map[string]any)["texts"])

	require.ElementsMatch(t, []string{"/tmp/a.go", "/tmp/b.go"}, taskPaths(task))
	require.Equal(t, "implement feature", taskInstruction(task))
	require.Equal(t, "go test ./...", taskVerification(task))
	require.Equal(t, "local_paths=1 workflow_retrieval", summarizeExpansion(expansion))
	require.Equal(t, "", summarizeExpansion(ContextExpansion{}))

	results := contentBlockResults([]core.ContentBlock{
		core.TextContentBlock{Text: "  alpha  "},
		core.StructuredContentBlock{Data: map[string]any{"text": "beta", "citations": []retrieval.PackedCitation{{DocID: "doc-1"}}}},
	})
	require.Len(t, results, 2)
	require.Equal(t, "alpha", results[0].Text)
	require.Equal(t, "beta", results[1].Text)
	require.Len(t, results[1].Citations, 1)
	require.Len(t, parseWorkflowRetrievalCitations([]retrieval.PackedCitation{{DocID: "doc-1"}}), 1)
	require.Len(t, parseWorkflowRetrievalCitations([]any{retrieval.PackedCitation{DocID: "doc-2"}}), 1)
	require.Nil(t, parseWorkflowRetrievalCitations("bad"))

	retrieved, err := hydrateWorkflowRetrieval(context.Background(), fakeProviderWithKnowledge{service: fakeRetrieverService{
		result: []core.ContentBlock{
			core.TextContentBlock{Text: "first"},
		},
		event: retrieval.RetrievalEvent{CacheTier: "mem", QueryID: "q1"},
	}}, "wf-1", workflowRetrievalQuery{Primary: "alpha", TaskText: "alpha", Verification: "verify"}, 3, 100)
	require.NoError(t, err)
	require.Equal(t, "workflow:wf-1", retrieved["scope"])
	require.Equal(t, 1, retrieved["result_size"])

	semantic := SemanticInputBundleFromSources(
		"wf-1",
		&archaeodomain.VersionedLivingPlan{
			ID:                      "plan-1",
			Version:                 2,
			Status:                  archaeodomain.LivingPlanVersionActive,
			DerivedFromExploration:  " exploration-1 ",
			BasedOnRevision:         " rev-1 ",
			PatternRefs:             []string{"p1", "p1"},
			TensionRefs:             []string{"t1"},
			FormationProvenanceRefs: []string{"prov-1"},
			SemanticSnapshotRef:     " snap-1 ",
			Plan:                    frameworkplan.LivingPlan{Title: "Plan title"},
		},
		&SemanticRequestHistory{Requests: []archaeodomain.RequestRecord{{
			ID:          "req-1",
			Kind:        archaeodomain.RequestPatternSurfacing,
			Status:      archaeodomain.RequestStatusPending,
			Title:       "Pattern request",
			Description: "Need patterns",
			SubjectRefs: []string{"p2"},
		}}},
		&SemanticProvenance{
			Requests:        []SemanticRequestProvenanceRef{{RequestID: "req-prov"}},
			Learning:        []SemanticLearningRef{{InteractionID: "learn-1"}},
			Tensions:        []SemanticTensionRef{{TensionID: "ten-1", PatternIDs: []string{"p3"}, AnchorRefs: []string{"a1"}}},
			PlanVersions:    []SemanticPlanVersionRef{{PatternRefs: []string{"p4"}, TensionRefs: []string{"t2"}, FormationProvenanceRefs: []string{"prov-2"}, FormationResultRef: "result-1", SemanticSnapshotRef: "snap-2"}},
			ConvergenceRefs: []string{"conv-1"},
			DecisionRefs:    []string{"dec-1"},
		},
		&SemanticLearningQueue{PendingLearning: []SemanticLearningRef{{InteractionID: "learn-2"}}},
		&archaeodomain.WorkspaceConvergenceProjection{Current: &archaeodomain.ConvergenceRecord{ID: "conv-current", Status: archaeodomain.ConvergenceResolutionResolved, Title: "Current", Question: "Why?"}},
	)
	require.Equal(t, "wf-1", semantic.WorkflowID)
	require.Contains(t, semantic.PatternRefs, "p2")
	require.Contains(t, semantic.LearningInteractionRefs, "learn-2")
	require.NotEmpty(t, semantic.ConvergenceFindings)
}

type fakeProvider struct {
	service fakeRetrieverService
}

func (f fakeProvider) RetrievalService() retrieval.RetrieverService { return f.service }

type fakeProviderWithKnowledge struct {
	service fakeRetrieverService
}

func (f fakeProviderWithKnowledge) RetrievalService() retrieval.RetrieverService { return f.service }

func (f fakeProviderWithKnowledge) ListKnowledge(context.Context, string, memory.KnowledgeKind, bool) ([]memory.KnowledgeRecord, error) {
	return nil, nil
}

func TestExecutionPolicyAndAmbiguityHelpers(t *testing.T) {
	spec := &core.AgentRuntimeSpec{}
	policy := contextPolicySummary(spec)
	require.NotEmpty(t, policy.PreferredDetail)

	execEnvelope := BuildExecutionEnvelope(&core.Task{}, nil, euclotypes.ModeResolution{ModeID: "planning"}, euclotypes.ExecutionProfileSelection{ProfileID: "profile"}, agentenv.AgentEnvironment{}, nil, nil, "wf-1", "run-1", nil)
	require.Equal(t, "wf-1", execEnvelope.WorkflowID)
	require.Equal(t, "run-1", execEnvelope.RunID)

	rp := BuildResolvedExecutionPolicy(&core.Task{}, nil, nil, euclotypes.ModeResolution{ModeID: "review"}, euclotypes.ExecutionProfileSelection{ProfileID: "profile"})
	require.Equal(t, "review", rp.ModeID)
	require.False(t, rp.ResolvedFromSkillPolicy)

	desc := SelectExecutorDescriptor(
		euclotypes.ModeResolution{ModeID: "planning"},
		euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		TaskClassification{RequiresDeterministicStages: true},
		rp,
		&UnitOfWorkPlanBinding{IsLongRunning: true},
		"",
		nil,
	)
	require.NotEmpty(t, desc.ExecutorID)

	frame := BuildAmbiguityFrame(ScoredClassification{Candidates: []ModeCandidate{{Mode: "debug"}, {Mode: "planning"}}})
	require.Equal(t, interaction.FrameQuestion, frame.Kind)
	require.NotNil(t, frame.Content)
	require.NotEmpty(t, frame.Actions)
	require.Equal(t, "debug", ResolveAmbiguity(ScoredClassification{Candidates: []ModeCandidate{{Mode: "debug"}}}, interaction.UserResponse{ActionID: "debug"}))
	require.Equal(t, "planning", ResolveAmbiguity(ScoredClassification{Candidates: []ModeCandidate{{Mode: "debug"}}}, interaction.UserResponse{ActionID: "planning"}))
	require.Equal(t, "Debug — investigate first", modeLabel("debug"))
	require.Equal(t, "", modeDescription("other"))
}
