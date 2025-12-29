package tui

import (
	"fmt"
	"strings"
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

func estimateTokensFromBytes(size int64) int {
	if size <= 0 {
		return 0
	}
	return max(1, int(size)/4)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024.0)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1024.0*1024.0))
	}
	return fmt.Sprintf("%.1fGB", float64(n)/(1024.0*1024.0*1024.0))
}

func formatSizeToken(size int64, tokens int) string {
	if tokens <= 0 {
		tokens = estimateTokensFromBytes(size)
	}
	return fmt.Sprintf("%s | ~%d tok", formatBytes(size), tokens)
}

func fuzzyMatchScore(query, target string) (bool, int) {
	if query == "" {
		return true, 0
	}
	q := strings.ToLower(query)
	t := strings.ToLower(target)
	qRunes := []rune(q)
	tRunes := []rune(t)
	qIndex := 0
	score := 0
	consecutive := 0
	start := -1
	for i := 0; i < len(tRunes) && qIndex < len(qRunes); i++ {
		if tRunes[i] == qRunes[qIndex] {
			if start == -1 {
				start = i
			}
			if qIndex > 0 && i > 0 && tRunes[i-1] == qRunes[qIndex-1] {
				consecutive++
				score += 6
			} else {
				consecutive = 0
				score += 2
			}
			qIndex++
		} else if consecutive > 0 {
			consecutive = 0
		}
	}
	if qIndex != len(qRunes) {
		return false, 0
	}
	if start >= 0 {
		score += max(0, 20-start)
	}
	score += max(0, 10-(len(tRunes)-len(qRunes)))
	return true, score
}
