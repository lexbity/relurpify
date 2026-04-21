package agenttest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// ToolSequenceFingerprint captures the essential characteristics of tool execution
type ToolSequenceFingerprint struct {
	ToolOrder       []string          `json:"tool_order"`
	ToolArgsHash    map[string]string `json:"tool_args_hash"`
	ToolResultsHash map[string]string `json:"tool_results_hash"`
}

// ComputeFingerprint creates a deterministic hash of the tool transcript
func ComputeFingerprint(transcript *ToolTranscriptArtifact) (*ToolSequenceFingerprint, error) {
	if transcript == nil {
		return nil, fmt.Errorf("transcript is nil")
	}

	fp := &ToolSequenceFingerprint{
		ToolOrder:       make([]string, 0, len(transcript.Entries)),
		ToolArgsHash:    make(map[string]string),
		ToolResultsHash: make(map[string]string),
	}

	for _, entry := range transcript.Entries {
		// Record tool order
		fp.ToolOrder = append(fp.ToolOrder, entry.Tool)

		// Create index key for this entry
		key := fmt.Sprintf("%d:%s", entry.Index, entry.Tool)

		// Hash call metadata if present
		if len(entry.CallMetadata) > 0 {
			argsJSON, err := json.Marshal(entry.CallMetadata)
			if err == nil {
				hash := sha256.Sum256(argsJSON)
				fp.ToolArgsHash[key] = hex.EncodeToString(hash[:8])
			}
		}

		// Hash result metadata if present
		if len(entry.ResultMetadata) > 0 {
			resultJSON, err := json.Marshal(entry.ResultMetadata)
			if err == nil {
				hash := sha256.Sum256(resultJSON)
				fp.ToolResultsHash[key] = hex.EncodeToString(hash[:8])
			}
		}
	}

	return fp, nil
}

// FingerprintDistance compares two fingerprints (0.0=identical, 1.0=completely different)
func FingerprintDistance(a, b *ToolSequenceFingerprint) float64 {
	if a == nil && b == nil {
		return 0.0
	}
	if a == nil || b == nil {
		return 1.0
	}

	// Calculate distance based on tool order (Levenshtein-like)
	orderDistance := sequenceDistance(a.ToolOrder, b.ToolOrder)

	// Calculate args hash similarity
	argsDistance := mapDistance(a.ToolArgsHash, b.ToolArgsHash)

	// Calculate results hash similarity
	resultsDistance := mapDistance(a.ToolResultsHash, b.ToolResultsHash)

	// Weighted average: 60% order, 20% args, 20% results
	return 0.6*orderDistance + 0.2*argsDistance + 0.2*resultsDistance
}

// sequenceDistance calculates normalized edit distance between two string sequences
func sequenceDistance(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}

	// Simple Levenshtein distance
	la, lb := len(a), len(b)
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}

	// Create distance matrix
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			dp[i][j] = min3(dp[i-1][j]+1, dp[i][j-1]+1, dp[i-1][j-1]+cost)
		}
	}

	return float64(dp[la][lb]) / float64(maxLen)
}

// mapDistance calculates normalized distance between two string maps
func mapDistance(a, b map[string]string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 1.0
	}

	// Count differences - each key in the union counts
	unionKeys := make(map[string]bool)
	for k := range a {
		unionKeys[k] = true
	}
	for k := range b {
		unionKeys[k] = true
	}

	differences := 0
	for k := range unionKeys {
		va, aok := a[k]
		vb, bok := b[k]
		if !aok || !bok || va != vb {
			differences++
		}
	}

	unionLen := len(unionKeys)
	if unionLen == 0 {
		return 0.0
	}

	return float64(differences) / float64(unionLen)
}

// min3 returns the minimum of three integers
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ExtractLLMFingerprints captures LLM response fingerprints from events
func ExtractLLMFingerprints(events []core.Event) map[string]string {
	fingerprints := make(map[string]string)
	callCount := 0

	for _, ev := range events {
		if ev.Type != core.EventLLMResponse {
			continue
		}
		callCount++

		// Create fingerprint from response content
		content := ""
		if rawContent, ok := ev.Metadata["content"]; ok {
			content = fmt.Sprint(rawContent)
		}

		// Hash the content (first 1KB for efficiency)
		if len(content) > 1024 {
			content = content[:1024]
		}

		hash := sha256.Sum256([]byte(content))
		key := fmt.Sprintf("llm_call_%d", callCount)
		fingerprints[key] = hex.EncodeToString(hash[:8])
	}

	return fingerprints
}

// DeterminismScore calculates a score (0.0-1.0) from fingerprint distance
func DeterminismScore(distance float64) float64 {
	// Negative distances are invalid, return 0
	if distance < 0 {
		return 0.0
	}
	// Clamp distance to [0, 1] and convert to score
	if distance > 1 {
		return 0.0
	}
	return 1.0 - distance
}

// LoadGoldenFingerprint loads a fingerprint from a tape path
func LoadGoldenFingerprint(tapePath string) (*ToolSequenceFingerprint, error) {
	// Read the tape file to extract tool sequence
	// This is a simplified implementation
	tape, err := LoadTape(tapePath)
	if err != nil {
		return nil, err
	}

	// Build transcript from tape
	transcript := BuildTranscriptFromTape(tape)
	return ComputeFingerprint(transcript)
}

// LoadTape loads a tape file (placeholder - integrate with existing tape system)
func LoadTape(path string) ([]map[string]any, error) {
	// This should integrate with the existing tape loading mechanism
	// For now, return empty as the tape system is in platform/llm
	return nil, fmt.Errorf("tape loading not yet implemented")
}

// BuildTranscriptFromTape builds a transcript from tape data
func BuildTranscriptFromTape(tape []map[string]any) *ToolTranscriptArtifact {
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{},
	}

	for i, entry := range tape {
		tool, _ := entry["tool"].(string)
		if tool == "" {
			continue
		}

		// Build entry
		tte := ToolTranscriptEntry{
			Index: i,
			Tool:  tool,
		}

		// Extract call metadata
		if rawArgs, ok := entry["arguments"].(map[string]any); ok {
			tte.CallMetadata = rawArgs
		}

		// Try to extract result
		if rawResult, ok := entry["result"].(map[string]any); ok {
			tte.Success = false
			if s, ok := rawResult["success"].(bool); ok {
				tte.Success = s
			}
			if err, ok := rawResult["error"].(string); ok {
				tte.Error = err
			}
		}

		transcript.Entries = append(transcript.Entries, tte)
	}

	return transcript
}

// CheckStateKeyStability checks if specified state keys are stable across runs
func CheckStateKeyStability(snapshots []*core.ContextSnapshot, keys []string) []string {
	var failures []string

	if len(snapshots) < 2 || len(keys) == 0 {
		return nil
	}

	for _, key := range keys {
		// Get values for this key across all snapshots
		var firstValue string
		stable := true

		for i, snapshot := range snapshots {
			value := ""
			if snapshot != nil {
				if v, ok := snapshot.State[key]; ok {
					value = fmt.Sprint(v)
				}
			}

			if i == 0 {
				firstValue = value
			} else if value != firstValue {
				stable = false
				break
			}
		}

		if !stable {
			failures = append(failures, fmt.Sprintf("state key %s is not stable across runs", key))
		}
	}

	return failures
}
