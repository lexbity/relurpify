package ports

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// DerivationID is a content-addressed identifier for a derivation step.
type DerivationID string

// DerivationStep records a single transformation in a content's lifecycle.
type DerivationStep struct {
	ID            DerivationID `json:"id"`
	ParentID      DerivationID `json:"parent_id,omitempty"`
	Transform     string       `json:"transform"`
	LossMagnitude float64      `json:"loss_magnitude"`
	SourceSystem  string       `json:"source_system"`
	Timestamp     time.Time    `json:"timestamp"`
	Detail        string       `json:"detail,omitempty"`
}

// DerivationChain is an ordered sequence of derivation steps from origin to current.
type DerivationChain struct {
	Steps []DerivationStep `json:"steps,omitempty"`
}

// Derive appends a new step to the chain and returns the updated chain.
func (c DerivationChain) Derive(transform, sourceSystem string, lossMagnitude float64, detail string) DerivationChain {
	if lossMagnitude < 0 {
		lossMagnitude = 0
	}
	if lossMagnitude > 1 {
		lossMagnitude = 1
	}
	timestamp := time.Now().UTC()
	var parentID DerivationID
	if len(c.Steps) > 0 {
		parentID = c.Steps[len(c.Steps)-1].ID
	}
	stepID := generateDerivationID(parentID, transform, timestamp)
	newStep := DerivationStep{
		ID:            stepID,
		ParentID:      parentID,
		Transform:     transform,
		LossMagnitude: lossMagnitude,
		SourceSystem:  sourceSystem,
		Timestamp:     timestamp,
		Detail:        detail,
	}
	return DerivationChain{
		Steps: append(c.Steps, newStep),
	}
}

// OriginDerivation creates a new chain with a single origin step.
func OriginDerivation(sourceSystem string) DerivationChain {
	timestamp := time.Now().UTC()
	stepID := generateDerivationID("", "origin", timestamp)
	return DerivationChain{
		Steps: []DerivationStep{
			{
				ID:            stepID,
				ParentID:      "",
				Transform:     "origin",
				LossMagnitude: 0.0,
				SourceSystem:  sourceSystem,
				Timestamp:     timestamp,
				Detail:        "",
			},
		},
	}
}

func generateDerivationID(parentID DerivationID, transform string, timestamp time.Time) DerivationID {
	hash := sha256.New()
	hash.Write([]byte(string(parentID) + ":" + transform + ":" + fmt.Sprintf("%d", timestamp.UnixNano())))
	hexStr := hex.EncodeToString(hash.Sum(nil))
	return DerivationID(hexStr[:16])
}

// MatchGlob checks whether a path matches a glob pattern using simple prefix/suffix matching.
// This is a minimal implementation for write path precheck; for full glob matching,
// use the search package's MatchGlob.
func MatchGlob(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" || pattern == "**" {
		return true
	}
	// Simple glob: split on * and match segments
	parts := splitGlob(pattern)
	if len(parts) == 1 {
		return path == parts[0]
	}
	// Prefix match before first *
	if !hasPrefixFold(path, parts[0]) {
		return false
	}
	path = path[len(parts[0]):]
	// Suffix match after last *
	last := parts[len(parts)-1]
	parts = parts[1 : len(parts)-1]
	if !hasSuffixFold(path, last) {
		return false
	}
	path = path[:len(path)-len(last)]
	// Middle segments must appear in order
	for _, part := range parts {
		idx := indexFold(path, part)
		if idx < 0 {
			return false
		}
		path = path[idx+len(part):]
	}
	return true
}

func splitGlob(pattern string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' {
			if i > start {
				parts = append(parts, pattern[start:i])
			}
			start = i + 1
		}
	}
	if start < len(pattern) {
		parts = append(parts, pattern[start:])
	}
	return parts
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return equalFold(s[:len(prefix)], prefix)
}

func hasSuffixFold(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return equalFold(s[len(s)-len(suffix):], suffix)
}

func indexFold(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
