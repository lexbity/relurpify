package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStateBoundaryPolicyAcceptsCoherentPolicy(t *testing.T) {
	policy := StateBoundaryPolicy{
		ReadKeys:                 []string{"task.input", "step.result"},
		WriteKeys:                []string{"routing.next"},
		AllowHistoryAccess:       true,
		AllowRetrieval:           true,
		AllowedMemoryClasses:     []MemoryClass{MemoryClassWorking, MemoryClassStreamed},
		AllowedDataClasses:       []StateDataClass{StateDataClassTaskMetadata, StateDataClassRetrievalDump, StateDataClassRawPayload, StateDataClassSubagentHistory},
		MaxStateEntryBytes:       4096,
		MaxInlineCollectionItems: 32,
		PreferArtifactReferences: true,
	}
	require.NoError(t, ValidateStateBoundaryPolicy(policy))
	require.NoError(t, ValidateStateBoundaryPolicy(StateBoundaryPolicy{}), "zero policy is coherent")
}

func TestValidateStateBoundaryPolicyRejectsIncoherentPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  StateBoundaryPolicy
		wantErr string
	}{
		{
			name:    "empty read key",
			policy:  StateBoundaryPolicy{ReadKeys: []string{"ok", "   "}},
			wantErr: "state boundary contains empty key",
		},
		{
			name:    "empty write key",
			policy:  StateBoundaryPolicy{WriteKeys: []string{""}},
			wantErr: "state boundary contains empty key",
		},
		{
			name:    "unknown memory class",
			policy:  StateBoundaryPolicy{AllowedMemoryClasses: []MemoryClass{"episodic"}},
			wantErr: "memory class episodic invalid",
		},
		{
			name:    "unknown data class",
			policy:  StateBoundaryPolicy{AllowedDataClasses: []StateDataClass{"browser-history"}},
			wantErr: "state data class browser-history invalid",
		},
		{
			name:    "negative max state entry bytes",
			policy:  StateBoundaryPolicy{MaxStateEntryBytes: -1},
			wantErr: "max_state_entry_bytes must be >= 0",
		},
		{
			name:    "negative max inline collection items",
			policy:  StateBoundaryPolicy{MaxInlineCollectionItems: -3},
			wantErr: "max_inline_collection_items must be >= 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStateBoundaryPolicy(tt.policy)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLintStateMapReturnsNilForEmptyState(t *testing.T) {
	require.Nil(t, LintStateMap(map[string]any{}, StateBoundaryPolicy{MaxStateEntryBytes: 1}))
	require.Nil(t, LintStateMap(nil, StateBoundaryPolicy{}))
}

func violationsByCode(violations []StateBoundaryViolation) map[string][]StateBoundaryViolation {
	byCode := map[string][]StateBoundaryViolation{}
	for _, v := range violations {
		byCode[v.Code] = append(byCode[v.Code], v)
	}
	return byCode
}

func TestLintStateMapFlagsDisallowedDataClass(t *testing.T) {
	policy := StateBoundaryPolicy{AllowedDataClasses: []StateDataClass{StateDataClassTaskMetadata}}
	state := map[string]any{
		"task_name": "build",        // task-metadata: allowed
		"scratch":   "short string", // structured-state: disallowed
	}

	violations := LintStateMap(state, policy)
	byCode := violationsByCode(violations)
	require.Len(t, byCode["disallowed_data_class"], 1)
	v := byCode["disallowed_data_class"][0]
	require.Equal(t, "scratch", v.Key)
	require.Equal(t, StateDataClassStructuredState, v.DataClass)
	require.Contains(t, v.Message, "scratch")
	require.Contains(t, v.Message, string(StateDataClassStructuredState))
	require.Positive(t, v.SizeBytes)
	// Violations are reported in sorted key order.
	require.Equal(t, []string{"scratch"}, []string{v.Key})
}

func TestLintStateMapFlagsHistoryAndRetrievalWhenDisallowed(t *testing.T) {
	state := map[string]any{
		"chat_transcript":   []map[string]any{{"role": "user", "content": "hi"}},
		"retrieval_results": []map[string]any{{"record_key": "k1"}},
	}

	denying := LintStateMap(state, StateBoundaryPolicy{})
	byCode := violationsByCode(denying)
	require.Len(t, byCode["history_access_disallowed"], 1)
	require.Equal(t, "chat_transcript", byCode["history_access_disallowed"][0].Key)
	require.Equal(t, StateDataClassTranscript, byCode["history_access_disallowed"][0].DataClass)
	require.Len(t, byCode["retrieval_access_disallowed"], 1)
	require.Equal(t, "retrieval_results", byCode["retrieval_access_disallowed"][0].Key)

	allowing := LintStateMap(map[string]any{
		"chat_transcript":   state["chat_transcript"],
		"retrieval_results": state["retrieval_results"],
	}, StateBoundaryPolicy{AllowHistoryAccess: true, AllowRetrieval: true})
	require.Empty(t, allowing)
}

func TestLintStateMapFlagsOversizeEntryAndCollection(t *testing.T) {
	policy := StateBoundaryPolicy{
		MaxStateEntryBytes:       20,
		MaxInlineCollectionItems: 2,
	}
	state := map[string]any{
		"blob":     strings.Repeat("x", 64),
		"items":    []string{"a", "b", "c", "d"},
		"small":    "ok",
		"pairlist": []any{"only", "two"},
	}

	byCode := violationsByCode(LintStateMap(state, policy))
	require.Len(t, byCode["oversize_state_entry"], 1)
	require.Equal(t, "blob", byCode["oversize_state_entry"][0].Key)
	require.Len(t, byCode["oversize_collection"], 1)
	require.Equal(t, "items", byCode["oversize_collection"][0].Key)
	require.Contains(t, byCode["oversize_collection"][0].Message, "4 inline items")
}

func TestLintStateMapPrefersArtifactReferencesWhenDeclared(t *testing.T) {
	policy := StateBoundaryPolicy{PreferArtifactReferences: true}
	state := map[string]any{
		"big_blob":    strings.Repeat("y", 600), // classified raw-payload
		"output_ref":  ArtifactReference{ArtifactID: "a1", URI: "workflow://artifact/a1"},
		"pointer_ref": "workflow://artifact/a1",
	}

	byCode := violationsByCode(LintStateMap(state, policy))
	require.Len(t, byCode["prefer_artifact_reference"], 1)
	require.Equal(t, "big_blob", byCode["prefer_artifact_reference"][0].Key)
}

func TestLintStateMapFlagsDisallowedMemoryClass(t *testing.T) {
	policy := StateBoundaryPolicy{AllowedMemoryClasses: []MemoryClass{MemoryClassWorking}}
	state := map[string]any{
		"notes": MemoryReference{MemoryClass: MemoryClassStreamed, RecordKey: "n1"},
		"cache": MemoryReference{MemoryClass: MemoryClassWorking, RecordKey: "c1"},
	}

	byCode := violationsByCode(LintStateMap(state, policy))
	require.Len(t, byCode["disallowed_memory_class"], 1)
	v := byCode["disallowed_memory_class"][0]
	require.Equal(t, "notes", v.Key)
	require.Equal(t, StateDataClassMemoryRef, v.DataClass)
	require.Contains(t, v.Message, string(MemoryClassStreamed))
}

func TestClassifyStateValueTable(t *testing.T) {
	longString := strings.Repeat("z", 600)
	tests := []struct {
		name  string
		key   string
		value any
		want  StateDataClass
	}{
		{"key containing task wins first", "task_history", []map[string]any{{"role": "u", "content": "c"}}, StateDataClassTaskMetadata},
		{"key containing step", "step_result", map[string]any{}, StateDataClassStepMetadata},
		{"key containing next", "next_action", "x", StateDataClassRoutingFlag},
		{"key containing route", "route_hint", "x", StateDataClassRoutingFlag},
		{"key containing flag", "user_flag", "x", StateDataClassRoutingFlag},
		{"key containing history", "chat_history", "short", StateDataClassTranscript},
		{"key containing transcript", "transcript_tail", "short", StateDataClassTranscript},
		{"key containing retriev", "retrieval_dump", "short", StateDataClassRetrievalDump},
		{"artifact reference struct", "ref", ArtifactReference{ArtifactID: "a"}, StateDataClassArtifactRef},
		{"artifact reference pointer", "ref", &ArtifactReference{ArtifactID: "a"}, StateDataClassArtifactRef},
		{"memory reference struct", "mem", MemoryReference{RecordKey: "k"}, StateDataClassMemoryRef},
		{"memory reference pointer", "mem", &MemoryReference{RecordKey: "k"}, StateDataClassMemoryRef},
		{"memory envelope slice", "mem", []MemoryRecordEnvelope{{Key: "k"}}, StateDataClassRetrievalDump},
		{"artifact URI string", "s", "workflow://artifact/abc", StateDataClassArtifactRef},
		{"memory URI string", "s", "memory://scope/key", StateDataClassMemoryRef},
		{"long plain string", "s", longString, StateDataClassRawPayload},
		{"artifact-like map", "m", map[string]any{"artifact_id": "a"}, StateDataClassArtifactRef},
		{"raw_ref map", "m", map[string]any{"raw_ref": "r"}, StateDataClassArtifactRef},
		{"uri map", "m", map[string]any{"uri": "u"}, StateDataClassArtifactRef},
		{"memory-like map", "m", map[string]any{"memory_class": "working"}, StateDataClassMemoryRef},
		{"record_key map", "m", map[string]any{"record_key": "k"}, StateDataClassMemoryRef},
		{"transcript slice", "m", []map[string]any{{"role": "u", "content": "c"}}, StateDataClassTranscript},
		{"retrieval slice via record_key", "m", []map[string]any{{"record_key": "k"}}, StateDataClassRetrievalDump},
		{"retrieval slice via memory_class", "m", []map[string]any{{"memory_class": "working"}}, StateDataClassRetrievalDump},
		{"oversize generic value", "m", map[string]any{"fill": strings.Repeat("f", 1200)}, StateDataClassRawPayload},
		{"nil value", "m", nil, StateDataClassStructuredState},
		{"plain short string", "s", "hello", StateDataClassStructuredState},
		{"plain int", "n", 42, StateDataClassStructuredState},
		{"empty transcript slice", "m", []map[string]any{}, StateDataClassStructuredState},
		{"empty retrieval slice", "m", []map[string]any{}, StateDataClassStructuredState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyStateValue(tt.key, tt.value))
		})
	}
}

func TestEstimateStateValueBytes(t *testing.T) {
	require.Zero(t, estimateStateValueBytes(nil))
	require.Equal(t, len(`"abc"`), estimateStateValueBytes("abc"))

	// Values JSON cannot marshal fall back to fmt.Sprint.
	ch := make(chan int)
	require.Equal(t, len(fmt.Sprint(ch)), estimateStateValueBytes(ch))
}

func TestInlineCollectionItems(t *testing.T) {
	require.Zero(t, inlineCollectionItems("not a collection"))
	require.Zero(t, inlineCollectionItems(map[string]any{}))
	require.Equal(t, 3, inlineCollectionItems([]any{1, 2, 3}))
	require.Equal(t, 2, inlineCollectionItems([]string{"a", "b"}))
	require.Equal(t, 1, inlineCollectionItems([]map[string]any{{"k": "v"}}))
	require.Equal(t, 4, inlineCollectionItems([]MemoryRecordEnvelope{{}, {}, {}, {}}))
}

// FuzzLintStateMap exercises the lint/classification pipeline against
// arbitrary (untrusted) state keys and JSON-encoded payloads: it must never
// panic and any reported violation must reference a key present in the input.
func FuzzLintStateMap(f *testing.F) {
	f.Add("task_name", []byte(`"build"`), 0, 0)
	f.Add("history", []byte(`["x","y","z"]`), 0, 10)
	f.Add("retrieval_results", []byte(`[{"record_key":"k1"}]`), 0, 0)
	f.Add("blob", []byte(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`), 8, 0)
	f.Add("mem", []byte(`{"memory_class":"streamed","record_key":"n"}`), 0, 0)
	f.Add("weird key with spaces", []byte(`null`), -5, -5)
	f.Add("payload", []byte(`{"artifact_id":"a","uri":"workflow://artifact/a"}`), 100, 3)

	f.Fuzz(func(t *testing.T, key string, payload []byte, maxBytes, maxItems int) {
		var value any
		if err := json.Unmarshal(payload, &value); err != nil {
			t.Skip()
		}
		policy := StateBoundaryPolicy{
			MaxStateEntryBytes:       maxBytes,
			MaxInlineCollectionItems: maxItems,
		}
		violations := LintStateMap(map[string]any{key: value}, policy)
		for _, v := range violations {
			require.Equal(t, key, v.Key, "violation must reference the linted key")
			require.NotEmpty(t, v.Code)
			require.NotEmpty(t, v.Message)
			require.GreaterOrEqual(t, v.SizeBytes, 0)
		}
	})
}
