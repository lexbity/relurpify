package memory

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWorkflowStateStore is an in-memory implementation for testing
type mockWorkflowStateStore struct {
	delegations   map[string]DelegationEntry
	transitions   map[string][]DelegationTransitionEntry
	artifacts     map[string]WorkflowArtifactRecord
}

func newMockWorkflowStateStore() *mockWorkflowStateStore {
	return &mockWorkflowStateStore{
		delegations: make(map[string]DelegationEntry),
		transitions: make(map[string][]DelegationTransitionEntry),
		artifacts:   make(map[string]WorkflowArtifactRecord),
	}
}

func (m *mockWorkflowStateStore) GetDelegation(ctx context.Context, delegationID string) (*DelegationEntry, error) {
	if entry, ok := m.delegations[delegationID]; ok {
		return &entry, nil
	}
	return nil, nil
}

func (m *mockWorkflowStateStore) UpsertDelegation(ctx context.Context, entry DelegationEntry) error {
	m.delegations[entry.DelegationID] = entry
	return nil
}

func (m *mockWorkflowStateStore) AppendDelegationTransition(ctx context.Context, transition DelegationTransitionEntry) error {
	m.transitions[transition.DelegationID] = append(m.transitions[transition.DelegationID], transition)
	return nil
}

func (m *mockWorkflowStateStore) GetWorkflowArtifact(ctx context.Context, artifactID string) (*WorkflowArtifactRecord, error) {
	if artifact, ok := m.artifacts[artifactID]; ok {
		return &artifact, nil
	}
	return nil, nil
}

func (m *mockWorkflowStateStore) UpsertWorkflowArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error {
	m.artifacts[artifact.ArtifactID] = artifact
	return nil
}

func TestWorkflowStateStoreInterface(t *testing.T) {
	// Verify mock implements the interface
	var store WorkflowStateStore = newMockWorkflowStateStore()
	require.NotNil(t, store)

	ctx := context.Background()

	// Test GetDelegation with empty store
	entry, err := store.GetDelegation(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, entry)
}

func TestDelegationEntry(t *testing.T) {
	now := time.Now().UTC()
	entry := DelegationEntry{
		DelegationID:   "delegation-1",
		WorkflowID:     "workflow-1",
		RunID:          "run-1",
		TaskID:         "task-1",
		State:          "pending",
		TrustClass:     "standard",
		Recoverability: "ephemeral",
		Background:     false,
		Request: core.DelegationRequest{
			ID:       "delegation-1",
			TaskID:   "task-1",
			Metadata: map[string]any{"key": "value"},
		},
		Result:    nil,
		Metadata:  map[string]any{"created_by": "test"},
		StartedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, "delegation-1", entry.DelegationID)
	assert.Equal(t, "workflow-1", entry.WorkflowID)
	assert.Equal(t, "pending", entry.State)
}

func TestDelegationTransitionEntry(t *testing.T) {
	now := time.Now().UTC()
	transition := DelegationTransitionEntry{
		TransitionID: "trans-1",
		DelegationID: "delegation-1",
		WorkflowID:   "workflow-1",
		RunID:        "run-1",
		ToState:      "completed",
		Metadata:     map[string]any{"reason": "success"},
		CreatedAt:    now,
	}

	assert.Equal(t, "trans-1", transition.TransitionID)
	assert.Equal(t, "completed", transition.ToState)
}

func TestWorkflowArtifactRecord(t *testing.T) {
	now := time.Now().UTC()
	artifact := WorkflowArtifactRecord{
		ArtifactID:        "artifact-1",
		WorkflowID:        "workflow-1",
		RunID:             "run-1",
		Kind:              "test-result",
		ContentType:       "application/json",
		StorageKind:       ArtifactStorageInline,
		SummaryText:       "Test passed",
		SummaryMetadata:   map[string]any{"duration_ms": 100},
		InlineRawText:     `{"status": "passed"}`,
		RawSizeBytes:      20,
		CompressionMethod: "",
		CreatedAt:         now,
	}

	assert.Equal(t, "artifact-1", artifact.ArtifactID)
	assert.Equal(t, ArtifactStorageInline, artifact.StorageKind)
}

func TestArtifactStorageKindConstants(t *testing.T) {
	assert.Equal(t, ArtifactStorageKind("inline"), ArtifactStorageInline)
	assert.Equal(t, ArtifactStorageKind("external"), ArtifactStorageExternal)
}

func TestWorkflowProjectionRoleConstants(t *testing.T) {
	assert.Equal(t, WorkflowProjectionRole("architect"), WorkflowProjectionRoleArchitect)
	assert.Equal(t, WorkflowProjectionRole("reviewer"), WorkflowProjectionRoleReviewer)
	assert.Equal(t, WorkflowProjectionRole("verifier"), WorkflowProjectionRoleVerifier)
	assert.Equal(t, WorkflowProjectionRole("executor"), WorkflowProjectionRoleExecutor)
}

func TestWorkflowResourceURI(t *testing.T) {
	uri := WorkflowResourceURI{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-1",
		Role:       WorkflowProjectionRoleArchitect,
		ResourceID: "resource-1",
	}

	str := uri.String()
	assert.Contains(t, str, "wf-1")
	assert.Contains(t, str, "run-1")
	assert.Contains(t, str, "resource-1")
}

func TestParseWorkflowResourceURI(t *testing.T) {
	uri, err := ParseWorkflowResourceURI("wf-1/run-1/resource-1")
	require.NoError(t, err)
	assert.Equal(t, "wf-1", uri.WorkflowID)
	assert.Equal(t, "run-1", uri.RunID)
	assert.Equal(t, "resource-1", uri.ResourceID)

	// Empty URI
	empty, err := ParseWorkflowResourceURI("")
	require.NoError(t, err)
	assert.Equal(t, "", empty.WorkflowID)
}

func TestDefaultWorkflowProjectionRefs(t *testing.T) {
	refs := DefaultWorkflowProjectionRefs("wf-1", "run-1", "step-1", WorkflowProjectionRoleArchitect)
	require.Len(t, refs, 2)
	assert.Contains(t, refs[0], "wf-1")
	assert.Contains(t, refs[0], "run-1")
	assert.Contains(t, refs[0], "context")
	assert.Contains(t, refs[1], "state")
}

func TestWorkflowProjectionService(t *testing.T) {
	mock := newMockWorkflowStateStore()
	service := WorkflowProjectionService{Store: mock}

	ctx := context.Background()
	uri := WorkflowResourceURI{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		ResourceID: "context",
	}

	result, err := service.Project(ctx, uri)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Contents)
	assert.NotNil(t, result.Metadata)
	assert.Contains(t, result.Metadata, "uri")

	// Test with nil store
	emptyService := WorkflowProjectionService{Store: nil}
	result, err = emptyService.Project(ctx, uri)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestWorkflowStateStoreWithMock(t *testing.T) {
	ctx := context.Background()
	store := newMockWorkflowStateStore()

	now := time.Now().UTC()

	// Create and upsert a delegation
	entry := DelegationEntry{
		DelegationID: "del-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		State:        "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}

	err := store.UpsertDelegation(ctx, entry)
	require.NoError(t, err)

	// Retrieve it back
	retrieved, err := store.GetDelegation(ctx, "del-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "active", retrieved.State)

	// Add a transition
	transition := DelegationTransitionEntry{
		TransitionID: "trans-1",
		DelegationID: "del-1",
		ToState:      "completed",
		CreatedAt:    now,
	}

	err = store.AppendDelegationTransition(ctx, transition)
	require.NoError(t, err)

	// Create an artifact
	artifact := WorkflowArtifactRecord{
		ArtifactID:   "art-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		Kind:         "result",
		ContentType:  "text/plain",
		StorageKind:  ArtifactStorageInline,
		CreatedAt:    now,
	}

	err = store.UpsertWorkflowArtifact(ctx, artifact)
	require.NoError(t, err)

	// Retrieve artifact
	retrievedArt, err := store.GetWorkflowArtifact(ctx, "art-1")
	require.NoError(t, err)
	require.NotNil(t, retrievedArt)
	assert.Equal(t, "result", retrievedArt.Kind)
}
