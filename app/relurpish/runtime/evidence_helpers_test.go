package runtime

import (
	"testing"
)

// TestHelperFunctions tests the helper functions.
func TestHelperFunctions(t *testing.T) {
	// Test extractString
	m := map[string]any{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	if extractString(m, "key1") != "value1" {
		t.Error("extractString should return 'value1' for key1")
	}
	if extractString(m, "nonexistent") != "" {
		t.Error("extractString should return empty string for nonexistent key")
	}
	if extractString(nil, "key1") != "" {
		t.Error("extractString should return empty string for nil map")
	}

	// Test extractInt64
	if extractInt64(m, "key2") != 123 {
		t.Error("extractInt64 should return 123 for key2")
	}
	if extractInt64(m, "nonexistent") != 0 {
		t.Error("extractInt64 should return 0 for nonexistent key")
	}
	if extractInt64(nil, "key2") != 0 {
		t.Error("extractInt64 should return 0 for nil map")
	}

	// Test extractStringSlice
	mSlice := map[string]any{
		"slice": []string{"a", "b", "c"},
	}
	slice := extractStringSlice(mSlice, "slice")
	if len(slice) != 3 || slice[0] != "a" {
		t.Error("extractStringSlice should return the string slice")
	}
	if len(extractStringSlice(mSlice, "nonexistent")) != 0 {
		t.Error("extractStringSlice should return empty slice for nonexistent key")
	}
	if len(extractStringSlice(nil, "slice")) != 0 {
		t.Error("extractStringSlice should return empty slice for nil map")
	}

	// Test extractBool
	if !extractBool(m, "key3") {
		t.Error("extractBool should return true for key3")
	}
	if extractBool(m, "nonexistent") {
		t.Error("extractBool should return false for nonexistent key")
	}
	if extractBool(nil, "key3") {
		t.Error("extractBool should return false for nil map")
	}

	// Test generateEvidenceID
	id1 := generateEvidenceID("test")
	id2 := generateEvidenceID("test")
	if id1 == "" || id2 == "" {
		t.Error("generateEvidenceID should not return empty string")
	}
	// They should be different (random component)
	// Note: In rare cases they could be the same, but very unlikely
	if id1 == id2 {
		t.Logf("Warning: generated duplicate IDs (possible but unlikely)")
	}

	// Test truncatePreview
	longStr := "a" + string(make([]byte, 300))
	truncated := truncatePreview(longStr, 100)
	if len(truncated) != 114 { // 100 + "...[truncated]" (14 chars)
		t.Errorf("Expected truncated length 114, got %d", len(truncated))
	}

	// Test that strings shorter than maxLen are not truncated
	shortStr := "short"
	notTruncated := truncatePreview(shortStr, 100)
	if notTruncated != shortStr {
		t.Errorf("Expected '%s', got '%s'", shortStr, notTruncated)
	}

	t.Logf("✓ Helper functions work")
}
