package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflowResourceURITable(t *testing.T) {
	cases := []struct {
		name       string
		uri        string
		wantZero   bool
		workflowID string
		runID      string
		resourceID string
	}{
		{name: "full uri", uri: "wf-1/run-2/step-3/artifact-4", workflowID: "wf-1", runID: "run-2", resourceID: "artifact-4"},
		{name: "three segments", uri: "wf/run/res", workflowID: "wf", runID: "run", resourceID: "res"},
		{name: "two segments reuse last", uri: "wf/run", workflowID: "wf", runID: "run", resourceID: "run"},
		{name: "single segment yields zero uri", uri: "wf", wantZero: true},
		{name: "empty uri yields zero uri", uri: "", wantZero: true},
		{name: "empty segments are skipped", uri: "/wf//run/res/", workflowID: "wf", runID: "run", resourceID: "res"},
		{name: "only separators yields zero uri", uri: "///", wantZero: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseWorkflowResourceURI(tc.uri)
			require.NoError(t, err, "the parser never fails on malformed input today")
			if tc.wantZero {
				require.Equal(t, WorkflowResourceURI{}, parsed)
				return
			}
			require.Equal(t, tc.workflowID, parsed.WorkflowID)
			require.Equal(t, tc.runID, parsed.RunID)
			require.Equal(t, tc.resourceID, parsed.ResourceID)
		})
	}
}

func TestWorkflowResourceURIString(t *testing.T) {
	require.Equal(t, "wf/run/res", WorkflowResourceURI{
		WorkflowID: "wf", RunID: "run", ResourceID: "res",
	}.String())
	// Missing fields render as empty segments.
	require.Equal(t, "//", WorkflowResourceURI{}.String())
}

func TestDefaultWorkflowProjectionRefs(t *testing.T) {
	refs := DefaultWorkflowProjectionRefs("wf-1", "run-2", "step-3", WorkflowProjectionRoleReviewer)
	require.Equal(t, []string{"wf-1/run-2/context", "wf-1/run-2/state"}, refs)

	// The step and role do not influence the default ref set today.
	other := DefaultWorkflowProjectionRefs("wf-1", "run-2", "ignored", WorkflowProjectionRoleArchitect)
	require.Equal(t, refs, other)
}

func TestProjectionRolesHaveDistinctValues(t *testing.T) {
	roles := []WorkflowProjectionRole{
		WorkflowProjectionRoleArchitect,
		WorkflowProjectionRoleReviewer,
		WorkflowProjectionRoleVerifier,
		WorkflowProjectionRoleExecutor,
	}
	seen := make(map[WorkflowProjectionRole]struct{}, len(roles))
	for _, r := range roles {
		require.NotContains(t, seen, r, "role %q duplicated", r)
		seen[r] = struct{}{}
	}
}

func TestArtifactStorageKindsAreDistinct(t *testing.T) {
	require.NotEqual(t, ArtifactStorageInline, ArtifactStorageExternal)
	require.Equal(t, "inline", string(ArtifactStorageInline))
	require.Equal(t, "external", string(ArtifactStorageExternal))
}

func TestWorkflowRunStatusesAreDistinct(t *testing.T) {
	statuses := []WorkflowRunStatus{
		WorkflowRunStatusRunning,
		WorkflowRunStatusCompleted,
		WorkflowRunStatusFailed,
		WorkflowRunStatusCanceled,
		WorkflowRunStatusNeedsReplan,
	}
	seen := make(map[WorkflowRunStatus]struct{}, len(statuses))
	for _, s := range statuses {
		require.NotContains(t, seen, s, "status %q duplicated", s)
		seen[s] = struct{}{}
	}
}

func TestProjectRequiresStore(t *testing.T) {
	svc := &WorkflowProjectionService{}
	_, err := svc.Project(t.Context(), WorkflowResourceURI{WorkflowID: "wf"})
	require.EqualError(t, err, "workflow store is nil")
}

func TestProjectReturnsEmptyProjectionWithURIMetadata(t *testing.T) {
	// Project must not touch the store; any call into it fails the test.
	svc := &WorkflowProjectionService{Store: &stubWorkflowStateStore{}}
	uri := WorkflowResourceURI{WorkflowID: "wf-1", RunID: "run-2", ResourceID: "res-3"}

	result, err := svc.Project(t.Context(), uri)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Contents)
	require.Equal(t, "wf-1/run-2/res-3", result.Metadata["uri"])
}

// stubWorkflowStateStore satisfies WorkflowStateStore without performing any
// I/O; Project must never call into it.
type stubWorkflowStateStore struct{}

func (s *stubWorkflowStateStore) GetDelegation(context.Context, string) (*DelegationEntry, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) UpsertDelegation(context.Context, DelegationEntry) error {
	return errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) AppendDelegationTransition(context.Context, DelegationTransitionEntry) error {
	return errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) GetWorkflowArtifact(context.Context, string) (*WorkflowArtifactRecord, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) UpsertWorkflowArtifact(context.Context, WorkflowArtifactRecord) error {
	return errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) CreateWorkflow(context.Context, string) error {
	return errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) ListWorkflows(context.Context) ([]string, error) {
	return nil, errors.New("unexpected call")
}

func (s *stubWorkflowStateStore) Close() error {
	return errors.New("unexpected call")
}

// TestWorkflowStateStoreContractPinsRecordShapes keeps the persisted record
// fields that the authorization package relies on from drifting silently.
func TestWorkflowStateStoreContractPinsRecordShapes(t *testing.T) {
	entry := DelegationEntry{
		DelegationID:   "d-1",
		WorkflowID:     "wf-1",
		State:          "requested",
		TrustClass:     "standard",
		Recoverability: "retryable",
		Request:        policy.DelegationRequest{ID: "d-1", TaskType: "review"},
	}
	require.Equal(t, "d-1", entry.DelegationID)
	require.Equal(t, "review", entry.Request.TaskType)

	transition := DelegationTransitionEntry{TransitionID: "t-1", DelegationID: "d-1", ToState: "approved"}
	require.Equal(t, "approved", transition.ToState)

	artifact := WorkflowArtifactRecord{
		ArtifactID:   "a-1",
		StorageKind:  ArtifactStorageInline,
		RawSizeBytes: 12,
	}
	require.Equal(t, ArtifactStorageInline, artifact.StorageKind)
	require.EqualValues(t, 12, artifact.RawSizeBytes)

	run := WorkflowRunRecord{RunID: "r-1", Status: WorkflowRunStatusRunning}
	require.Equal(t, WorkflowRunStatusRunning, run.Status)
}

// FuzzParseWorkflowResourceURI exercises the URI parser against arbitrary
// untrusted strings: it must never panic and must slice the segments it
// consumed into the right fields.
func FuzzParseWorkflowResourceURI(f *testing.F) {
	f.Add("wf/run/res")
	f.Add("wf/run/step/res")
	f.Add("wf/run")
	f.Add("wf")
	f.Add("")
	f.Add("/wf//run/res/")
	f.Add("///")
	f.Add("a/b/c/d/e/f")

	f.Fuzz(func(t *testing.T, uri string) {
		parsed, err := ParseWorkflowResourceURI(uri)
		require.NoError(t, err, "parser returned an error for %q", uri)

		segments := strings.Split(uri, "/")
		clean := len(uri) > 0 && !strings.HasPrefix(uri, "/") && !strings.HasSuffix(uri, "/") &&
			len(segments) >= 2
		for _, seg := range segments {
			if seg == "" {
				clean = false
				break
			}
		}
		if !clean {
			return
		}

		require.Equal(t, segments[0], parsed.WorkflowID, "uri %q", uri)
		require.Equal(t, segments[1], parsed.RunID, "uri %q", uri)
		require.Equal(t, segments[len(segments)-1], parsed.ResourceID, "uri %q", uri)
	})
}
