package agents

import (
	"context"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/core"
)

type WorkflowRetrievalProvider = workflowutil.RetrievalProvider
type WorkflowRetrievalQuery = workflowutil.RetrievalQuery
type workflowRetrievalQuery = WorkflowRetrievalQuery

func HydrateWorkflowRetrieval(ctx context.Context, provider WorkflowRetrievalProvider, workflowID string, query WorkflowRetrievalQuery, limit, maxTokens int) (map[string]any, error) {
	return workflowutil.Hydrate(ctx, provider, workflowID, workflowutil.RetrievalQuery(query), limit, maxTokens)
}

func ApplyWorkflowRetrievalState(state *core.Context, key string, payload map[string]any) {
	workflowutil.ApplyState(state, key, payload)
}

func ApplyWorkflowRetrievalTask(task *core.Task, payload map[string]any) *core.Task {
	return workflowutil.ApplyTask(task, payload)
}

func WorkflowRetrievalPayloadKey(base string) string {
	return workflowutil.PayloadKey(base)
}

func WorkflowRetrievalStatePayload(state *core.Context, key string) map[string]any {
	return workflowutil.StatePayload(state, key)
}

func WorkflowRetrievalTaskPayload(task *core.Task, key string) map[string]any {
	return workflowutil.TaskPayload(task, key)
}

func TaskRetrievalPaths(task *core.Task) []string {
	return workflowutil.TaskPaths(task)
}

func hydrateWorkflowRetrieval(ctx context.Context, provider WorkflowRetrievalProvider, workflowID string, query WorkflowRetrievalQuery, limit, maxTokens int) (map[string]any, error) {
	return HydrateWorkflowRetrieval(ctx, provider, workflowID, query, limit, maxTokens)
}

func applyWorkflowRetrievalState(state *core.Context, key string, payload map[string]any) {
	ApplyWorkflowRetrievalState(state, key, payload)
}

func applyWorkflowRetrievalTask(task *core.Task, payload map[string]any) *core.Task {
	return ApplyWorkflowRetrievalTask(task, payload)
}

func taskRetrievalPaths(task *core.Task) []string {
	return TaskRetrievalPaths(task)
}

func buildWorkflowRetrievalQuery(q WorkflowRetrievalQuery) string {
	return workflowutil.BuildQuery(workflowutil.RetrievalQuery(q))
}
