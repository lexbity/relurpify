package summarization

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// ASTSummarizer preserves function signatures, class declarations, doc comments;
// elides function bodies. Deterministic; no model call.
type ASTSummarizer struct{}

// Kind returns the kind of summarizer.
func (s *ASTSummarizer) Kind() SummarizerKind {
	return SummarizerKindAST
}

// CanSummarize checks if this summarizer can handle the given chunks.
func (s *ASTSummarizer) CanSummarize(chunks []knowledge.KnowledgeChunk) bool {
	if len(chunks) == 0 {
		return false
	}
	// Check if any chunk has code-like content type
	for _, chunk := range chunks {
		contentType := s.getContentType(chunk)
		switch contentType {
		case "go", "python", "rust", "javascript", "typescript", "java", "c", "cpp", "csharp":
			return true
		}
	}
	return false
}

// Summarize generates a summary preserving signatures and eliding bodies.
func (s *ASTSummarizer) Summarize(ctx context.Context, req SummarizationRequest) (*SummarizationResult, error) {
	if len(req.Chunks) == 0 {
		return nil, fmt.Errorf("no chunks to summarize")
	}

	var summaries []string
	var sourceCoverage []knowledge.ChunkID

	for _, chunk := range req.Chunks {
		content := s.getContent(chunk)
		contentType := s.getContentType(chunk)
		summary := s.summarizeCode(content, contentType)
		summaries = append(summaries, summary)
		sourceCoverage = append(sourceCoverage, chunk.ID)
	}

	combined := strings.Join(summaries, "\n\n")

	// Compute coverage hash
	coverageHash := ComputeCoverageHash(req.Chunks)

	// Build derivation method fingerprint
	derivationMethod := fmt.Sprintf("ast_summarizer:%s", coverageHash[:8])

	return &SummarizationResult{
		Summary:          combined,
		TokenEstimate:    CountTokens(combined),
		DerivationMethod: derivationMethod,
		SourceCoverage:   sourceCoverage,
		CoverageHash:     coverageHash,
		UsedModel:        false,
	}, nil
}

// summarizeCode summarizes code content based on language.
func (s *ASTSummarizer) summarizeCode(content string, contentType string) string {
	switch contentType {
	case "go":
		return s.summarizeGo(content)
	case "python":
		return s.summarizePython(content)
	case "rust":
		return s.summarizeRust(content)
	case "javascript", "typescript":
		return s.summarizeJS(content)
	default:
		return s.summarizeGeneric(content)
	}
}

// summarizeGo summarizes Go code.
func (s *ASTSummarizer) summarizeGo(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	inFuncBody := false
	braceDepth := 0
	funcDecl := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Function declaration
		if strings.HasPrefix(trimmed, "func ") && !inFuncBody {
			// Check if it's a one-line function
			if strings.Contains(trimmed, "{") {
				if strings.Contains(trimmed, "}") {
					// One-liner - keep it
					result = append(result, line)
				} else {
					funcDecl = line
					inFuncBody = true
					braceDepth = 1
				}
			} else {
				funcDecl = line
				// Wait for opening brace
			}
			continue
		}

		// If collecting function declaration
		if funcDecl != "" && !inFuncBody {
			funcDecl += " " + trimmed
			if strings.Contains(trimmed, "{") {
				inFuncBody = true
				braceDepth = 1
			}
			continue
		}

		// Inside function body - track brace depth
		if inFuncBody {
			for _, ch := range line {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}

			if braceDepth == 0 {
				// End of function - emit signature + elided body
				result = append(result, funcDecl)
				result = append(result, "\t// ...")
				result = append(result, "}")
				funcDecl = ""
				inFuncBody = false
			}
			continue
		}

		// Type declarations - preserve
		if strings.HasPrefix(trimmed, "type ") {
			result = append(result, line)
			continue
		}

		// Package and imports - preserve
		if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "import ") {
			result = append(result, line)
			continue
		}

		// Comments - preserve doc comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			result = append(result, line)
			continue
		}

		// Variable/constant declarations at package level - preserve
		if strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "const ") {
			// Simple declarations only
			if !strings.Contains(trimmed, "{") {
				result = append(result, line)
			}
			continue
		}

		// Other top-level declarations - preserve
		result = append(result, line)
	}

	// Handle incomplete function
	if funcDecl != "" {
		result = append(result, funcDecl)
		result = append(result, "\t// ...")
		result = append(result, "}")
	}

	return strings.Join(result, "\n")
}

// summarizePython summarizes Python code.
func (s *ASTSummarizer) summarizePython(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	indentRegex := regexp.MustCompile(`^(\s*)`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Function/class definitions - preserve and mark body
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") {
			result = append(result, line)
			// Look for function body start
			currentIndent := len(indentRegex.FindString(line))
			j := i + 1
			for j < len(lines) {
				nextLine := lines[j]
				nextTrimmed := strings.TrimSpace(nextLine)
				if nextTrimmed == "" {
					j++
					continue
				}
				nextIndent := len(indentRegex.FindString(nextLine))
				if nextIndent > currentIndent {
					// This is body content - skip until next same-level or dedent
					result = append(result, strings.Repeat(" ", currentIndent+4)+"# ...")
					for j < len(lines) {
						checkLine := lines[j]
						if strings.TrimSpace(checkLine) == "" {
							j++
							continue
						}
						checkIndent := len(indentRegex.FindString(checkLine))
						if checkIndent <= currentIndent {
							break
						}
						j++
					}
					break
				} else {
					break
				}
			}
			continue
		}

		// Comments - preserve docstrings and comments
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "\"") {
			result = append(result, line)
			continue
		}

		// Import statements - preserve
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
			result = append(result, line)
			continue
		}

		// Module-level assignments - preserve
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, " ") {
			result = append(result, line)
			continue
		}
	}

	return strings.Join(result, "\n")
}

// summarizeRust summarizes Rust code.
func (s *ASTSummarizer) summarizeRust(content string) string {
	// Similar to Go but with Rust-specific syntax
	lines := strings.Split(content, "\n")
	var result []string

	inFuncBody := false
	braceDepth := 0
	funcDecl := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Function with body
		if strings.HasPrefix(trimmed, "fn ") {
			if strings.Contains(line, "{") {
				if !strings.Contains(line, "}") {
					funcDecl = line
					inFuncBody = true
					braceDepth = 1
					continue
				}
			}
			result = append(result, line)
			continue
		}

		// Struct/enum/trait declarations - preserve
		if strings.HasPrefix(trimmed, "struct ") ||
			strings.HasPrefix(trimmed, "enum ") ||
			strings.HasPrefix(trimmed, "trait ") ||
			strings.HasPrefix(trimmed, "impl ") {
			result = append(result, line)
			continue
		}

		// Inside function body
		if inFuncBody {
			for _, ch := range line {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}

			if braceDepth == 0 {
				result = append(result, funcDecl)
				result = append(result, "    // ...")
				result = append(result, "}")
				funcDecl = ""
				inFuncBody = false
			}
			continue
		}

		// Module declarations - preserve
		if strings.HasPrefix(trimmed, "mod ") || strings.HasPrefix(trimmed, "use ") {
			result = append(result, line)
			continue
		}

		// Attributes - preserve
		if strings.HasPrefix(trimmed, "#") {
			result = append(result, line)
			continue
		}
	}

	if funcDecl != "" {
		result = append(result, funcDecl)
		result = append(result, "    // ...")
		result = append(result, "}")
	}

	return strings.Join(result, "\n")
}

// summarizeJS summarizes JavaScript/TypeScript code.
func (s *ASTSummarizer) summarizeJS(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	braceDepth := 0
	declStart := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Function declarations
		funcPattern := regexp.MustCompile(`^(function|const|let|var)\s+\w+\s*[\(=]`)
		if funcPattern.MatchString(trimmed) {
			if strings.Contains(line, "{") {
				declStart = i
				braceDepth = 1
				continue
			}
		}

		// Class declarations
		if strings.HasPrefix(trimmed, "class ") {
			result = append(result, line)
			continue
		}

		// Arrow functions
		if strings.Contains(trimmed, "=>") && strings.Contains(line, "{") {
			declStart = i
			braceDepth = 1
			continue
		}

		// Inside function body
		if declStart >= 0 {
			for _, ch := range line {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}

			if braceDepth == 0 {
				// End of function - emit signature + elided body
				result = append(result, lines[declStart])
				result = append(result, "  // ...")
				result = append(result, "}")
				declStart = -1
			}
			continue
		}

		// Import/export - preserve
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "export ") {
			result = append(result, line)
			continue
		}

		// Comments - preserve
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			result = append(result, line)
			continue
		}

		// TypeScript interfaces/types
		if strings.HasPrefix(trimmed, "interface ") ||
			strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "declare ") {
			result = append(result, line)
			continue
		}
	}

	return strings.Join(result, "\n")
}

// summarizeGeneric provides generic code summarization.
func (s *ASTSummarizer) summarizeGeneric(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Keep function signatures (rough heuristic)
		if strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") &&
			(strings.Contains(trimmed, "function") || strings.Contains(trimmed, "def ") ||
				strings.Contains(trimmed, "func ") || strings.Contains(trimmed, "fn ")) {
			result = append(result, line)
			result = append(result, "  // ...")
			continue
		}

		// Keep class/type declarations
		if strings.Contains(trimmed, "class ") || strings.Contains(trimmed, "struct ") ||
			strings.Contains(trimmed, "interface ") || strings.Contains(trimmed, "type ") {
			result = append(result, line)
			continue
		}

		// Keep comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			result = append(result, line)
			continue
		}
	}

	return strings.Join(result, "\n")
}

// getContent extracts content from a chunk.
func (s *ASTSummarizer) getContent(chunk knowledge.KnowledgeChunk) string {
	return chunk.Body.Raw
}

// getContentType extracts the content type from a chunk.
func (s *ASTSummarizer) getContentType(chunk knowledge.KnowledgeChunk) string {
	// Check Body.Fields first
	if contentType, ok := chunk.Body.Fields["content_type"].(string); ok {
		return contentType
	}
	// Default to empty
	return ""
}

// NewASTSummarizer creates a new AST summarizer.
func NewASTSummarizer() *ASTSummarizer {
	return &ASTSummarizer{}
}
