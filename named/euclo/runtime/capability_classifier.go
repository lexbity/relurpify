package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// CapabilityClassificationResult is the output of the three-tier classifier.
type CapabilityClassificationResult struct {
	Sequence []string // ordered capability IDs; len >= 1 on success
	Operator string   // "AND" | "OR"; only meaningful when len(Sequence) > 1
	Source   string   // "keyword" | "llm_semantic" | "fallback"
	Meta     string   // matched keyword(s) joined, or raw LLM response token
}

// CapabilityIntentClassifier routes an instruction to one or more capabilities
// within a resolved mode using three tiers: static keywords -> LLM query -> fallback.
type CapabilityIntentClassifier struct {
	Registry      *euclorelurpic.Registry
	ExtraKeywords map[string][]string // capability ID -> user-configured keywords (from manifest)
	Model         core.LanguageModel  // nil disables Tier 2 and goes to Tier 3 directly
}

// Classify returns the capability sequence for instruction in modeID.
// If Model is nil and no keywords match, it returns the mode's fallback capability.
// If Model is non-nil and both Tier 1 and Tier 2 fail or error, it returns an error
// that the caller must surface to the user.
func (c *CapabilityIntentClassifier) Classify(
	ctx context.Context,
	instruction string,
	modeID string,
) (CapabilityClassificationResult, error) {
	// Tier 1: Static keyword match
	matches := c.staticKeywordMatch(instruction, modeID)
	if len(matches) > 0 {
		result, ambiguous := c.resolveKeywordMatches(matches, instruction)
		if !ambiguous && len(result.Sequence) > 0 {
			return result, nil
		}
		// Ambiguous multi-match with tied scores: go to Tier 2 if available
		if ambiguous && c.Model != nil {
			candidates := make([]euclorelurpic.Descriptor, 0, len(matches))
			for _, m := range matches {
				candidates = append(candidates, m.Descriptor)
			}
			result, err := c.llmSemanticQuery(ctx, instruction, modeID, candidates)
			if err != nil {
				return CapabilityClassificationResult{}, err // hard provider error
			}
			if len(result.Sequence) > 0 {
				return result, nil
			}
			// LLM returned nothing or parse failed — fall through to Tier 3
		}
	}

	// Tier 2: LLM semantic approximation when no confident keyword match
	if c.Model != nil {
		// Get all candidates for this mode
		candidates := c.Registry.PrimaryCapabilitiesForMode(modeID)
		if len(candidates) > 0 {
			result, err := c.llmSemanticQuery(ctx, instruction, modeID, candidates)
			if err != nil {
				return CapabilityClassificationResult{}, err
			}
			// If LLM returned a valid result (not "none"), use it
			if len(result.Sequence) > 0 {
				return result, nil
			}
			// If LLM returned "none", fall through to Tier 3
		}
	}

	// Tier 3: Mode default fallback
	return c.modeDefaultFallback(modeID)
}

// staticKeywordMatch returns all capability descriptors that match keywords in the instruction.
func (c *CapabilityIntentClassifier) staticKeywordMatch(instruction, modeID string) []euclorelurpic.KeywordMatch {
	if c.Registry == nil {
		return nil
	}
	return c.Registry.MatchByKeywords(instruction, modeID, c.ExtraKeywords)
}

// resolveKeywordMatches converts keyword matches into a classification result.
// Handles single match, compound AND/OR detection, and highest match count selection.
// Returns (result, ambiguous) where ambiguous is true when multiple matches have same highest score.
func (c *CapabilityIntentClassifier) resolveKeywordMatches(matches []euclorelurpic.KeywordMatch, instruction string) (CapabilityClassificationResult, bool) {
	if len(matches) == 0 {
		return CapabilityClassificationResult{}, false
	}

	// Single match: return directly (not ambiguous)
	if len(matches) == 1 {
		return CapabilityClassificationResult{
			Sequence: []string{matches[0].ID},
			Operator: "AND",
			Source:   "keyword",
			Meta:     strings.Join(matches[0].MatchedKeywords, ", "),
		}, false
	}

	// Multiple matches: check for compound connectors (AND or OR)
	if hasCompoundAndConnector(instruction) {
		// Build AND sequence sorted by MatchCount descending
		seq := make([]string, 0, len(matches))
		for _, m := range matches {
			seq = append(seq, m.ID)
		}
		var metaParts []string
		for _, m := range matches {
			metaParts = append(metaParts, fmt.Sprintf("%s:[%s]", m.ID, strings.Join(m.MatchedKeywords, ", ")))
		}
		return CapabilityClassificationResult{
			Sequence: seq,
			Operator: "AND",
			Source:   "keyword",
			Meta:     strings.Join(metaParts, "; "),
		}, false
	}

	if hasCompoundOrConnector(instruction) {
		// Build OR sequence sorted by MatchCount descending
		seq := make([]string, 0, len(matches))
		for _, m := range matches {
			seq = append(seq, m.ID)
		}
		var metaParts []string
		for _, m := range matches {
			metaParts = append(metaParts, fmt.Sprintf("%s:[%s]", m.ID, strings.Join(m.MatchedKeywords, ", ")))
		}
		return CapabilityClassificationResult{
			Sequence: seq,
			Operator: "OR",
			Source:   "keyword",
			Meta:     strings.Join(metaParts, "; "),
		}, false
	}

	// No compound connector: check if top matches are tied
	highestCount := matches[0].MatchCount
	tiedMatches := 1
	for i := 1; i < len(matches); i++ {
		if matches[i].MatchCount == highestCount {
			tiedMatches++
		} else {
			break
		}
	}

	// If tied, signal ambiguity for Tier 2
	if tiedMatches > 1 {
		return CapabilityClassificationResult{}, true
	}

	// Clear winner: pick highest match count (already sorted by MatchCount descending)
	winner := matches[0]
	return CapabilityClassificationResult{
		Sequence: []string{winner.ID},
		Operator: "AND",
		Source:   "keyword",
		Meta:     strings.Join(winner.MatchedKeywords, ", "),
	}, false
}

// modeDefaultFallback returns the mode's fallback capability (Tier 3).
func (c *CapabilityIntentClassifier) modeDefaultFallback(modeID string) (CapabilityClassificationResult, error) {
	if c.Registry == nil {
		return CapabilityClassificationResult{}, fmt.Errorf("no capability match and no fallback for mode %q", modeID)
	}
	desc, ok := c.Registry.FallbackCapabilityForMode(modeID)
	if !ok {
		return CapabilityClassificationResult{}, fmt.Errorf("no capability match and no fallback for mode %q", modeID)
	}
	return CapabilityClassificationResult{
		Sequence: []string{desc.ID},
		Operator: "AND",
		Source:   "fallback",
	}, nil
}

// compoundAndPatterns detect sequencing intent in instructions.
var compoundAndPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\band then\b`),
	regexp.MustCompile(`(?i)\band also\b`),
	regexp.MustCompile(`(?i)\bfirst .{1,60} then\b`),
	regexp.MustCompile(`(?i)\bafter (that|which|doing so)\b`),
	regexp.MustCompile(`(?i)\bfollowed by\b`),
	regexp.MustCompile(`(?i)\bthen fix\b`),
	regexp.MustCompile(`(?i)\bthen implement\b`),
	regexp.MustCompile(`(?i)\bthen apply\b`),
}

// hasCompoundAndConnector returns true if the instruction contains explicit sequencing signals.
func hasCompoundAndConnector(instruction string) bool {
	lower := strings.ToLower(instruction)
	for _, re := range compoundAndPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

// compoundOrPatterns detect exclusive selection intent in instructions.
var compoundOrPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bor\b`),
	regexp.MustCompile(`(?i)\beither\b`),
	regexp.MustCompile(`(?i)\bwhether\b`),
	regexp.MustCompile(`(?i)\bone of\b`),
	regexp.MustCompile(`(?i)\bwhich (one|option|approach)\b`),
}

// hasCompoundOrConnector returns true if the instruction contains exclusive selection signals.
func hasCompoundOrConnector(instruction string) bool {
	lower := strings.ToLower(instruction)
	for _, re := range compoundOrPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

// llmSemanticQuery performs Tier 2 classification using an LLM query.
// Called when Tier 1 is ambiguous or when no keywords match.
func (c *CapabilityIntentClassifier) llmSemanticQuery(
	ctx context.Context,
	instruction string,
	modeID string,
	candidates []euclorelurpic.Descriptor,
) (CapabilityClassificationResult, error) {
	if c.Model == nil {
		return CapabilityClassificationResult{}, fmt.Errorf("llm semantic query called with nil model")
	}

	prompt := buildLLMClassificationPrompt(instruction, modeID, candidates)

	resp, err := c.Model.Generate(ctx, prompt, &core.LLMOptions{MaxTokens: 64})
	if err != nil {
		// Hard provider error: surface to caller
		return CapabilityClassificationResult{}, fmt.Errorf("llm classification query failed: %w", err)
	}
	if resp == nil || resp.Text == "" {
		// Empty response: fall through to Tier 3
		return CapabilityClassificationResult{}, nil
	}

	// Build valid IDs map for validation
	validIDs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		validIDs[c.ID] = true
	}

	ids, operator, err := parseCapabilitySequenceResponse(resp.Text, validIDs)
	if err != nil {
		// Parse/unknown-ID error: log and fall through to Tier 3
		// TODO: replace with structured logger when available
		_ = fmt.Sprintf("euclo classifier: tier 2 parse error (falling through to tier 3): %v", err)
		return CapabilityClassificationResult{}, nil
	}

	// "none" response signals to go to Tier 3
	if len(ids) == 0 {
		return CapabilityClassificationResult{}, nil
	}

	return CapabilityClassificationResult{
		Sequence: ids,
		Operator: operator,
		Source:   "llm_semantic",
		Meta:     strings.TrimSpace(resp.Text),
	}, nil
}

// buildLLMClassificationPrompt constructs the prompt for the LLM classifier.
func buildLLMClassificationPrompt(instruction string, modeID string, candidates []euclorelurpic.Descriptor) string {
	var sb strings.Builder
	sb.WriteString("You are a routing assistant for a coding agent. Select the capability or capabilities that best handle the request.\n")
	sb.WriteString(fmt.Sprintf("Mode: %s\n\n", modeID))
	sb.WriteString("Available capabilities:\n")

	for _, c := range candidates {
		keywords := strings.Join(c.Keywords, ", ")
		sb.WriteString(fmt.Sprintf("%s: keywords=[%s] — %s\n", c.ID, keywords, c.Summary))
	}

	sb.WriteString(fmt.Sprintf("\nRequest: \"%s\"\n\n", instruction))
	sb.WriteString("Reply with one of:\n")
	sb.WriteString("- A single capability ID: euclo:chat.ask\n")
	sb.WriteString("- An AND sequence (both run, ordered): euclo:chat.inspect AND euclo:chat.implement\n")
	sb.WriteString("- An OR sequence (pick first/best): euclo:debug.investigate-repair OR euclo:debug.repair.simple\n")
	sb.WriteString("- none (if the request does not match any capability above)\n\n")
	sb.WriteString("Reply with ONLY the capability expression. No explanation.")

	return sb.String()
}

// parseCapabilitySequenceResponse parses the LLM response into capability IDs and operator.
// Returns empty ids and nil error for "none" response.
func parseCapabilitySequenceResponse(raw string, validIDs map[string]bool) (ids []string, operator string, err error) {
	trimmed := strings.TrimSpace(raw)
	if strings.EqualFold(trimmed, "none") {
		return nil, "AND", nil
	}

	// Detect operator
	andRe := regexp.MustCompile(`(?i)\s+AND\s+`)
	orRe := regexp.MustCompile(`(?i)\s+OR\s+`)

	switch {
	case andRe.MatchString(trimmed):
		operator = "AND"
		parts := andRe.Split(trimmed, -1)
		for _, p := range parts {
			id := strings.TrimSpace(p)
			if id == "" {
				continue
			}
			if !validIDs[id] {
				return nil, "", fmt.Errorf("unknown capability ID: %q", id)
			}
			ids = append(ids, id)
		}
	case orRe.MatchString(trimmed):
		operator = "OR"
		parts := orRe.Split(trimmed, -1)
		for _, p := range parts {
			id := strings.TrimSpace(p)
			if id == "" {
				continue
			}
			if !validIDs[id] {
				return nil, "", fmt.Errorf("unknown capability ID: %q", id)
			}
			ids = append(ids, id)
		}
	default:
		// Single capability
		operator = "AND"
		id := trimmed
		if !validIDs[id] {
			return nil, "", fmt.Errorf("unknown capability ID: %q", id)
		}
		ids = []string{id}
	}

	if len(ids) == 0 {
		return nil, "", fmt.Errorf("no valid capability IDs found in response: %q", raw)
	}

	return ids, operator, nil
}
