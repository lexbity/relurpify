package tui

import (
	"fmt"
	"time"
)

// generateID produces a lightweight unique identifier for feed entries.
func generateID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

// max helper avoids importing math for a single use.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// estimateTokens performs a rough heuristic conversion from characters to tokens.
func estimateTokens(content string) int {
	if content == "" {
		return 0
	}
	return max(1, len(content)/4)
}
