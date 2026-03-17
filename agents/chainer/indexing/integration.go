package indexing

import (
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// IndexingListener observes ChainerAgent execution and indexes code snippets.
//
// Phase 7: Hooks into link execution to capture evaluated code for later retrieval.
// Extracts symbols and metadata from evaluation results to support context-aware
// tool invocations in subsequent links.
type IndexingListener struct {
	index      *CodeIndex
	retriever  *Retriever
	taskID     string
	idCounter  int
}

// NewIndexingListener creates a listener that will index code into the given index.
func NewIndexingListener(index *CodeIndex, taskID string) *IndexingListener {
	if index == nil {
		return nil
	}

	return &IndexingListener{
		index:     index,
		retriever: NewRetriever(index),
		taskID:    taskID,
		idCounter: 0,
	}
}

// OnLinkEvaluated is called when a link completes evaluation.
//
// Phase 7 stub: Records the evaluated code with basic metadata extraction.
// Future phases will add:
// - AST-based symbol extraction (using framework/ast)
// - Dependency analysis
// - Cross-link correlation
func (l *IndexingListener) OnLinkEvaluated(linkName string, result *core.LLMResponse) error {
	if l == nil || result == nil {
		return fmt.Errorf("listener or result not initialized")
	}

	l.idCounter++
	snippetID := fmt.Sprintf("%s-link-%s-%d", l.taskID, linkName, l.idCounter)

	snippet := &IndexedCodeSnippet{
		ID:        snippetID,
		TaskID:    l.taskID,
		LinkName:  linkName,
		Source:    result.Text,
		Timestamp: time.Now(),
		Language:  detectLanguage(result.Text),
	}

	// Extract basic symbols from the response
	snippet.Symbols = extractSymbols(result.Text)

	return l.index.Index(snippet)
}

// OnLinkFailed is called when a link evaluation fails.
//
// Records the failed code for error analysis and recovery planning.
func (l *IndexingListener) OnLinkFailed(linkName string, err error, context string) error {
	if l == nil {
		return fmt.Errorf("listener not initialized")
	}

	l.idCounter++
	snippetID := fmt.Sprintf("%s-error-%s-%d", l.taskID, linkName, l.idCounter)

	snippet := &IndexedCodeSnippet{
		ID:           snippetID,
		TaskID:       l.taskID,
		LinkName:     linkName,
		Source:       context,
		IsError:      true,
		ErrorMessage: err.Error(),
		Timestamp:    time.Now(),
		Language:     detectLanguage(context),
	}

	snippet.Symbols = extractSymbols(context)

	return l.index.Index(snippet)
}

// QueryContext retrieves relevant context for a link invocation.
//
// Phase 7: Returns indexed snippets that may be relevant to the current operation.
// Searches by:
// 1. Same link name (previous invocations of this link)
// 2. Similar symbols
// 3. Same language/file path
func (l *IndexingListener) QueryContext(linkName string, limit int) []*IndexedCodeSnippet {
	if l == nil || l.retriever == nil {
		return nil
	}

	// Start with previous invocations of same link
	query := &RetrievalQuery{
		LinkName: linkName,
		Limit:    limit,
	}

	results := l.retriever.Retrieve(query)
	if len(results) > 0 {
		return results
	}

	// Fallback: Recent snippets
	return l.retriever.RetrieveRecent(1*time.Hour, limit)
}

// Stats returns statistics about indexed code for this task.
func (l *IndexingListener) Stats() *Statistics {
	if l == nil || l.retriever == nil {
		return &Statistics{}
	}

	return l.retriever.Stats()
}

// Clear removes all indexed snippets.
func (l *IndexingListener) Clear() {
	if l == nil || l.index == nil {
		return
	}

	l.index.Clear()
	l.idCounter = 0
}

// Helper functions

// detectLanguage identifies the programming language from source code.
//
// Phase 7 stub: Returns guesses based on simple patterns.
// Phase 7+ should integrate with framework/ast LanguageDetector.
func detectLanguage(source string) string {
	if source == "" {
		return ""
	}

	// Simple heuristics
	if hasPattern(source, "package ", "func ", "type ", "interface") {
		return "go"
	}
	if hasPattern(source, "def ", "import ", "class ") {
		return "python"
	}
	if hasPattern(source, "function ", "const ", "let ", "var ") {
		return "javascript"
	}
	if hasPattern(source, "class ", "public ", "private ") {
		return "java"
	}
	if hasPattern(source, "fn ", "let ", "match ") {
		return "rust"
	}

	return "unknown"
}

// extractSymbols extracts symbol names from source code.
//
// Phase 7 stub: Returns identifiers that look like function/type names.
// Phase 7+ should use framework/ast parsers for accurate extraction.
func extractSymbols(source string) []string {
	var symbols []string
	seen := make(map[string]bool)

	// Extract function/method names (simple pattern matching)
	patterns := []string{
		"func ",
		"type ",
		"interface ",
		"const ",
		"var ",
		"class ",
		"def ",
	}

	for _, pattern := range patterns {
		if idx := indexOf(source, pattern); idx >= 0 {
			start := idx + len(pattern)
			// Find next valid identifier
			for start < len(source) && (isWhitespace(rune(source[start]))) {
				start++
			}

			end := start
			for end < len(source) && isIdentifierChar(rune(source[end])) {
				end++
			}

			if end > start {
				sym := source[start:end]
				if !seen[sym] {
					symbols = append(symbols, sym)
					seen[sym] = true
				}
			}
		}
	}

	return symbols
}

// Helper utility functions

func hasPattern(s string, patterns ...string) bool {
	for _, pattern := range patterns {
		if indexOf(s, pattern) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isIdentifierChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}
