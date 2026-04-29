package agenttest

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestComputeFingerprint(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", CallMetadata: map[string]any{"path": "test.go"}, ResultMetadata: map[string]any{"content": "data"}},
			{Index: 1, Tool: "go_test", CallMetadata: map[string]any{"pattern": "./..."}, ResultMetadata: map[string]any{"pass": true}},
		},
	}

	fp, err := ComputeFingerprint(transcript)
	if err != nil {
		t.Fatalf("ComputeFingerprint failed: %v", err)
	}

	if len(fp.ToolOrder) != 2 {
		t.Errorf("Expected 2 tools in order, got %d", len(fp.ToolOrder))
	}

	if fp.ToolOrder[0] != "file_read" || fp.ToolOrder[1] != "go_test" {
		t.Errorf("Unexpected tool order: %v", fp.ToolOrder)
	}

	// Check hashes are generated
	if len(fp.ToolArgsHash) != 2 {
		t.Errorf("Expected 2 arg hashes, got %d", len(fp.ToolArgsHash))
	}

	if len(fp.ToolResultsHash) != 2 {
		t.Errorf("Expected 2 result hashes, got %d", len(fp.ToolResultsHash))
	}
}

func TestComputeFingerprint_Nil(t *testing.T) {
	_, err := ComputeFingerprint(nil)
	if err == nil {
		t.Error("Expected error for nil transcript")
	}
}

func TestComputeFingerprint_Empty(t *testing.T) {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{},
	}

	fp, err := ComputeFingerprint(transcript)
	if err != nil {
		t.Fatalf("ComputeFingerprint failed: %v", err)
	}

	if len(fp.ToolOrder) != 0 {
		t.Errorf("Expected empty tool order, got %d", len(fp.ToolOrder))
	}
}

func TestFingerprintDistance_Identical(t *testing.T) {
	fp := &ToolSequenceFingerprint{
		ToolOrder: []string{"a", "b", "c"},
		ToolArgsHash: map[string]string{
			"0:a": "hash1",
			"1:b": "hash2",
		},
	}

	dist := FingerprintDistance(fp, fp)
	if dist != 0.0 {
		t.Errorf("Expected distance 0.0 for identical fingerprints, got %f", dist)
	}
}

func TestFingerprintDistance_CompletelyDifferent(t *testing.T) {
	a := &ToolSequenceFingerprint{
		ToolOrder:    []string{"a", "b"},
		ToolArgsHash: map[string]string{"0:a": "h1"},
	}
	b := &ToolSequenceFingerprint{
		ToolOrder:    []string{"x", "y", "z"},
		ToolArgsHash: map[string]string{"0:x": "h2"},
	}

	dist := FingerprintDistance(a, b)
	if dist < 0.5 || dist > 1.0 {
		t.Errorf("Expected high distance for different fingerprints, got %f", dist)
	}
}

func TestFingerprintDistance_NilHandling(t *testing.T) {
	a := &ToolSequenceFingerprint{ToolOrder: []string{"a"}}

	if FingerprintDistance(nil, nil) != 0.0 {
		t.Error("Expected 0.0 distance for both nil")
	}

	if FingerprintDistance(a, nil) != 1.0 {
		t.Error("Expected 1.0 distance when one is nil")
	}

	if FingerprintDistance(nil, a) != 1.0 {
		t.Error("Expected 1.0 distance when one is nil")
	}
}

func TestSequenceDistance(t *testing.T) {
	tests := []struct {
		a, b []string
		want float64
	}{
		{[]string{}, []string{}, 0.0},
		{[]string{"a"}, []string{"a"}, 0.0},
		{[]string{"a"}, []string{"b"}, 1.0},
		{[]string{"a", "b"}, []string{"a", "b"}, 0.0},
		{[]string{"a", "b"}, []string{}, 1.0},
	}

	for _, tt := range tests {
		got := sequenceDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("sequenceDistance(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMapDistance(t *testing.T) {
	tests := []struct {
		a, b map[string]string
		want float64
	}{
		{map[string]string{}, map[string]string{}, 0.0},
		{map[string]string{"k": "v"}, map[string]string{"k": "v"}, 0.0},
		{map[string]string{"k": "v1"}, map[string]string{"k": "v2"}, 1.0},
		{map[string]string{"k1": "v"}, map[string]string{"k2": "v"}, 1.0},
		{map[string]string{}, map[string]string{"k": "v"}, 1.0},
	}

	for _, tt := range tests {
		got := mapDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("mapDistance(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDeterminismScore(t *testing.T) {
	tests := []struct {
		distance float64
		want     float64
	}{
		{0.0, 1.0},
		{0.5, 0.5},
		{1.0, 0.0},
		{0.2, 0.8},
		{-0.1, 0.0},
		{1.1, 0.0},
	}

	for _, tt := range tests {
		got := DeterminismScore(tt.distance)
		if got != tt.want {
			t.Errorf("DeterminismScore(%f) = %f, want %f", tt.distance, got, tt.want)
		}
	}
}

func TestExtractLLMFingerprints(t *testing.T) {
	events := []core.Event{
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_read"}},
		{Type: core.EventLLMResponse, Metadata: map[string]any{"content": "response1"}},
		{Type: core.EventLLMResponse, Metadata: map[string]any{"content": "response2"}},
	}

	fingerprints := ExtractLLMFingerprints(events)

	if len(fingerprints) != 2 {
		t.Errorf("Expected 2 LLM fingerprints, got %d", len(fingerprints))
	}

	// Check that different content produces different fingerprints
	if fingerprints["llm_call_1"] == fingerprints["llm_call_2"] {
		t.Error("Expected different fingerprints for different content")
	}

	// Check that same content produces same fingerprint
	events2 := []core.Event{
		{Type: core.EventLLMResponse, Metadata: map[string]any{"content": "response1"}},
	}
	fingerprints2 := ExtractLLMFingerprints(events2)

	if fingerprints["llm_call_1"] != fingerprints2["llm_call_1"] {
		t.Error("Expected same fingerprint for same content")
	}
}

func TestCheckStateKeyStability(t *testing.T) {
	snapshots := []*contextdata.Envelope{
		{WorkingData: map[string]any{"key1": "value1", "key2": "value2"}},
		{WorkingData: map[string]any{"key1": "value1", "key2": "value2"}},
		{WorkingData: map[string]any{"key1": "value1", "key2": "value2"}},
	}

	// All keys stable
	failures := CheckStateKeyStability(snapshots, []string{"key1", "key2"})
	if len(failures) > 0 {
		t.Errorf("Expected no failures for stable keys, got: %v", failures)
	}

	// Unstable key
	snapshots[1].WorkingData["key2"] = "different"
	failures = CheckStateKeyStability(snapshots, []string{"key1", "key2"})
	if len(failures) != 1 {
		t.Errorf("Expected 1 failure for unstable key2, got %d", len(failures))
	}
}

func TestCheckStateKeyStability_EdgeCases(t *testing.T) {
	// Empty snapshots
	failures := CheckStateKeyStability([]*contextdata.Envelope{}, []string{"key"})
	if failures != nil {
		t.Error("Expected nil for empty snapshots")
	}

	// Single snapshot
	snapshots := []*contextdata.Envelope{{WorkingData: map[string]any{"key": "value"}}}
	failures = CheckStateKeyStability(snapshots, []string{"key"})
	if len(failures) > 0 {
		t.Error("Expected no failures for single snapshot")
	}

	// Empty keys
	failures = CheckStateKeyStability([]*contextdata.Envelope{{}}, []string{})
	if failures != nil {
		t.Error("Expected nil for empty keys")
	}
}

func TestBuildTranscriptFromTape(t *testing.T) {
	tape := []map[string]any{
		{
			"tool":      "file_read",
			"arguments": map[string]any{"path": "test.go"},
			"result": map[string]any{
				"success": true,
				"data":    map[string]any{"content": "hello"},
			},
		},
		{
			"tool": "file_write",
			"result": map[string]any{
				"success": false,
				"error":   "permission denied",
			},
		},
	}

	transcript := BuildTranscriptFromTape(tape)

	if len(transcript.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(transcript.Entries))
	}

	// Check first entry
	if transcript.Entries[0].Tool != "file_read" {
		t.Errorf("Expected tool file_read, got %s", transcript.Entries[0].Tool)
	}
	if transcript.Entries[0].CallMetadata["path"] != "test.go" {
		t.Error("Expected call metadata preserved")
	}
	if !transcript.Entries[0].Success {
		t.Error("Expected success true for first entry")
	}

	// Check second entry
	if transcript.Entries[1].Tool != "file_write" {
		t.Errorf("Expected tool file_write, got %s", transcript.Entries[1].Tool)
	}
	if transcript.Entries[1].Success {
		t.Error("Expected success false for second entry")
	}
	if transcript.Entries[1].Error != "permission denied" {
		t.Errorf("Expected error 'permission denied', got %s", transcript.Entries[1].Error)
	}
}

func TestBuildTranscriptFromTape_SkipsEmpty(t *testing.T) {
	tape := []map[string]any{
		{"not_tool": "data"},
		{"tool": ""},
		{"tool": "valid"},
	}

	transcript := BuildTranscriptFromTape(tape)

	if len(transcript.Entries) != 1 {
		t.Errorf("Expected 1 entry (only valid one), got %d", len(transcript.Entries))
	}
}

func TestMin3(t *testing.T) {
	tests := []struct {
		a, b, c, want int
	}{
		{1, 2, 3, 1},
		{3, 2, 1, 1},
		{2, 1, 3, 1},
		{1, 1, 1, 1},
	}

	for _, tt := range tests {
		got := min3(tt.a, tt.b, tt.c)
		if got != tt.want {
			t.Errorf("min3(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}
