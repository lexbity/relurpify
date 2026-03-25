package retrieval

import (
	"strings"
	"unicode"
)

// JaccardSimilarity computes word-level Jaccard similarity between two strings.
// Returns a value between 0.0 and 1.0, where 1.0 is identical.
func JaccardSimilarity(a, b string) float64 {
	if a == "" && b == "" {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	tokensA := tokenizeWords(a)
	tokensB := tokenizeWords(b)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	// Build sets
	setA := make(map[string]struct{}, len(tokensA))
	setB := make(map[string]struct{}, len(tokensB))
	for _, t := range tokensA {
		setA[t] = struct{}{}
	}
	for _, t := range tokensB {
		setB[t] = struct{}{}
	}

	// Compute intersection and union
	intersection := 0
	for t := range setA {
		if _, ok := setB[t]; ok {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// ExtractSentence extracts the sentence containing the given term from text.
// Returns the full sentence, its start byte offset, end byte offset, and whether the term was found.
func ExtractSentence(text, term string) (sentence string, start, end int, found bool) {
	if text == "" || term == "" {
		return "", 0, 0, false
	}

	// Normalize term for matching
	termLower := strings.ToLower(term)
	textLower := strings.ToLower(text)

	// Find the term in text
	idx := strings.Index(textLower, termLower)
	if idx == -1 {
		return "", 0, 0, false
	}

	// Find sentence boundaries (. ! ? or beginning/end of text)
	sentenceStart := 0
	sentenceEnd := len(text)

	// Search backwards for sentence start
	for i := idx - 1; i >= 0; i-- {
		if text[i] == '.' || text[i] == '!' || text[i] == '?' {
			sentenceStart = i + 1
			break
		}
	}

	// Search forwards for sentence end
	for i := idx + len(term); i < len(text); i++ {
		if text[i] == '.' || text[i] == '!' || text[i] == '?' {
			sentenceEnd = i + 1
			break
		}
	}

	sentence = strings.TrimSpace(text[sentenceStart:sentenceEnd])
	return sentence, sentenceStart, sentenceEnd, true
}

// TermPresent checks if a normalized term appears in text (case-insensitive).
func TermPresent(text, termNormalized string) bool {
	if text == "" || termNormalized == "" {
		return false
	}

	textLower := strings.ToLower(text)
	termLower := strings.ToLower(termNormalized)

	return strings.Contains(textLower, termLower)
}

// tokenizeWords splits text into words (alphanumeric sequences).
func tokenizeWords(text string) []string {
	if text == "" {
		return nil
	}

	// Replace non-alphanumeric with spaces
	var builder strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune(' ')
		}
	}

	// Split on whitespace
	words := strings.Fields(builder.String())
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}

	return words
}
