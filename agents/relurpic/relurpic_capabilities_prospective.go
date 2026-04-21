package relurpic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type prospectiveMatcherMatchCapabilityHandler struct {
	model        core.LanguageModel
	config       *core.Config
	patternStore patterns.PatternStore
	retrievalDB  *sql.DB
}

func (h prospectiveMatcherMatchCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:prospective-matcher.match",
		"prospective-matcher.match",
		"Match a new feature description against confirmed workspace and external patterns.",
		core.CapabilityKindTool,
		core.CoordinationRoleDomainPack,
		[]string{"analyze", "match"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredObjectSchema(map[string]*core.Schema{
			"description":  {Type: "string"},
			"corpus_scope": {Type: "string"},
			"limit":        {Type: "integer"},
			"min_score":    {Type: "number"},
		}, "description", "corpus_scope"),
		structuredObjectSchema(map[string]*core.Schema{
			"matches": {
				Type:  "array",
				Items: &core.Schema{Type: "object"},
			},
			"count": {Type: "integer"},
		}, "matches", "count"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "prospective-match",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h prospectiveMatcherMatchCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]any) (*core.CapabilityExecutionResult, error) {
	description := stringArg(args["description"])
	if description == "" {
		return nil, fmt.Errorf("description required")
	}
	corpusScope := stringArg(args["corpus_scope"])
	if corpusScope == "" {
		return nil, fmt.Errorf("corpus_scope required")
	}
	limit := intArg(args["limit"], 10)
	if limit <= 0 {
		limit = 10
	}
	minScore := floatArg(args["min_score"], 0.3)
	if minScore < 0 {
		minScore = 0
	}

	if h.patternStore == nil {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]any{
				"matches": []any{},
				"count":   0,
			},
		}, nil
	}

	candidates, err := h.loadConfirmedPatterns(ctx, corpusScope)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]any{
				"matches": []any{},
				"count":   0,
			},
		}, nil
	}

	prefiltered := prefilterPatternCandidates(description, candidates)
	if len(prefiltered) == 0 {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]any{
				"matches": []any{},
				"count":   0,
			},
		}, nil
	}

	llmScores, err := h.rankPatternCandidates(ctx, description, prefiltered)
	if err != nil {
		return nil, err
	}

	matches := mergePatternCandidateScores(prefiltered, llmScores, minScore, limit)
	h.attachMatchingAnchors(ctx, corpusScope, description, matches)

	return &core.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"matches": prospectiveMatchesAsAny(matches),
			"count":   len(matches),
		},
	}, nil
}

func (h prospectiveMatcherMatchCapabilityHandler) loadConfirmedPatterns(ctx context.Context, corpusScope string) ([]patterns.PatternRecord, error) {
	kinds := []patterns.PatternKind{
		patterns.PatternKindStructural,
		patterns.PatternKindSemantic,
		patterns.PatternKindBehavioral,
		patterns.PatternKindBoundary,
	}
	out := make([]patterns.PatternRecord, 0)
	seen := make(map[string]struct{})
	for _, kind := range kinds {
		records, err := h.patternStore.ListByKind(ctx, kind, corpusScope)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if record.Status != patterns.PatternStatusConfirmed {
				continue
			}
			if _, ok := seen[record.ID]; ok {
				continue
			}
			seen[record.ID] = struct{}{}
			out = append(out, record)
		}
	}
	return out, nil
}

type patternCandidate struct {
	Record       patterns.PatternRecord
	JaccardScore float64
}

func prefilterPatternCandidates(description string, records []patterns.PatternRecord) []patternCandidate {
	out := make([]patternCandidate, 0, len(records))
	for _, record := range records {
		score := retrieval.JaccardSimilarity(description, record.Description)
		if score < 0.15 {
			continue
		}
		out = append(out, patternCandidate{Record: record, JaccardScore: score})
	}
	return out
}

type llmPatternMatchScore struct {
	PatternID string  `json:"pattern_id"`
	Relevance float64 `json:"relevance"`
}

func (h prospectiveMatcherMatchCapabilityHandler) rankPatternCandidates(ctx context.Context, description string, candidates []patternCandidate) (map[string]float64, error) {
	prompt := buildProspectiveMatcherPrompt(description, candidates)
	resp, err := h.model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       modelName(h.config),
		Temperature: 0.1,
		MaxTokens:   1000,
	})
	if err != nil {
		return nil, err
	}
	scores, err := parseProspectiveMatcherResponse(resp.Text)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(scores))
	for _, score := range scores {
		out[score.PatternID] = clampConfidence(score.Relevance)
	}
	return out, nil
}

func buildProspectiveMatcherPrompt(description string, candidates []patternCandidate) string {
	payload := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		payload = append(payload, map[string]any{
			"id":          candidate.Record.ID,
			"title":       candidate.Record.Title,
			"description": candidate.Record.Description,
			"kind":        candidate.Record.Kind,
		})
	}
	raw, _ := json.Marshal(payload)
	return fmt.Sprintf("New feature description: %s\nPatterns (JSON array): %s\nScore each pattern's relevance to the new feature. Return valid JSON: [{\"pattern_id\":\"...\",\"relevance\":0.0}].", description, string(raw))
}

func parseProspectiveMatcherResponse(text string) ([]llmPatternMatchScore, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "[") {
		var direct []llmPatternMatchScore
		if err := json.Unmarshal([]byte(trimmed), &direct); err != nil {
			return nil, err
		}
		return direct, nil
	}
	extracted := reactpkg.ExtractJSON(text)
	var payload struct {
		Matches []llmPatternMatchScore `json:"matches"`
	}
	if err := json.Unmarshal([]byte(extracted), &payload); err == nil && payload.Matches != nil {
		return payload.Matches, nil
	}
	var direct []llmPatternMatchScore
	if err := json.Unmarshal([]byte(extracted), &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

type prospectivePatternMatch struct {
	PatternID    string   `json:"pattern_id"`
	Title        string   `json:"title"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Relevance    float64  `json:"relevance"`
	CorpusSource string   `json:"corpus_source"`
	AnchorRefs   []string `json:"anchor_refs,omitempty"`
}

func mergePatternCandidateScores(candidates []patternCandidate, llmScores map[string]float64, minScore float64, limit int) []prospectivePatternMatch {
	out := make([]prospectivePatternMatch, 0, len(candidates))
	for _, candidate := range candidates {
		llmScore := llmScores[candidate.Record.ID]
		merged := 0.7*llmScore + 0.3*candidate.JaccardScore
		if merged < minScore {
			continue
		}
		out = append(out, prospectivePatternMatch{
			PatternID:    candidate.Record.ID,
			Title:        candidate.Record.Title,
			Kind:         string(candidate.Record.Kind),
			Description:  candidate.Record.Description,
			Relevance:    merged,
			CorpusSource: candidate.Record.CorpusSource,
			AnchorRefs:   append([]string(nil), candidate.Record.AnchorRefs...),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Relevance == out[j].Relevance {
			return out[i].PatternID < out[j].PatternID
		}
		return out[i].Relevance > out[j].Relevance
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (h prospectiveMatcherMatchCapabilityHandler) attachMatchingAnchors(ctx context.Context, corpusScope, description string, matches []prospectivePatternMatch) {
	if h.retrievalDB == nil || len(matches) == 0 {
		return
	}
	terms := scopeTokenPattern.FindAllString(description, -1)
	if len(terms) == 0 {
		return
	}
	anchors, err := retrieval.AnchorsForTerms(ctx, h.retrievalDB, terms, corpusScope)
	if err != nil || len(anchors) == 0 {
		return
	}
	for _, anchor := range anchors {
		if anchor.AnchorID == "" {
			continue
		}
		matches[0].AnchorRefs = appendIfMissing(matches[0].AnchorRefs, anchor.AnchorID)
	}
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func prospectiveMatchesAsAny(matches []prospectivePatternMatch) []any {
	out := make([]any, 0, len(matches))
	for _, match := range matches {
		out = append(out, map[string]any{
			"pattern_id":    match.PatternID,
			"title":         match.Title,
			"kind":          match.Kind,
			"description":   match.Description,
			"relevance":     match.Relevance,
			"corpus_source": match.CorpusSource,
			"anchor_refs":   append([]string(nil), match.AnchorRefs...),
		})
	}
	return out
}

func floatArg(raw any, defaultValue float64) float64 {
	switch typed := raw.(type) {
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		value, err := typed.Float64()
		if err == nil {
			return value
		}
	}
	return defaultValue
}
