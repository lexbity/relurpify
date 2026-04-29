package agentlifecycle

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestDelegationEntryZeroValue(t *testing.T) {
	var entry DelegationEntry

	if entry.DelegationID != "" {
		t.Error("DelegationID should be empty for zero value")
	}
	if entry.State != "" {
		t.Error("State should be empty for zero value")
	}
	if entry.Request.TargetProviderID != "" {
		t.Error("Request.TargetProviderID should be empty for zero value")
	}
}

func TestWorkflowRecordZeroValue(t *testing.T) {
	var record WorkflowRecord

	if record.WorkflowID != "" {
		t.Error("WorkflowID should be empty for zero value")
	}
	if !record.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero for zero value")
	}
}

func TestWorkflowRunRecordZeroValue(t *testing.T) {
	var record WorkflowRunRecord

	if record.RunID != "" {
		t.Error("RunID should be empty for zero value")
	}
	if record.Status != "" {
		t.Error("Status should be empty for zero value")
	}
}

func TestLineageBindingRecordZeroValue(t *testing.T) {
	var record LineageBindingRecord

	if record.BindingID != "" {
		t.Error("BindingID should be empty for zero value")
	}
	if record.LineageID != "" {
		t.Error("LineageID should be empty for zero value")
	}
}

func TestArtifactStorageKindConstants(t *testing.T) {
	if ArtifactStorageInline == "" {
		t.Error("ArtifactStorageInline should not be empty")
	}
	if ArtifactStorageExternal == "" {
		t.Error("ArtifactStorageExternal should not be empty")
	}
}

func TestWorkflowProjectionRoleConstants(t *testing.T) {
	roles := []WorkflowProjectionRole{
		WorkflowProjectionRoleArchitect,
		WorkflowProjectionRoleReviewer,
		WorkflowProjectionRoleVerifier,
		WorkflowProjectionRoleExecutor,
	}

	for _, role := range roles {
		if role == "" {
			t.Errorf("WorkflowProjectionRole constant should not be empty: %v", role)
		}
	}
}

func TestDelegationEntryFields(t *testing.T) {
	now := time.Now().UTC()
	entry := DelegationEntry{
		DelegationID:   "del-123",
		WorkflowID:     "wf-456",
		RunID:          "run-789",
		TaskID:         "task-abc",
		State:          "active",
		TrustClass:     "trusted",
		Recoverability: "recoverable",
		Background:     true,
		Request: core.DelegationRequest{
			TargetProviderID: "provider-xyz",
		},
		StartedAt: now,
		UpdatedAt: now,
	}

	if entry.DelegationID != "del-123" {
		t.Errorf("DelegationID = %v, want del-123", entry.DelegationID)
	}
	if entry.WorkflowID != "wf-456" {
		t.Errorf("WorkflowID = %v, want wf-456", entry.WorkflowID)
	}
	if entry.State != "active" {
		t.Errorf("State = %v, want active", entry.State)
	}
	if !entry.Background {
		t.Error("Background should be true")
	}
	if entry.Request.TargetProviderID != "provider-xyz" {
		t.Errorf("Request.TargetProviderID = %v, want provider-xyz", entry.Request.TargetProviderID)
	}
}
