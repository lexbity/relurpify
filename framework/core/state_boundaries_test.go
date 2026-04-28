package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStateBoundaryPolicyRejectsInvalidMemoryClass(t *testing.T) {
	err := ValidateStateBoundaryPolicy(StateBoundaryPolicy{
		AllowedMemoryClasses: []MemoryClass{"invalid"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory class")
}

func TestLintStateMapFlagsOversizeRawPayloadAndTranscript(t *testing.T) {
	state := map[string]interface{}{
		"tool.raw_output": strings.Repeat("x", 1500),
		"react.history": []map[string]interface{}{
			{"role": "user", "content": "one"},
			{"role": "assistant", "content": "two"},
		},
	}
	policy := StateBoundaryPolicy{
		AllowedDataClasses:       []StateDataClass{StateDataClassStructuredState, StateDataClassArtifactRef},
		MaxStateEntryBytes:       512,
		MaxInlineCollectionItems: 1,
		PreferArtifactReferences: true,
	}

	violations := LintStateMap(state, policy)
	require.NotEmpty(t, violations)

	var codes []string
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	require.Contains(t, codes, "oversize_state_entry")
	require.Contains(t, codes, "prefer_artifact_reference")
	require.Contains(t, codes, "disallowed_data_class")
}

func TestLintStateMapAllowsArtifactAndMemoryReferences(t *testing.T) {
	state := map[string]interface{}{
		"artifact.last": ArtifactReference{
			ArtifactID: "artifact-1",
			URI:        "workflow://artifact/artifact-1",
		},
		"memory.fact": MemoryReference{
			MemoryClass: MemoryClassStreamed,
			RecordKey:   "fact-1",
			Store:       "sqlite",
		},
	}
	policy := StateBoundaryPolicy{
		AllowedMemoryClasses: []MemoryClass{MemoryClassStreamed},
		AllowedDataClasses:   []StateDataClass{StateDataClassArtifactRef, StateDataClassMemoryRef},
		MaxStateEntryBytes:   1024,
	}

	violations := LintStateMap(state, policy)
	require.Empty(t, violations)
}

func TestValidateStateBoundaryPolicyRejectsInvalidDataClassAndBounds(t *testing.T) {
	err := ValidateStateBoundaryPolicy(StateBoundaryPolicy{
		ReadKeys:                 []string{"", "task.*"},
		AllowedDataClasses:       []StateDataClass{"invalid"},
		MaxStateEntryBytes:       -1,
		MaxInlineCollectionItems: -1,
	})
	require.Error(t, err)

	err = ValidateStateBoundaryPolicy(StateBoundaryPolicy{
		WriteKeys:                []string{"  "},
		MaxStateEntryBytes:       1,
		MaxInlineCollectionItems: 1,
	})
	require.Error(t, err)

	err = ValidateStateBoundaryPolicy(StateBoundaryPolicy{
		AllowedDataClasses:       []StateDataClass{StateDataClassStructuredState},
		MaxStateEntryBytes:       -1,
		MaxInlineCollectionItems: 1,
	})
	require.Error(t, err)
}

func TestLintStateMapFlagsDisallowedMemoryClassAndOversizeCollection(t *testing.T) {
	state := map[string]interface{}{
		"memory.routine": MemoryReference{
			MemoryClass: MemoryClassWorking,
			RecordKey:   "routine-1",
		},
		"graph.items": []string{"a", "b", "c"},
	}
	policy := StateBoundaryPolicy{
		AllowedMemoryClasses:     []MemoryClass{MemoryClassStreamed},
		AllowedDataClasses:       []StateDataClass{StateDataClassMemoryRef, StateDataClassStructuredState},
		MaxInlineCollectionItems: 2,
	}

	violations := LintStateMap(state, policy)
	var codes []string
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	require.Contains(t, codes, "disallowed_memory_class")
	require.Contains(t, codes, "oversize_collection")
}

func TestLintStateMapFlagsHistoryAndRetrievalWhenNotAllowed(t *testing.T) {
	state := map[string]interface{}{
		"react.history": []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
		"graph.retrieval": []map[string]interface{}{
			{"record_key": "fact-1", "memory_class": "declarative"},
		},
	}

	violations := LintStateMap(state, StateBoundaryPolicy{
		AllowedDataClasses: []StateDataClass{StateDataClassTranscript, StateDataClassRetrievalDump},
	})

	var codes []string
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	require.Contains(t, codes, "history_access_disallowed")
	require.Contains(t, codes, "retrieval_access_disallowed")
}

func TestClassifierHelpersDetectStructuredShapes(t *testing.T) {
	require.True(t, looksLikeArtifactReference(map[string]interface{}{"artifact_id": "a-1"}))
	require.True(t, looksLikeMemoryReference(map[string]interface{}{"memory_class": "streamed", "record_key": "r-1"}))
	require.True(t, looksLikeTranscript([]map[string]interface{}{{"role": "assistant", "content": "done"}}))
	require.True(t, looksLikeRetrievalDump([]map[string]interface{}{{"record_key": "fact-1"}}))
	require.False(t, looksLikeArtifactReference(map[string]interface{}{"summary": "x"}))
	require.False(t, looksLikeMemoryReference("memory://fact-1"))
	require.False(t, looksLikeTranscript([]map[string]interface{}{{"summary": "x"}}))
	require.False(t, looksLikeRetrievalDump([]map[string]interface{}{{"summary": "x"}}))
}
