package euclo

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

type retrievalProviderStub struct {
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
	know   []memory.KnowledgeRecord
}

func (s retrievalProviderStub) RetrievalService() retrieval.RetrieverService {
	return retrievalServiceStub{blocks: s.blocks, event: s.event}
}

func (s retrievalProviderStub) ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error) {
	return append([]memory.KnowledgeRecord{}, s.know...), nil
}

type retrievalServiceStub struct {
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
}

func (s retrievalServiceStub) Retrieve(context.Context, retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return s.blocks, s.event, nil
}

func TestExpandContextPrefersLocalPathsWithoutWorkflowWideningForCode(t *testing.T) {
	policy := ResolveRetrievalPolicy(ModeResolution{ModeID: "code"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"})
	expansion, err := ExpandContext(context.Background(), retrievalProviderStub{}, "wf-1", &core.Task{
		Instruction: "fix bug",
		Context:     map[string]any{"file_path": "main.go"},
	}, core.NewContext(), policy)
	require.NoError(t, err)
	require.Equal(t, []string{"main.go"}, expansion.LocalPaths)
	require.False(t, expansion.WidenedToWorkflow)
	require.Empty(t, expansion.WorkflowRetrieval)
}

func TestExpandContextWidensToWorkflowForPlanningMode(t *testing.T) {
	policy := ResolveRetrievalPolicy(ModeResolution{ModeID: "planning"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"})
	expansion, err := ExpandContext(context.Background(), retrievalProviderStub{
		blocks: []core.ContentBlock{
			core.TextContentBlock{Text: "workflow context"},
		},
		event: retrieval.RetrievalEvent{QueryID: "rq-1", CacheTier: "l2_hot"},
	}, "wf-1", &core.Task{
		Instruction: "plan the migration",
		Context:     map[string]any{"file_path": "main.go"},
	}, core.NewContext(), policy)
	require.NoError(t, err)
	require.True(t, expansion.WidenedToWorkflow)
	require.NotEmpty(t, expansion.WorkflowRetrieval)
	require.True(t, strings.Contains(expansion.Summary, "workflow_retrieval"))
}

func TestExpandContextFallsBackToWorkflowKnowledgeWhenSemanticRetrievalEmpty(t *testing.T) {
	policy := ResolveRetrievalPolicy(ModeResolution{ModeID: "debug"}, ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"})
	expansion, err := ExpandContext(context.Background(), retrievalProviderStub{
		know: []memory.KnowledgeRecord{{Title: "Issue", Content: "panic in parser"}},
	}, "wf-1", &core.Task{
		Instruction: "debug failing parser",
	}, core.NewContext(), policy)
	require.NoError(t, err)
	require.True(t, expansion.WidenedToWorkflow)
	require.Equal(t, "Issue: panic in parser", expansion.WorkflowRetrieval["summary"])
}
