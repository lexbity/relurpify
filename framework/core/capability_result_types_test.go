package core

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"github.com/stretchr/testify/require"
)

func TestDefaultInsertionDecisionUsesTrustClass(t *testing.T) {
	direct := DefaultInsertionDecision(CapabilityDescriptor{TrustClass: TrustClassBuiltinTrusted}, ContentDispositionRaw)
	require.Equal(t, InsertionActionDirect, direct.Action)

	summarized := DefaultInsertionDecision(CapabilityDescriptor{TrustClass: TrustClassRemoteApproved}, ContentDispositionRaw)
	require.Equal(t, InsertionActionSummarized, summarized.Action)

	metadataOnly := DefaultInsertionDecision(CapabilityDescriptor{TrustClass: TrustClassRemoteDeclared}, ContentDispositionRaw)
	require.Equal(t, InsertionActionMetadataOnly, metadataOnly.Action)
}

func TestSummarizeCapabilityResultEnvelopePreservesProvenance(t *testing.T) {
	envelope := NewCapabilityResultEnvelope(
		CapabilityDescriptor{
			ID:         "tool:echo",
			Kind:       CapabilityKindTool,
			Name:       "echo",
			TrustClass: TrustClassBuiltinTrusted,
			Source: CapabilitySource{
				ProviderID: "builtin",
				Scope:      CapabilityScopeBuiltin,
			},
		},
		&ToolResult{
			Success: true,
			Data:    map[string]interface{}{"echo": "hello"},
		},
		ContentDispositionRaw,
		&PolicySnapshot{ID: "policy-1"},
		&ApprovalBinding{CapabilityID: "tool:echo"},
	)

	summary := SummarizeCapabilityResultEnvelope(envelope, "echo returned hello")
	require.NotNil(t, summary)
	require.Equal(t, envelope.Descriptor.ID, summary.Descriptor.ID)
	require.Equal(t, envelope.Provenance.CapabilityID, summary.Provenance.CapabilityID)
	require.Equal(t, ContentDispositionSummarized, summary.Provenance.Disposition)
	require.Equal(t, InsertionActionSummarized, summary.Insertion.Action)
	require.Equal(t, "echo returned hello", summary.Result.Data["summary"])
	require.NotEmpty(t, summary.BlockInsertions)
	require.Equal(t, "text", summary.BlockInsertions[0].ContentType)
}

func TestApprovalBindingPermissionMetadataIncludesScope(t *testing.T) {
	state := map[string]interface{}{
		"task.id":               "task-1",
		"architect.workflow_id": "wf-1",
	}

	binding := ApprovalBindingFromCapability(
		CapabilityDescriptor{
			ID:   "tool:file_read",
			Name: "file_read",
			Source: CapabilitySource{
				ProviderID: "builtin",
				SessionID:  "session-1",
			},
			EffectClasses: []EffectClass{EffectClassContextInsertion},
		},
		state,
		map[string]interface{}{"path": "README.md"},
	)
	require.NotNil(t, binding)
	metadata := binding.PermissionMetadata()
	require.Equal(t, "tool:file_read", metadata["capability_id"])
	require.Equal(t, "builtin", metadata["provider_id"])
	require.Equal(t, "session-1", metadata["session_id"])
	require.Equal(t, "README.md", metadata["target_resource"])
	require.Equal(t, "task-1", metadata["task_id"])
	require.Equal(t, "wf-1", metadata["workflow_id"])
}

func TestEffectiveInsertionDecisionAppliesMostRestrictivePolicy(t *testing.T) {
	spec := &agentspec.AgentRuntimeSpec{
		InsertionPolicies: []agentspec.CapabilityInsertionPolicy{
			{
				Selector: agentspec.CapabilitySelector{
					Kind:         CapabilityKindTool,
					TrustClasses: []TrustClass{TrustClassBuiltinTrusted},
				},
				Action: InsertionActionSummarized,
			},
			{
				Selector: agentspec.CapabilitySelector{
					Name: "echo",
				},
				Action: InsertionActionMetadataOnly,
			},
		},
	}
	envelope := NewCapabilityResultEnvelope(
		CapabilityDescriptor{
			ID:         "tool:echo",
			Kind:       CapabilityKindTool,
			Name:       "echo",
			TrustClass: TrustClassBuiltinTrusted,
		},
		&ToolResult{Success: true, Data: map[string]interface{}{"echo": "hello"}},
		ContentDispositionRaw,
		nil,
		nil,
	)

	decision := EffectiveInsertionDecision(spec, envelope)
	require.Equal(t, InsertionActionMetadataOnly, decision.Action)
	require.Contains(t, decision.Reason, "manifest insertion policy")
}

func TestApplyInsertionDecisionUpdatesEnvelopeAndBlocks(t *testing.T) {
	envelope := NewCapabilityResultEnvelope(
		CapabilityDescriptor{
			ID:         "tool:echo",
			Kind:       CapabilityKindTool,
			Name:       "echo",
			TrustClass: TrustClassBuiltinTrusted,
		},
		&ToolResult{Success: true, Data: map[string]interface{}{"summary": "hello"}},
		ContentDispositionRaw,
		&PolicySnapshot{ID: "policy-1"},
		nil,
	)

	ApplyInsertionDecision(envelope, InsertionDecision{
		Action: InsertionActionMetadataOnly,
		Reason: "test override",
	})

	require.Equal(t, InsertionActionMetadataOnly, envelope.Insertion.Action)
	require.Equal(t, "policy-1", envelope.Insertion.PolicySnapshotID)
	require.Len(t, envelope.BlockInsertions, 1)
	require.Equal(t, InsertionActionMetadataOnly, envelope.BlockInsertions[0].Decision.Action)
}

func TestResourceBlocksRemainMetadataOnlyEvenWhenEnvelopeAllowsDirectInsertion(t *testing.T) {
	envelope := &CapabilityResultEnvelope{
		Policy: &PolicySnapshot{ID: "policy-1"},
		ContentBlocks: []ContentBlock{
			TextContentBlock{Text: "safe"},
			ResourceLinkContentBlock{URI: "file:///tmp/demo"},
			BinaryReferenceContentBlock{Ref: "blob-1"},
		},
		Insertion: InsertionDecision{
			Action:           InsertionActionDirect,
			PolicySnapshotID: "policy-1",
		},
	}

	ApplyInsertionDecision(envelope, envelope.Insertion)

	require.Len(t, envelope.BlockInsertions, 3)
	require.Equal(t, InsertionActionDirect, envelope.BlockInsertions[0].Decision.Action)
	require.Equal(t, InsertionActionMetadataOnly, envelope.BlockInsertions[1].Decision.Action)
	require.Equal(t, InsertionActionMetadataOnly, envelope.BlockInsertions[2].Decision.Action)
	require.Equal(t, "policy-1", envelope.BlockInsertions[1].Decision.PolicySnapshotID)
}
