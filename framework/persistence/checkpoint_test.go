package persistence

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"github.com/stretchr/testify/require"
)

type checkpointRepoStub struct {
	artifacts []agentlifecycle.WorkflowArtifactRecord
}

func (r *checkpointRepoStub) CreateWorkflow(context.Context, agentlifecycle.WorkflowRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetWorkflow(context.Context, string) (*agentlifecycle.WorkflowRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListWorkflows(context.Context) ([]agentlifecycle.WorkflowRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) CreateRun(context.Context, agentlifecycle.WorkflowRunRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetRun(context.Context, string) (*agentlifecycle.WorkflowRunRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListRuns(context.Context, string) ([]agentlifecycle.WorkflowRunRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) UpdateRunStatus(context.Context, string, string) error { return nil }
func (r *checkpointRepoStub) UpsertDelegation(context.Context, agentlifecycle.DelegationEntry) error {
	return nil
}
func (r *checkpointRepoStub) GetDelegation(context.Context, string) (*agentlifecycle.DelegationEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListDelegations(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListDelegationsByRun(context.Context, string) ([]agentlifecycle.DelegationEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) AppendDelegationTransition(context.Context, agentlifecycle.DelegationTransitionEntry) error {
	return nil
}
func (r *checkpointRepoStub) ListDelegationTransitions(context.Context, string) ([]agentlifecycle.DelegationTransitionEntry, error) {
	return nil, nil
}
func (r *checkpointRepoStub) AppendEvent(context.Context, agentlifecycle.WorkflowEventRecord) error {
	return nil
}
func (r *checkpointRepoStub) ListEvents(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListEventsByRun(context.Context, string, int) ([]agentlifecycle.WorkflowEventRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) UpsertArtifact(ctx context.Context, artifact agentlifecycle.WorkflowArtifactRecord) error {
	r.artifacts = append(r.artifacts, artifact)
	return nil
}
func (r *checkpointRepoStub) GetArtifact(context.Context, string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) ListArtifacts(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return append([]agentlifecycle.WorkflowArtifactRecord(nil), r.artifacts...), nil
}
func (r *checkpointRepoStub) ListArtifactsByRun(context.Context, string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	return append([]agentlifecycle.WorkflowArtifactRecord(nil), r.artifacts...), nil
}
func (r *checkpointRepoStub) UpsertLineageBinding(context.Context, agentlifecycle.LineageBindingRecord) error {
	return nil
}
func (r *checkpointRepoStub) GetLineageBinding(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByWorkflow(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByRun(context.Context, string) ([]agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByLineageID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) FindLineageBindingByAttemptID(context.Context, string) (*agentlifecycle.LineageBindingRecord, error) {
	return nil, nil
}
func (r *checkpointRepoStub) Close() error { return nil }

func TestSaveAndLoadCheckpointArtifact(t *testing.T) {
	repo := &checkpointRepoStub{}
	env := contextdata.NewEnvelope("task-1", "session-1")
	ref, err := SaveCheckpointArtifact(context.Background(), env, repo, CheckpointSnapshot{
		CheckpointID: "checkpoint-1",
		WorkflowID:   "workflow-1",
		RunID:        "run-1",
		Kind:         "checkpoint",
		Summary:      "checkpoint summary",
		Metadata:     map[string]any{"kind": "checkpoint"},
		InlineRaw:    `{"ok":true}`,
	})
	require.NoError(t, err)
	require.NotNil(t, ref)
	require.Equal(t, "checkpoint-1", ref.ArtifactID)

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), repo, "run-1", "checkpoint")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "checkpoint-1", loaded.ArtifactID)
	require.Equal(t, "checkpoint summary", loaded.SummaryText)
}

func TestLoadLatestCheckpointArtifactReturnsNewestMatch(t *testing.T) {
	repo := &checkpointRepoStub{
		artifacts: []agentlifecycle.WorkflowArtifactRecord{
			{
				ArtifactID:  "checkpoint-old",
				RunID:       "run-1",
				Kind:        "checkpoint",
				CreatedAt:   time.Date(2024, time.January, 1, 10, 0, 0, 0, time.UTC),
				SummaryText: "old",
			},
			{
				ArtifactID:  "checkpoint-new",
				RunID:       "run-1",
				Kind:        "checkpoint",
				CreatedAt:   time.Date(2024, time.January, 1, 11, 0, 0, 0, time.UTC),
				SummaryText: "new",
			},
			{
				ArtifactID:  "other-kind",
				RunID:       "run-1",
				Kind:        "summary",
				CreatedAt:   time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC),
				SummaryText: "ignored",
			},
		},
	}

	loaded, err := LoadLatestCheckpointArtifact(context.Background(), repo, "run-1", "checkpoint")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "checkpoint-new", loaded.ArtifactID)
}
