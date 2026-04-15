package euclo

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestManifestExtensionParsing verifies YAML struct parsing round-trip
func TestManifestExtensionParsing(t *testing.T) {
	yamlContent := `
capability_keywords:
  "euclo:chat.ask":
    - clarify
    - elaborate
    - explain
  "euclo:chat.implement":
    - hack
    - quickfix
    - patch
`

	var ext EucloManifestExtension
	if err := yaml.Unmarshal([]byte(yamlContent), &ext); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Verify keywords were parsed
	if len(ext.CapabilityKeywords) != 2 {
		t.Errorf("expected 2 capability entries, got %d", len(ext.CapabilityKeywords))
	}

	askKeywords, ok := ext.CapabilityKeywords["euclo:chat.ask"]
	if !ok {
		t.Fatal("expected euclo:chat.ask keywords not found")
	}
	if len(askKeywords) != 3 {
		t.Errorf("expected 3 ask keywords, got %d", len(askKeywords))
	}

	// Verify specific keywords
	expectedAsk := []string{"clarify", "elaborate", "explain"}
	for i, kw := range expectedAsk {
		if i >= len(askKeywords) || askKeywords[i] != kw {
			t.Errorf("expected ask keyword %d to be %q, got %v", i, kw, askKeywords)
			break
		}
	}

	implementKeywords, ok := ext.CapabilityKeywords["euclo:chat.implement"]
	if !ok {
		t.Fatal("expected euclo:chat.implement keywords not found")
	}
	if len(implementKeywords) != 3 {
		t.Errorf("expected 3 implement keywords, got %d", len(implementKeywords))
	}

	// Test round-trip: marshal and unmarshal
	marshaled, err := yaml.Marshal(&ext)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	var ext2 EucloManifestExtension
	if err := yaml.Unmarshal(marshaled, &ext2); err != nil {
		t.Fatalf("failed to unmarshal marshaled YAML: %v", err)
	}

	if len(ext2.CapabilityKeywords) != len(ext.CapabilityKeywords) {
		t.Errorf("round-trip mismatch: expected %d keywords, got %d", len(ext.CapabilityKeywords), len(ext2.CapabilityKeywords))
	}
}

// TestManifestExtensionParsing_Empty verifies empty extension parsing
func TestManifestExtensionParsing_Empty(t *testing.T) {
	yamlContent := ``

	var ext EucloManifestExtension
	if err := yaml.Unmarshal([]byte(yamlContent), &ext); err != nil {
		t.Fatalf("failed to unmarshal empty YAML: %v", err)
	}

	if len(ext.CapabilityKeywords) != 0 {
		t.Errorf("expected empty keywords, got %d", len(ext.CapabilityKeywords))
	}
}

// TestCapabilityKeywordsFromManifest verifies the helper function
func TestCapabilityKeywordsFromManifest(t *testing.T) {
	tests := []struct {
		name     string
		input    *EucloManifestExtension
		expected map[string][]string
	}{
		{
			name:     "nil extension",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty keywords",
			input:    &EucloManifestExtension{},
			expected: nil,
		},
		{
			name: "with keywords",
			input: &EucloManifestExtension{
				CapabilityKeywords: map[string][]string{
					"euclo:chat.ask": {"clarify", "explain"},
				},
			},
			expected: map[string][]string{
				"euclo:chat.ask": {"clarify", "explain"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := capabilityKeywordsFromManifest(tc.input)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d entries, got %d", len(tc.expected), len(result))
				return
			}

			for capID, expectedWords := range tc.expected {
				words, ok := result[capID]
				if !ok {
					t.Errorf("expected capability %s not found", capID)
					continue
				}
				if len(words) != len(expectedWords) {
					t.Errorf("expected %d words for %s, got %d", len(expectedWords), capID, len(words))
				}
			}
		})
	}
}

// TestParseEucloExtension verifies the parseEucloExtension function
func TestParseEucloExtension(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
		check   func(*EucloManifestExtension) bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:  "already EucloManifestExtension",
			input: &EucloManifestExtension{CapabilityKeywords: map[string][]string{"test": {"word"}}},
			check: func(ext *EucloManifestExtension) bool {
				return len(ext.CapabilityKeywords) == 1
			},
		},
		{
			name: "valid map",
			input: map[string]any{
				"capability_keywords": map[string]any{
					"euclo:chat.ask": []any{"clarify", "explain"},
				},
			},
			check: func(ext *EucloManifestExtension) bool {
				words, ok := ext.CapabilityKeywords["euclo:chat.ask"]
				return ok && len(words) == 2 && words[0] == "clarify"
			},
		},
		{
			name:    "invalid type",
			input:   "string",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ext, err := parseEucloExtension(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.check != nil && !tc.check(ext) {
				t.Errorf("check failed for extension: %+v", ext)
			}
		})
	}
}
