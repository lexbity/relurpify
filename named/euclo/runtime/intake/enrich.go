package intake

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

// ClassifyCapabilityIntent performs capability-level classification using Tier 1 (static keywords),
// Tier 2 (LLM semantic), and Tier 3 (fallback). Result is returned directly; callers are
// responsible for persisting to state.
// This is extracted from agent_state_helpers.go classifyCapabilityIntent.
func ClassifyCapabilityIntent(
	ctx context.Context,
	task *core.Task,
	instruction string,
	modeID string,
	classifier CapabilityClassifier,
) (eucloruntime.CapabilityClassificationResult, error) {
	if classifier == nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("classifier not available")
	}

	seq, op, err := classifier.Classify(ctx, instruction, modeID)
	if err != nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("euclo capability classification: %w", err)
	}

	return eucloruntime.CapabilityClassificationResult{
		Sequence: seq,
		Operator: op,
		Source:   "classifier",
		Meta:     "", // Could be enriched with match details
	}, nil
}

// DefaultCapabilityClassifier creates a default classifier using the relurpic registry.
func DefaultCapabilityClassifier(registry *euclorelurpic.Registry, model core.LanguageModel, extraKeywords map[string][]string) CapabilityClassifier {
	if registry == nil {
		registry = euclorelurpic.DefaultRegistry()
	}
	return &TieredCapabilityClassifier{
		Registry:      registry,
		Model:         model,
		ExtraKeywords: extraKeywords,
	}
}

// TieredCapabilityClassifier implements capability classification directly in the intake layer.
type TieredCapabilityClassifier struct {
	Registry      *euclorelurpic.Registry
	Model         core.LanguageModel
	ExtraKeywords map[string][]string
}

func (c *TieredCapabilityClassifier) Classify(ctx context.Context, instruction, modeID string) ([]string, string, error) {
	matches := c.staticKeywordMatch(instruction, modeID)
	if len(matches) > 0 {
		result, ambiguous := c.resolveKeywordMatches(matches, instruction)
		if !ambiguous && len(result.Sequence) > 0 {
			return result.Sequence, result.Operator, nil
		}
		if ambiguous && c.Model != nil {
			candidates := make([]euclorelurpic.Descriptor, 0, len(matches))
			for _, m := range matches {
				candidates = append(candidates, m.Descriptor)
			}
			result, err := c.llmSemanticQuery(ctx, instruction, modeID, candidates)
			if err != nil {
				return nil, "", err
			}
			if len(result.Sequence) > 0 {
				return result.Sequence, result.Operator, nil
			}
		}
	}

	if c.Model != nil {
		if c.Registry != nil {
			candidates := c.Registry.PrimaryCapabilitiesForMode(modeID)
			if len(candidates) > 0 {
				result, err := c.llmSemanticQuery(ctx, instruction, modeID, candidates)
				if err != nil {
					return nil, "", err
				}
				if len(result.Sequence) > 0 {
					return result.Sequence, result.Operator, nil
				}
			}
		}
	}

	result, err := c.modeDefaultFallback(modeID)
	if err != nil {
		return nil, "", err
	}
	return result.Sequence, result.Operator, nil
}

func (c *TieredCapabilityClassifier) staticKeywordMatch(instruction, modeID string) []euclorelurpic.KeywordMatch {
	if c == nil || c.Registry == nil {
		return nil
	}
	return c.Registry.MatchByKeywords(instruction, modeID, c.ExtraKeywords)
}

func (c *TieredCapabilityClassifier) resolveKeywordMatches(matches []euclorelurpic.KeywordMatch, instruction string) (eucloruntime.CapabilityClassificationResult, bool) {
	if len(matches) == 0 {
		return eucloruntime.CapabilityClassificationResult{}, false
	}
	if len(matches) == 1 {
		return eucloruntime.CapabilityClassificationResult{
			Sequence: []string{matches[0].ID},
			Operator: "AND",
			Source:   "keyword",
			Meta:     strings.Join(matches[0].MatchedKeywords, ", "),
		}, false
	}
	if hasCompoundAndConnector(instruction) {
		seq := make([]string, 0, len(matches))
		metaParts := make([]string, 0, len(matches))
		for _, m := range matches {
			seq = append(seq, m.ID)
			metaParts = append(metaParts, fmt.Sprintf("%s:[%s]", m.ID, strings.Join(m.MatchedKeywords, ", ")))
		}
		return eucloruntime.CapabilityClassificationResult{
			Sequence: seq,
			Operator: "AND",
			Source:   "keyword",
			Meta:     strings.Join(metaParts, "; "),
		}, false
	}
	if hasCompoundOrConnector(instruction) {
		seq := make([]string, 0, len(matches))
		metaParts := make([]string, 0, len(matches))
		for _, m := range matches {
			seq = append(seq, m.ID)
			metaParts = append(metaParts, fmt.Sprintf("%s:[%s]", m.ID, strings.Join(m.MatchedKeywords, ", ")))
		}
		return eucloruntime.CapabilityClassificationResult{
			Sequence: seq,
			Operator: "OR",
			Source:   "keyword",
			Meta:     strings.Join(metaParts, "; "),
		}, false
	}
	highestCount := matches[0].MatchCount
	tiedMatches := 1
	for i := 1; i < len(matches); i++ {
		if matches[i].MatchCount == highestCount {
			tiedMatches++
		} else {
			break
		}
	}
	if tiedMatches > 1 {
		return eucloruntime.CapabilityClassificationResult{}, true
	}
	winner := matches[0]
	return eucloruntime.CapabilityClassificationResult{
		Sequence: []string{winner.ID},
		Operator: "AND",
		Source:   "keyword",
		Meta:     strings.Join(winner.MatchedKeywords, ", "),
	}, false
}

func (c *TieredCapabilityClassifier) modeDefaultFallback(modeID string) (eucloruntime.CapabilityClassificationResult, error) {
	if c == nil || c.Registry == nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("no capability match and no fallback for mode %q", modeID)
	}
	desc, ok := c.Registry.FallbackCapabilityForMode(modeID)
	if !ok {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("no capability match and no fallback for mode %q", modeID)
	}
	return eucloruntime.CapabilityClassificationResult{
		Sequence: []string{desc.ID},
		Operator: "AND",
		Source:   "fallback",
	}, nil
}

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

func hasCompoundAndConnector(instruction string) bool {
	lower := strings.ToLower(instruction)
	for _, re := range compoundAndPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

var compoundOrPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bor\b`),
	regexp.MustCompile(`(?i)\beither\b`),
	regexp.MustCompile(`(?i)\bwhether\b`),
	regexp.MustCompile(`(?i)\bone of\b`),
	regexp.MustCompile(`(?i)\bwhich (one|option|approach)\b`),
}

func hasCompoundOrConnector(instruction string) bool {
	lower := strings.ToLower(instruction)
	for _, re := range compoundOrPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

func (c *TieredCapabilityClassifier) llmSemanticQuery(
	ctx context.Context,
	instruction string,
	modeID string,
	candidates []euclorelurpic.Descriptor,
) (eucloruntime.CapabilityClassificationResult, error) {
	if c == nil || c.Model == nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("llm semantic query called with nil model")
	}

	prompt := buildLLMClassificationPrompt(instruction, modeID, candidates)
	resp, err := c.Model.Generate(ctx, prompt, &core.LLMOptions{MaxTokens: 64})
	if err != nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("llm classification query failed: %w", err)
	}
	if resp == nil || resp.Text == "" {
		return eucloruntime.CapabilityClassificationResult{}, nil
	}

	validIDs := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		validIDs[candidate.ID] = true
	}

	lines := strings.Split(resp.Text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "none") {
			return eucloruntime.CapabilityClassificationResult{}, nil
		}
		if strings.HasPrefix(line, "id:") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			if validIDs[id] {
				return eucloruntime.CapabilityClassificationResult{
					Sequence: []string{id},
					Operator: "AND",
					Source:   "llm_semantic",
					Meta:     id,
				}, nil
			}
		}
		if strings.HasPrefix(line, "sequence:") {
			rawSeq := strings.TrimSpace(strings.TrimPrefix(line, "sequence:"))
			parts := strings.Split(rawSeq, ",")
			seq := make([]string, 0, len(parts))
			for _, part := range parts {
				id := strings.TrimSpace(part)
				if id == "" || !validIDs[id] {
					continue
				}
				seq = append(seq, id)
			}
			if len(seq) > 0 {
				return eucloruntime.CapabilityClassificationResult{
					Sequence: seq,
					Operator: "AND",
					Source:   "llm_semantic",
					Meta:     rawSeq,
				}, nil
			}
		}
	}

	return eucloruntime.CapabilityClassificationResult{}, nil
}

func buildLLMClassificationPrompt(instruction, modeID string, candidates []euclorelurpic.Descriptor) string {
	var b strings.Builder
	b.WriteString("Select the best capability or sequence for mode ")
	b.WriteString(modeID)
	b.WriteString(".\nInstruction: ")
	b.WriteString(instruction)
	b.WriteString("\nCandidates:\n")
	for _, candidate := range candidates {
		b.WriteString("- ")
		b.WriteString(candidate.ID)
		if candidate.DisplayName != "" {
			b.WriteString(" (")
			b.WriteString(candidate.DisplayName)
			b.WriteString(")")
		}
		if len(candidate.Keywords) > 0 {
			b.WriteString(": ")
			b.WriteString(strings.Join(candidate.Keywords, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("Return either 'none', 'id:<capability-id>', or 'sequence:<id1,id2,...>'.")
	return b.String()
}
