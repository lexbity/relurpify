package ingestion

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"
)

// defaultScanners returns the default set of scanners.
func defaultScanners() []Scanner {
	return []Scanner{
		&SignatureScanner{},
		&UnicodeTagScanner{},
		&StructuralHeuristicScanner{},
		&Base64BlobScanner{},
	}
}

// SignatureScanner detects known injection patterns via regex.
type SignatureScanner struct {
	Patterns []*regexp.Regexp
}

func (s *SignatureScanner) Name() string {
	return "signature"
}

func (s *SignatureScanner) Scan(ctx context.Context, chunk TypedChunk) ScanResult {
	if len(s.Patterns) == 0 {
		// Initialize default patterns
		s.Patterns = []*regexp.Regexp{
			// SQL injection patterns
			regexp.MustCompile(`(?i)(SELECT\s+.*FROM|INSERT\s+INTO|UPDATE\s+.*SET|DELETE\s+FROM)`),
			// Command injection
			regexp.MustCompile(`(?i)(;\s*rm\s+-rf|\|\s*bash|\|\s*sh\s+-c)`),
			// XSS patterns
			regexp.MustCompile(`(?i)(<script|javascript:|on\w+\s*=)`),
			// Path traversal
			regexp.MustCompile(`\.\.(/|\\)`),
			// Common backdoors
			regexp.MustCompile(`(?i)(eval\s*\(|exec\s*\(|system\s*\()`),
		}
	}

	content := string(chunk.Content)
	result := ScanResult{
		SuspicionScore: 0,
		Flags:          []string{},
	}

	for _, pattern := range s.Patterns {
		if pattern.MatchString(content) {
			result.SuspicionScore = max(result.SuspicionScore, 0.8)
			patternStr := pattern.String()
			truncLen := len(patternStr)
			if truncLen > 20 {
				truncLen = 20
			}
			result.Flags = append(result.Flags, "signature_match:"+patternStr[:truncLen])
		}
	}

	return result
}

// UnicodeTagScanner detects invisible Unicode tag characters.
type UnicodeTagScanner struct{}

func (s *UnicodeTagScanner) Name() string {
	return "unicode_tag"
}

func (s *UnicodeTagScanner) Scan(ctx context.Context, chunk TypedChunk) ScanResult {
	result := ScanResult{
		SuspicionScore: 0,
		Flags:          []string{},
	}

	// Unicode tag characters range: U+E0000 to U+E007F
	hasTagChars := false
	tagCharCount := 0

	for _, r := range string(chunk.Content) {
		if r >= 0xE0000 && r <= 0xE007F {
			hasTagChars = true
			tagCharCount++
		}
	}

	if hasTagChars {
		// Suspicion increases with tag character density
		density := float64(tagCharCount) / float64(len(chunk.Content))
		result.SuspicionScore = min(0.3+density*10, 0.95)
		result.Flags = append(result.Flags, "unicode_tag_chars")
		result.Flags = append(result.Flags, "invisible_chars")
	}

	return result
}

// StructuralHeuristicScanner detects suspicious structural patterns.
type StructuralHeuristicScanner struct{}

func (s *StructuralHeuristicScanner) Name() string {
	return "structural_heuristic"
}

func (s *StructuralHeuristicScanner) Scan(ctx context.Context, chunk TypedChunk) ScanResult {
	result := ScanResult{
		SuspicionScore: 0,
		Flags:          []string{},
	}

	content := string(chunk.Content)
	lines := strings.Split(content, "\n")

	// Check for role-switching patterns (imperative verbs, second-person AI addressing)
	imperativeCount := 0
	aiAddressingCount := 0

	imperativeVerbs := []string{"ignore", "disregard", "forget", "do not", "don't", "stop", "halt"}
	aiAddressings := []string{"you are", "you're", "you must", "you should", "as an ai", "as a language model"}

	for _, line := range lines {
		lower := strings.ToLower(line)

		for _, verb := range imperativeVerbs {
			if strings.Contains(lower, verb) {
				imperativeCount++
			}
		}

		for _, addressing := range aiAddressings {
			if strings.Contains(lower, addressing) {
				aiAddressingCount++
			}
		}
	}

	// Calculate suspicion based on density of suspicious patterns
	if len(lines) > 0 {
		imperativeDensity := float64(imperativeCount) / float64(len(lines))
		aiDensity := float64(aiAddressingCount) / float64(len(lines))

		if imperativeDensity > 0.1 {
			result.SuspicionScore = max(result.SuspicionScore, 0.6)
			result.Flags = append(result.Flags, "high_imperative_density")
		}

		if aiDensity > 0.05 {
			result.SuspicionScore = max(result.SuspicionScore, 0.5)
			result.Flags = append(result.Flags, "ai_addressing")
		}
	}

	// Check for repeated suspicious patterns
	if imperativeCount > 5 || aiAddressingCount > 3 {
		result.SuspicionScore = max(result.SuspicionScore, 0.7)
		result.Flags = append(result.Flags, "repeated_suspicious_patterns")
	}

	return result
}

// Base64BlobScanner detects and decodes suspicious Base64 blobs.
type Base64BlobScanner struct{}

func (s *Base64BlobScanner) Name() string {
	return "base64_blob"
}

func (s *Base64BlobScanner) Scan(ctx context.Context, chunk TypedChunk) ScanResult {
	result := ScanResult{
		SuspicionScore: 0,
		Flags:          []string{},
	}

	content := string(chunk.Content)

	// Look for base64-like sequences (long strings of base64 characters)
	base64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	matches := base64Pattern.FindAllString(content, -1)

	for _, match := range matches {
		// Try to decode
		if decoded, err := base64.StdEncoding.DecodeString(match); err == nil {
			// Successfully decoded - check if decoded content is suspicious
			decodedStr := string(decoded)

			// Check for suspicious patterns in decoded content
			if containsSuspiciousPatterns(decodedStr) {
				result.SuspicionScore = max(result.SuspicionScore, 0.9)
				result.Flags = append(result.Flags, "encoded_suspicious_content")
			} else {
				// Just encoded, not necessarily suspicious
				result.SuspicionScore = max(result.SuspicionScore, 0.3)
				result.Flags = append(result.Flags, "base64_encoded")
			}
		}
	}

	return result
}

func containsSuspiciousPatterns(s string) bool {
	suspicious := []string{
		"eval", "exec", "system", "shell", "script",
		"import", "from", "__import__", "compile",
	}

	lower := strings.ToLower(s)
	for _, pattern := range suspicious {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// IsPrintable checks if a rune is printable.
func IsPrintable(r rune) bool {
	return unicode.IsPrint(r) || unicode.IsSpace(r)
}

// CountNonPrintable counts non-printable characters in a string.
func CountNonPrintable(s string) int {
	count := 0
	for _, r := range s {
		if !IsPrintable(r) && r != '\n' && r != '\r' && r != '\t' {
			count++
		}
	}
	return count
}
