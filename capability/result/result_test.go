package capresult

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func testDescriptor(id string, trust agentspec.TrustClass) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:         id,
		Kind:       agentspec.CapabilityKindTool,
		Name:       id + "-name",
		TrustClass: trust,
		Source: descriptor.CapabilitySource{
			ProviderID: "prov-1",
			Scope:      classification.CapabilityScopeProvider,
		},
	}
}

func TestContentBlockContentTypeValues(t *testing.T) {
	assert.Equal(t, "text", TextContentBlock{}.ContentType())
	assert.Equal(t, "structured", StructuredContentBlock{}.ContentType())
	assert.Equal(t, "resource-link", ResourceLinkContentBlock{}.ContentType())
	assert.Equal(t, "embedded-resource", EmbeddedResourceContentBlock{}.ContentType())
	assert.Equal(t, "binary-reference", BinaryReferenceContentBlock{}.ContentType())
	assert.Equal(t, "error", ErrorContentBlock{}.ContentType())
}

func TestApprovalBindingPermissionMetadataIncludesPopulatedFields(t *testing.T) {
	binding := ApprovalBinding{
		CapabilityID:   "cap-1",
		CapabilityName: "cap name",
		ProviderID:     "prov-1",
		SessionID:      "sess-1",
		EffectClasses:  []classification.EffectClass{classification.EffectClassFilesystemMutation, "", classification.EffectClassNetworkEgress},
		TargetResource: "/workspace/file.txt",
		TaskID:         "task-9",
		WorkflowID:     "wf-3",
	}

	metadata := binding.PermissionMetadata()

	assert.Equal(t, "cap-1", metadata["capability_id"])
	assert.Equal(t, "cap name", metadata["capability_name"])
	assert.Equal(t, "prov-1", metadata["provider_id"])
	assert.Equal(t, "sess-1", metadata["approval_session_id"])
	assert.Equal(t, "filesystem-mutation,network-egress", metadata["effect_classes"])
	assert.Equal(t, "/workspace/file.txt", metadata["target_resource"])
	assert.Equal(t, "task-9", metadata["task_id"])
	assert.Equal(t, "wf-3", metadata["workflow_id"])
}

func TestApprovalBindingPermissionMetadataOmitsEmptyFields(t *testing.T) {
	binding := ApprovalBinding{
		EffectClasses: []classification.EffectClass{"", ""},
	}

	metadata := binding.PermissionMetadata()

	assert.Equal(t, map[string]string{}, metadata)
	assert.NotContains(t, metadata, "effect_classes")
}

func TestNewCapabilityResultEnvelopeDerivesProvenanceAndInsertion(t *testing.T) {
	desc := testDescriptor("cap-1", agentspec.TrustClassBuiltinTrusted)
	snapshot := &PolicySnapshot{ID: "policy-7"}
	approval := &ApprovalBinding{CapabilityID: "cap-1"}
	result := &ports.ToolResult{Success: true, Data: map[string]any{"summary": "did the thing"}}

	envelope := NewCapabilityResultEnvelope(desc, result, ContentDispositionRaw, snapshot, approval)

	require.NotNil(t, envelope)
	assert.Equal(t, desc, envelope.Descriptor)
	assert.Same(t, result, envelope.Result)
	assert.Equal(t, "cap-1", envelope.Provenance.CapabilityID)
	assert.Equal(t, "prov-1", envelope.Provenance.ProviderID)
	assert.Equal(t, agentspec.TrustClassBuiltinTrusted, envelope.Provenance.TrustClass)
	assert.Equal(t, ContentDispositionRaw, envelope.Provenance.Disposition)
	assert.Equal(t, ContentDispositionRaw, envelope.Disposition)
	assert.False(t, envelope.RecordedAt.IsZero())
	assert.Equal(t, agentspec.InsertionActionDirect, envelope.Insertion.Action)
	assert.Equal(t, "policy-7", envelope.Insertion.PolicySnapshotID)
	assert.Same(t, approval, envelope.Approval)
	assert.Same(t, snapshot, envelope.Policy)

	// A summary-only result derives a single text block with inherited decision.
	require.Len(t, envelope.ContentBlocks, 1)
	text, ok := envelope.ContentBlocks[0].(TextContentBlock)
	require.True(t, ok)
	assert.Equal(t, "did the thing", text.Text)
	assert.Equal(t, envelope.Provenance, text.Provenance)
	require.Len(t, envelope.BlockInsertions, 1)
	assert.Equal(t, "text", envelope.BlockInsertions[0].ContentType)
	assert.Equal(t, agentspec.InsertionActionDirect, envelope.BlockInsertions[0].Decision.Action)
}

func TestNewCapabilityResultEnvelopeWithoutSnapshotLeavesPolicySnapshotIDEmpty(t *testing.T) {
	envelope := NewCapabilityResultEnvelope(testDescriptor("cap-1", agentspec.TrustClassBuiltinTrusted), nil, ContentDispositionRaw, nil, nil)

	require.NotNil(t, envelope)
	assert.Nil(t, envelope.Result)
	assert.Nil(t, envelope.ContentBlocks)
	assert.Nil(t, envelope.BlockInsertions)
	assert.Empty(t, envelope.Insertion.PolicySnapshotID)
	assert.Equal(t, agentspec.InsertionActionDirect, envelope.Insertion.Action)
}

func TestNewCapabilityResultEnvelopeWithBlocksUsesProvidedBlocks(t *testing.T) {
	provenance := ContentProvenance{CapabilityID: "cap-1"}
	blocks := []ContentBlock{
		TextContentBlock{Text: "hello", Provenance: provenance},
		BinaryReferenceContentBlock{Ref: "sha256:abc"},
	}

	envelope := NewCapabilityResultEnvelopeWithBlocks(
		testDescriptor("cap-1", agentspec.TrustClassBuiltinTrusted),
		&ports.ToolResult{Success: true, Data: map[string]any{"ignored": true}},
		ContentDispositionRaw, nil, nil, blocks,
	)

	require.Len(t, envelope.ContentBlocks, 2)
	assert.Equal(t, blocks, envelope.ContentBlocks)
	require.Len(t, envelope.BlockInsertions, 2)
	assert.Equal(t, "text", envelope.BlockInsertions[0].ContentType)
	assert.Equal(t, agentspec.InsertionActionDirect, envelope.BlockInsertions[0].Decision.Action)
	assert.Equal(t, "binary-reference", envelope.BlockInsertions[1].ContentType)
	assert.Equal(t, agentspec.InsertionActionMetadataOnly, envelope.BlockInsertions[1].Decision.Action,
		"binary references must downgrade direct insertion to metadata-only")
}

func TestApplyInsertionDecisionNilEnvelopeReturnsNil(t *testing.T) {
	assert.Nil(t, ApplyInsertionDecision(nil, InsertionDecision{Action: agentspec.InsertionActionDenied}))
}

func TestApplyInsertionDecisionInheritsSnapshotIDAndRecomputesHITL(t *testing.T) {
	envelope := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassBuiltinTrusted),
		&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
		ContentDispositionRaw,
		&PolicySnapshot{ID: "policy-7"}, nil,
	)

	updated := ApplyInsertionDecision(envelope, InsertionDecision{Action: agentspec.InsertionActionHITLRequired, RequiresHITL: false})

	assert.Same(t, envelope, updated)
	assert.Equal(t, agentspec.InsertionActionHITLRequired, envelope.Insertion.Action)
	assert.True(t, envelope.Insertion.RequiresHITL, "hitl-required action must force the HITL flag")
	assert.Equal(t, "policy-7", envelope.Insertion.PolicySnapshotID, "decision inherits the envelope policy snapshot id")
	require.Len(t, envelope.BlockInsertions, 1)
	assert.True(t, envelope.BlockInsertions[0].Decision.RequiresHITL)

	// An explicit HITL flag on a non-HITL action is recomputed, not trusted.
	ApplyInsertionDecision(envelope, InsertionDecision{Action: agentspec.InsertionActionDirect, RequiresHITL: true, PolicySnapshotID: "explicit"})
	assert.Equal(t, agentspec.InsertionActionDirect, envelope.Insertion.Action)
	assert.False(t, envelope.Insertion.RequiresHITL)
	assert.Equal(t, "explicit", envelope.Insertion.PolicySnapshotID, "explicit snapshot id is preserved")
}

func TestDefaultInsertionDecisionPerTrustClass(t *testing.T) {
	cases := []struct {
		name        string
		trust       agentspec.TrustClass
		wantAction  agentspec.InsertionAction
		wantKeyword string
	}{
		{"builtin trusted is direct", agentspec.TrustClassBuiltinTrusted, agentspec.InsertionActionDirect, "trusted"},
		{"workspace trusted is direct", agentspec.TrustClassWorkspaceTrusted, agentspec.InsertionActionDirect, "trusted"},
		{"llm generated is summarized", agentspec.TrustClassLLMGenerated, agentspec.InsertionActionSummarized, "summarized"},
		{"tool result is summarized", agentspec.TrustClassToolResult, agentspec.InsertionActionSummarized, "summarized"},
		{"remote approved is summarized", agentspec.TrustClassRemoteApproved, agentspec.InsertionActionSummarized, "summarized"},
		{"provider local untrusted is metadata-only", agentspec.TrustClassProviderLocalUntrusted, agentspec.InsertionActionMetadataOnly, "metadata-only"},
		{"remote declared is metadata-only", agentspec.TrustClassRemoteDeclared, agentspec.InsertionActionMetadataOnly, "metadata-only"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := DefaultInsertionDecision(testDescriptor("cap", tc.trust), ContentDispositionRaw)
			assert.Equal(t, tc.wantAction, decision.Action)
			assert.Contains(t, decision.Reason, tc.wantKeyword)
			assert.False(t, decision.RequiresHITL)
		})
	}
}

func TestDefaultInsertionDecisionUnknownTrustClassFallsBackToDisposition(t *testing.T) {
	desc := testDescriptor("cap", agentspec.TrustClass("mystery"))

	metadataOnly := DefaultInsertionDecision(desc, ContentDispositionMetadataOnly)
	assert.Equal(t, agentspec.InsertionActionMetadataOnly, metadataOnly.Action)
	assert.Contains(t, metadataOnly.Reason, "metadata-only content disposition")

	summarized := DefaultInsertionDecision(desc, ContentDispositionSummarized)
	assert.Equal(t, agentspec.InsertionActionSummarized, summarized.Action)
	assert.Contains(t, summarized.Reason, "summarized content disposition")

	fallback := DefaultInsertionDecision(desc, ContentDispositionRaw)
	assert.Equal(t, agentspec.InsertionActionSummarized, fallback.Action)
	assert.Contains(t, fallback.Reason, "by default")
}

func TestSummarizeCapabilityResultEnvelopeNilSourceReturnsNil(t *testing.T) {
	assert.Nil(t, SummarizeCapabilityResultEnvelope(nil, "summary"))
}

func TestSummarizeCapabilityResultEnvelopeStartsDerivationChain(t *testing.T) {
	source := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
		&ports.ToolResult{
			Success:  false,
			Error:    "boom",
			Data:     map[string]any{"raw": "payload"},
			Metadata: map[string]any{"k": "v"},
		},
		ContentDispositionRaw, nil, nil,
	)
	originalRecordedAt := source.RecordedAt

	summarized := SummarizeCapabilityResultEnvelope(source, "  condensed story  ")

	require.NotNil(t, summarized)
	assert.Equal(t, ContentDispositionSummarized, summarized.Disposition)
	assert.Equal(t, originalRecordedAt, summarized.RecordedAt, "recorded_at is carried over from the source envelope")
	assert.False(t, summarized.Result.Success, "success flag is copied from the source result")
	assert.Equal(t, "boom", summarized.Result.Error)
	assert.Equal(t, map[string]any{"summary": "condensed story"}, summarized.Result.Data)
	require.NotNil(t, summarized.Result.Metadata)
	assert.Equal(t, "v", summarized.Result.Metadata["k"])

	// Mutating the cloned metadata must not leak into the source result.
	summarized.Result.Metadata["k"] = "mutated"
	assert.Equal(t, "v", source.Result.Metadata["k"])

	require.NotNil(t, summarized.Provenance.Derivation)
	require.Len(t, summarized.Provenance.Derivation.Steps, 1)
	assert.Equal(t, "origin", string(summarized.Provenance.Derivation.Steps[0].Transform))
	assert.Equal(t, "capability_result", summarized.Provenance.Derivation.Steps[0].SourceSystem)
}

// NOTE: this test documents current behavior, which looks like a production
// bug (see report): SummarizeCapabilityResultEnvelope rebuilds the envelope
// from scratch and never copies source.Provenance.Derivation, so the
// "append summarize derivation step" branch in
// SummarizeCapabilityResultEnvelope is unreachable and every summarize
// restarts the derivation chain from a fresh origin instead of appending.
func TestSummarizeCapabilityResultEnvelopeRestartsDerivationChain(t *testing.T) {
	source := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
		&ports.ToolResult{Success: true, Data: map[string]any{"summary": "first"}},
		ContentDispositionRaw, nil, nil,
	)
	once := SummarizeCapabilityResultEnvelope(source, "first summary")
	require.NotNil(t, once)
	require.NotNil(t, once.Provenance.Derivation)
	require.Len(t, once.Provenance.Derivation.Steps, 1)

	twice := SummarizeCapabilityResultEnvelope(once, "second summary")
	require.NotNil(t, twice)
	require.NotNil(t, twice.Provenance.Derivation)
	require.Len(t, twice.Provenance.Derivation.Steps, 1,
		"summarize restarts the chain rather than appending (documented bug)")
	assert.Equal(t, "origin", string(twice.Provenance.Derivation.Steps[0].Transform))
	assert.Equal(t, "capability_result", twice.Provenance.Derivation.Steps[0].SourceSystem)
	assert.NotEqual(t, once.Provenance.Derivation.Steps[0].ID, twice.Provenance.Derivation.Steps[0].ID,
		"a fresh origin step is minted per summarize")
}

func TestSummarizeCapabilityResultEnvelopeCarriesApprovalAndPolicy(t *testing.T) {
	snapshot := &PolicySnapshot{ID: "policy-3"}
	approval := &ApprovalBinding{CapabilityID: "cap-1", TaskID: "task-1"}
	source := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
		&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
		ContentDispositionRaw, snapshot, approval,
	)

	summarized := SummarizeCapabilityResultEnvelope(source, "s")
	require.NotNil(t, summarized)
	assert.Same(t, snapshot, summarized.Policy)
	assert.Same(t, approval, summarized.Approval)
	assert.Equal(t, "policy-3", summarized.Insertion.PolicySnapshotID)
}

func TestSummarizeCapabilityResultEnvelopeMapsInsertionAction(t *testing.T) {
	cases := []struct {
		name             string
		sourceAction     agentspec.InsertionAction
		wantAction       agentspec.InsertionAction
		wantReason       string
		wantRequiresHITL bool
	}{
		{
			"direct downgrades to summarized",
			agentspec.InsertionActionDirect,
			agentspec.InsertionActionSummarized,
			"summarized insertion preserves provenance", false,
		},
		{
			"summarized stays",
			agentspec.InsertionActionSummarized,
			agentspec.InsertionActionSummarized, "", false,
		},
		{
			"metadata-only stays",
			agentspec.InsertionActionMetadataOnly,
			agentspec.InsertionActionMetadataOnly, "", false,
		},
		{
			"hitl-required stays and keeps the flag",
			agentspec.InsertionActionHITLRequired,
			agentspec.InsertionActionHITLRequired, "", true,
		},
		{
			"denied stays",
			agentspec.InsertionActionDenied,
			agentspec.InsertionActionDenied, "", false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := NewCapabilityResultEnvelope(
				testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
				&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
				ContentDispositionRaw, nil, nil,
			)
			require.NotNil(t, ApplyInsertionDecision(source, InsertionDecision{Action: tc.sourceAction}))

			summarized := SummarizeCapabilityResultEnvelope(source, "s")
			require.NotNil(t, summarized)
			assert.Equal(t, tc.wantAction, summarized.Insertion.Action)
			if tc.wantReason != "" {
				assert.Equal(t, tc.wantReason, summarized.Insertion.Reason)
			}
			assert.Equal(t, tc.wantRequiresHITL, summarized.Insertion.RequiresHITL)
		})
	}
}

func TestSummarizeCapabilityResultEnvelopeUnknownOrEmptyActionFallsBackToEnvelopeDefault(t *testing.T) {
	actions := []agentspec.InsertionAction{"", "totally-bogus"}
	for _, action := range actions {
		source := NewCapabilityResultEnvelope(
			testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
			&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
			ContentDispositionRaw, nil, nil,
		)
		if action != "" {
			require.NotNil(t, ApplyInsertionDecision(source, InsertionDecision{Action: action}))
		} else {
			// Force an empty action onto the source envelope.
			source.Insertion = InsertionDecision{}
		}

		summarized := SummarizeCapabilityResultEnvelope(source, "s")
		require.NotNil(t, summarized)
		assert.Equal(t, agentspec.InsertionActionSummarized, summarized.Insertion.Action,
			"unknown or empty action falls back to the freshly computed envelope decision")
		assert.NotEmpty(t, summarized.Insertion.Reason)
	}
}

func TestToolResultEnvelopeExtraction(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(nil)
		assert.False(t, ok)
		assert.Nil(t, envelope)
	})
	t.Run("nil metadata", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(&ports.ToolResult{Success: true})
		assert.False(t, ok)
		assert.Nil(t, envelope)
	})
	t.Run("missing key", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(&ports.ToolResult{Metadata: map[string]any{"other": 1}})
		assert.False(t, ok)
		assert.Nil(t, envelope)
	})
	t.Run("explicit nil value", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(&ports.ToolResult{Metadata: map[string]any{"capability_result_envelope": nil}})
		assert.False(t, ok)
		assert.Nil(t, envelope)
	})
	t.Run("wrong type", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(&ports.ToolResult{Metadata: map[string]any{"capability_result_envelope": "nope"}})
		assert.False(t, ok)
		assert.Nil(t, envelope)
	})
	t.Run("typed nil envelope", func(t *testing.T) {
		envelope, ok := ToolResultEnvelope(&ports.ToolResult{Metadata: map[string]any{
			"capability_result_envelope": (*CapabilityResultEnvelope)(nil),
		}})
		assert.False(t, ok, "a stored nil envelope must not be reported as found")
		assert.Nil(t, envelope)
	})
	t.Run("valid envelope", func(t *testing.T) {
		want := NewCapabilityResultEnvelope(testDescriptor("cap-1", agentspec.TrustClassToolResult), nil, ContentDispositionRaw, nil, nil)
		result := &ports.ToolResult{Metadata: map[string]any{"capability_result_envelope": want}}

		envelope, ok := ToolResultEnvelope(result)
		assert.True(t, ok)
		assert.Same(t, want, envelope)

		execEnvelope, execOK := CapabilityExecutionEnvelope(result)
		assert.True(t, execOK)
		assert.Same(t, want, execEnvelope)
	})
}

func TestCapabilityResultBlocksDerivation(t *testing.T) {
	provenance := ContentProvenance{CapabilityID: "cap-1"}

	t.Run("nil result yields no blocks", func(t *testing.T) {
		assert.Nil(t, CapabilityResultBlocks(nil, provenance))
	})

	t.Run("summary only yields text block", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{
			Success: true,
			Data:    map[string]any{"summary": "  all good  "},
		}, provenance)
		require.Len(t, blocks, 1)
		text, ok := blocks[0].(TextContentBlock)
		require.True(t, ok)
		assert.Equal(t, "  all good  ", text.Text, "the raw summary is stored untrimmed; only the emptiness check trims")
	})

	t.Run("whitespace summary is not treated as text", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{
			Success: true,
			Data:    map[string]any{"summary": "   "},
		}, provenance)
		require.Len(t, blocks, 1)
		_, ok := blocks[0].(StructuredContentBlock)
		assert.True(t, ok, "a whitespace-only summary must fall back to a structured block")
	})

	t.Run("multi key data yields structured block", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{
			Success: true,
			Data:    map[string]any{"summary": "s", "count": 2},
		}, provenance)
		require.Len(t, blocks, 1)
		structured, ok := blocks[0].(StructuredContentBlock)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"summary": "s", "count": 2}, structured.Data)
	})

	t.Run("error yields error block", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{Error: "  exploded  "}, provenance)
		require.Len(t, blocks, 1)
		errBlock, ok := blocks[0].(ErrorContentBlock)
		require.True(t, ok)
		assert.Equal(t, "exploded", errBlock.Message)
		assert.Equal(t, provenance, errBlock.Provenance)
	})

	t.Run("data and error yield both blocks", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{
			Data:  map[string]any{"result": 1},
			Error: "partial failure",
		}, provenance)
		require.Len(t, blocks, 2)
		_, isStructured := blocks[0].(StructuredContentBlock)
		assert.True(t, isStructured)
		_, isError := blocks[1].(ErrorContentBlock)
		assert.True(t, isError)
	})

	t.Run("empty data and empty error yield no blocks", func(t *testing.T) {
		blocks := CapabilityResultBlocks(&ports.ToolResult{Success: true, Data: map[string]any{}}, provenance)
		assert.Empty(t, blocks)
	})
}

func TestBuildContentBlockInsertionsResourceLikeBlocksForceMetadataOnly(t *testing.T) {
	decision := InsertionDecision{
		Action:           agentspec.InsertionActionDirect,
		Reason:           "trusted",
		PolicySnapshotID: "policy-1",
	}

	blocks := []ContentBlock{
		TextContentBlock{Text: "t"},
		ResourceLinkContentBlock{URI: "file:///tmp/x"},
		EmbeddedResourceContentBlock{Resource: "blob"},
		BinaryReferenceContentBlock{Ref: "sha256:1"},
		nil,
	}

	insertions := buildContentBlockInsertions(blocks, decision)

	require.Len(t, insertions, 4, "nil blocks are skipped")
	assert.Equal(t, agentspec.InsertionActionDirect, insertions[0].Decision.Action)
	for _, idx := range []int{1, 2, 3} {
		assert.Equal(t, agentspec.InsertionActionMetadataOnly, insertions[idx].Decision.Action,
			"resource-like block %d must downgrade to metadata-only", idx)
		assert.Equal(t, "policy-1", insertions[idx].Decision.PolicySnapshotID)
		assert.False(t, insertions[idx].Decision.RequiresHITL)
	}
}

func TestBuildContentBlockInsertionsEmptyInputs(t *testing.T) {
	assert.Nil(t, buildContentBlockInsertions(nil, InsertionDecision{Action: agentspec.InsertionActionDirect}))
	assert.Nil(t, buildContentBlockInsertions([]ContentBlock{nil}, InsertionDecision{Action: agentspec.InsertionActionDirect}))
}

func TestBuildContentBlockInsertionsKeepsMoreRestrictiveInheritedDecision(t *testing.T) {
	// A resource-like block's metadata-only default is LESS restrictive than
	// an inherited hitl-required decision, so the inherited decision wins.
	decision := InsertionDecision{Action: agentspec.InsertionActionHITLRequired, PolicySnapshotID: "policy-1"}

	insertions := buildContentBlockInsertions([]ContentBlock{BinaryReferenceContentBlock{Ref: "sha256:1"}}, decision)

	require.Len(t, insertions, 1)
	assert.Equal(t, agentspec.InsertionActionHITLRequired, insertions[0].Decision.Action)
	assert.Equal(t, "policy-1", insertions[0].Decision.PolicySnapshotID)
	// The base branch is returned verbatim (no RequiresHITL recompute on
	// this path, unlike ApplyInsertionDecision).
	assert.False(t, insertions[0].Decision.RequiresHITL)
}

func TestMoreRestrictiveInsertionDecisionNeverLoosensInheritedDecision(t *testing.T) {
	inherited := InsertionDecision{Action: agentspec.InsertionActionMetadataOnly, PolicySnapshotID: "policy-1"}

	blocks := []ContentBlock{TextContentBlock{Text: "t"}}
	insertions := buildContentBlockInsertions(blocks, inherited)
	require.Len(t, insertions, 1)
	assert.Equal(t, agentspec.InsertionActionMetadataOnly, insertions[0].Decision.Action,
		"a text block cannot be less restrictive than the inherited decision")
	assert.Equal(t, "policy-1", insertions[0].Decision.PolicySnapshotID)
}

func TestApprovalBindingFromCapabilityEmptyInputsReturnNil(t *testing.T) {
	assert.Nil(t, ApprovalBindingFromCapability(descriptor.CapabilityDescriptor{}, nil, nil))
	assert.Nil(t, ApprovalBindingFromCapability(descriptor.CapabilityDescriptor{}, map[string]any{}, map[string]any{}))
}

func TestApprovalBindingFromCapabilityExtractsStateAndArgs(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:   "cap-1",
		Name: "cap name",
		Source: descriptor.CapabilitySource{
			ProviderID: "prov-1",
			SessionID:  "sess-1",
		},
		EffectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
	}
	state := map[string]any{
		"task.id":               "  task-42  ",
		"architect.workflow_id": "wf-7",
	}
	args := map[string]any{"path": "/workspace/in.txt"}

	binding := ApprovalBindingFromCapability(desc, state, args)

	require.NotNil(t, binding)
	assert.Equal(t, "cap-1", binding.CapabilityID)
	assert.Equal(t, "cap name", binding.CapabilityName)
	assert.Equal(t, "prov-1", binding.ProviderID)
	assert.Equal(t, "sess-1", binding.SessionID)
	assert.Equal(t, "task-42", binding.TaskID, "state values are trimmed")
	assert.Equal(t, "wf-7", binding.WorkflowID)
	assert.Equal(t, "/workspace/in.txt", binding.TargetResource)
	assert.Equal(t, []classification.EffectClass{classification.EffectClassProcessSpawn}, binding.EffectClasses)
}

func TestApprovalBindingFromCapabilityWithoutEffectClassesLeavesSliceNil(t *testing.T) {
	// A descriptor with only identity fields (no provider/session/effects and
	// no state or args) produces no binding at all: identity alone is not a
	// binding-relevant element.
	assert.Nil(t, ApprovalBindingFromCapability(descriptor.CapabilityDescriptor{ID: "cap-1", Name: "n"}, nil, nil))

	desc := descriptor.CapabilityDescriptor{
		ID:     "cap-1",
		Name:   "n",
		Source: descriptor.CapabilitySource{ProviderID: "prov-1"},
	}

	binding := ApprovalBindingFromCapability(desc, nil, nil)
	require.NotNil(t, binding)
	assert.Nil(t, binding.EffectClasses)
	assert.Empty(t, binding.TargetResource)
}

func TestInferTargetResourceKeyPrecedence(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"empty args", map[string]any{}, ""},
		{"path wins over target", map[string]any{"path": "/a", "target": "/b"}, "/a"},
		{"target wins over resource", map[string]any{"target": "/b", "resource": "/c"}, "/b"},
		{"resource wins over uri", map[string]any{"resource": "/c", "uri": "u"}, "/c"},
		{"uri wins over url", map[string]any{"uri": "u", "url": "l"}, "u"},
		{"url wins over file", map[string]any{"url": "l", "file": "f"}, "l"},
		{"file wins over binary", map[string]any{"file": "f", "binary": "b"}, "f"},
		{"binary wins over database_path", map[string]any{"binary": "b", "database_path": "d"}, "b"},
		{"database_path wins over host", map[string]any{"database_path": "d", "host": "h"}, "d"},
		{"host used alone", map[string]any{"host": "db.internal"}, "db.internal"},
		{"whitespace value falls through to next key", map[string]any{"path": "   ", "target": " t2 "}, "t2"},
		{"non string values are stringified", map[string]any{"path": 42}, "42"},
		{"whitespace only args yield empty", map[string]any{"path": "  "}, ""},
		{"unrelated keys are ignored", map[string]any{"query": "x"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, inferTargetResource(tc.args))
		})
	}
}

func TestInsertionRestrictivenessOrdering(t *testing.T) {
	rank := func(action agentspec.InsertionAction) int {
		return InsertionRestrictiveness(action)
	}

	assert.Less(t, rank(agentspec.InsertionActionDirect), rank(agentspec.InsertionActionSummarized))
	assert.Less(t, rank(agentspec.InsertionActionSummarized), rank(agentspec.InsertionActionMetadataOnly))
	assert.Less(t, rank(agentspec.InsertionActionMetadataOnly), rank(agentspec.InsertionActionHITLRequired))
	assert.Less(t, rank(agentspec.InsertionActionHITLRequired), rank(agentspec.InsertionActionDenied))
	assert.Equal(t, rank(agentspec.InsertionActionDenied), rank(agentspec.InsertionAction("unknown")),
		"unknown actions must be treated as maximally restrictive")
}

func TestCapabilityResultEnvelopeJSONRoundTrip(t *testing.T) {
	envelope := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassBuiltinTrusted),
		&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
		ContentDispositionRaw,
		&PolicySnapshot{ID: "policy-1", CapturedAt: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)},
		&ApprovalBinding{CapabilityID: "cap-1"},
	)

	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "content_blocks", "in-memory content blocks are excluded from the wire format")
	assert.Contains(t, string(raw), `"policy_snapshot_id":"policy-1"`)

	var decoded CapabilityResultEnvelope
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "cap-1", decoded.Descriptor.ID)
	assert.Equal(t, ContentDispositionRaw, decoded.Disposition)
	assert.Equal(t, agentspec.InsertionActionDirect, decoded.Insertion.Action)
	assert.Equal(t, "policy-1", decoded.Insertion.PolicySnapshotID)
	assert.True(t, envelope.RecordedAt.Equal(decoded.RecordedAt))
	require.NotNil(t, decoded.Policy)
	assert.Equal(t, "policy-1", decoded.Policy.ID)
	require.NotNil(t, decoded.Approval)
	assert.Equal(t, "cap-1", decoded.Approval.CapabilityID)
}

func TestInsertionDecisionJSONOmitsEmptyOptionalFields(t *testing.T) {
	raw, err := json.Marshal(InsertionDecision{Action: agentspec.InsertionActionDenied})
	require.NoError(t, err)
	assert.Equal(t, `{"action":"denied"}`, string(raw))

	raw, err = json.Marshal(InsertionDecision{
		Action:           agentspec.InsertionActionHITLRequired,
		Reason:           "why",
		RequiresHITL:     true,
		PolicySnapshotID: "p",
	})
	require.NoError(t, err)
	assert.Equal(t, `{"action":"hitl-required","reason":"why","requires_hitl":true,"policy_snapshot_id":"p"}`, string(raw))
}

func TestSummarizeCapabilityResultEnvelopeTrimsSummary(t *testing.T) {
	source := NewCapabilityResultEnvelope(
		testDescriptor("cap-1", agentspec.TrustClassLLMGenerated),
		&ports.ToolResult{Success: true, Data: map[string]any{"summary": "s"}},
		ContentDispositionRaw, nil, nil,
	)

	summarized := SummarizeCapabilityResultEnvelope(source, "\n  padded summary \t")
	require.NotNil(t, summarized)
	assert.Equal(t, map[string]any{"summary": "padded summary"}, summarized.Result.Data,
		"the summary payload is trimmed before it is stored")
}
