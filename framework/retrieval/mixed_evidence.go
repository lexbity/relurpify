package retrieval

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

type MixedEvidenceResult struct {
	Text       string                `json:"text"`
	Summary    string                `json:"summary,omitempty"`
	Source     string                `json:"source,omitempty"`
	RecordID   string                `json:"record_id,omitempty"`
	Kind       string                `json:"kind,omitempty"`
	Reference  map[string]any        `json:"reference,omitempty"`
	Citations  []PackedCitation      `json:"citations,omitempty"`
	Anchors    []AnchorRef           `json:"anchors,omitempty"` // Semantic anchors from evidence
	Derivation *core.DerivationChain `json:"derivation,omitempty"`
	score      float64
	order      int
}

// SupplementalEvidenceRecord is a generic side-evidence input that can be ranked
// together with retrieval blocks regardless of its backing store.
type SupplementalEvidenceRecord struct {
	Text       string
	Summary    string
	Source     string
	RecordID   string
	Kind       string
	Reference  map[string]any
	ScoreBoost float64
}

// MixedEvidenceResultsFromBlocks converts retrieval blocks into ranked mixed-evidence results.
func MixedEvidenceResultsFromBlocks(blocks []core.ContentBlock) []MixedEvidenceResult {
	results := make([]MixedEvidenceResult, 0, len(blocks))
	for idx, block := range blocks {
		switch typed := block.(type) {
		case core.TextContentBlock:
			if text := strings.TrimSpace(typed.Text); text != "" {
				origin := core.OriginDerivation("retrieval")
				results = append(results, MixedEvidenceResult{
					Text:       text,
					Source:     "retrieval",
					Derivation: &origin,
					score:      mixedEvidenceRankScore(idx),
					order:      idx,
				})
			}
		case core.StructuredContentBlock:
			payload, ok := typed.Data.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(toString(payload["text"]))
			if text == "" || text == "<nil>" {
				continue
			}
			// Extract derivation from payload if available
			var derivation *core.DerivationChain
			if _, ok := payload["derivation"].(map[string]interface{}); ok {
				// Derivation summary exists in payload, create origin derivation
				origin := core.OriginDerivation("retrieval")
				derivation = &origin
			} else {
				origin := core.OriginDerivation("retrieval")
				derivation = &origin
			}

			result := MixedEvidenceResult{
				Text:       text,
				Summary:    strings.TrimSpace(toString(payload["summary"])),
				Source:     "retrieval",
				Reference:  cloneMapAny(payload["reference"]),
				Citations:  mixedEvidenceCitations(payload["citations"]),
				Derivation: derivation,
				score:      mixedEvidenceRankScore(idx),
				order:      idx,
			}
			results = append(results, result)
		}
	}
	return results
}

// BuildMixedEvidenceResults score-orders retrieval evidence and workflow knowledge together.
func BuildMixedEvidenceResults(queryText string, blocks []core.ContentBlock, records []memory.KnowledgeEntry) []MixedEvidenceResult {
	supplemental := make([]SupplementalEvidenceRecord, 0, len(records))
	for _, rec := range records {
		supplemental = append(supplemental, SupplementalEvidenceRecord{
			Text:       rec.Content,
			Summary:    rec.Title,
			Source:     "workflow_knowledge",
			RecordID:   rec.RecordID,
			Kind:       string(rec.Kind),
			ScoreBoost: 0.2,
		})
	}
	return BuildMixedEvidenceResultsWithSupplemental(queryText, blocks, supplemental)
}

// BuildMixedEvidenceResultsWithSupplemental score-orders retrieval evidence and supplemental evidence together.
func BuildMixedEvidenceResultsWithSupplemental(queryText string, blocks []core.ContentBlock, records []SupplementalEvidenceRecord) []MixedEvidenceResult {
	results := MixedEvidenceResultsFromBlocks(blocks)
	if len(records) == 0 {
		return results
	}
	seen := make(map[string]struct{}, len(results)+len(records))
	for _, result := range results {
		if key := mixedEvidenceDedupeKey(result); key != "" {
			seen[key] = struct{}{}
		}
	}
	out := make([]MixedEvidenceResult, 0, len(results)+len(records))
	out = append(out, results...)
	for _, rec := range records {
		text := strings.TrimSpace(rec.Text)
		if text == "" {
			continue
		}

		// Determine origin based on source
		var derivation *core.DerivationChain
		if rec.Source == "runtime_memory" {
			origin := core.OriginDerivation("memory")
			derived := origin.Derive("memory_recall", "memory", 0.0, "")
			derivation = &derived
		} else {
			origin := core.OriginDerivation(rec.Source)
			derivation = &origin
		}

		queryBoost := queryTermRecall(queryText, text+" "+rec.Summary)
		result := MixedEvidenceResult{
			Text:       text,
			Summary:    supplementalSummary(rec),
			Source:     rec.Source,
			RecordID:   rec.RecordID,
			Kind:       rec.Kind,
			Reference:  cloneMapAny(rec.Reference),
			Derivation: derivation,
			score:      supplementalScore(rec) + queryBoost,
			order:      len(out),
		}
		key := mixedEvidenceDedupeKey(result)
		if _, alreadySeen := seen[key]; key != "" && alreadySeen {
			continue
		}
		if key != "" {
			seen[key] = struct{}{}
		}
		out = append(out, result)
	}

	sortMixedEvidenceResults(out)
	return out
}

// MixedEvidenceMemoryEnvelopes converts runtime memory records into mixed-evidence results
// suitable for inclusion in LLM prompts without requiring live database access.
func MixedEvidenceMemoryEnvelopes(records []memory.KnowledgeEntry) []MixedEvidenceResult {
	return BuildMixedEvidenceResults("", nil, records)
}

// ReconstructReferenceFromMixedResult restores a full ContextReference from mixed evidence.
func ReconstructReferenceFromMixedResult(result MixedEvidenceResult) core.ContextReference {
	if result.Reference == nil {
		return core.ContextReference{}
	}
	return core.ContextReference{
		Kind:    core.ContextReferenceKind(toString(result.Reference["kind"])),
		ID:      toString(result.Reference["id"]),
		URI:     toString(result.Reference["uri"]),
		Version: toString(result.Reference["version"]),
		Detail:  toString(result.Reference["detail"]),
		Metadata: func() map[string]string {
			md := make(map[string]string)
			if structural, ok := result.Reference["structural"].(string); ok && structural != "" {
				md["structural"] = structural
			}
			if chunkIDs, ok := result.Reference["chunk_ids"].(string); ok && chunkIDs != "" {
				md["chunk_ids"] = chunkIDs
			}
			if len(md) == 0 {
				return nil
			}
			return md
		}(),
	}
}

func mixedEvidenceCitations(raw any) []PackedCitation {
	if raw == nil {
		return nil
	}
	typed, ok := raw.([]PackedCitation)
	if ok {
		return typed
	}
	// Try to decode from []any (JSON unmarshaling)
	citations, ok := raw.([]any)
	if !ok || len(citations) == 0 {
		return nil
	}
	out := make([]PackedCitation, 0, len(citations))
	for _, citation := range citations {
		if c, ok := citation.(map[string]any); ok {
			out = append(out, PackedCitation{
				DocID:        toString(c["doc_id"]),
				ChunkID:      toString(c["chunk_id"]),
				VersionID:    toString(c["version_id"]),
				CanonicalURI: toString(c["canonical_uri"]),
				SourceType:   toString(c["source_type"]),
				StartOffset:  toInt(c["start_offset"]),
				EndOffset:    toInt(c["end_offset"]),
			})
		}
	}
	return out
}

// queryTermRecall returns the fraction of query terms present in the candidate text.
// Returns 1.0 if all query terms appear, 0.0 if none do.
func queryTermRecall(queryText, candidateText string) float64 {
	queryTokens := tokenizeWords(queryText)
	if len(queryTokens) == 0 {
		return 0.0
	}
	candidateLower := strings.ToLower(candidateText)
	matched := 0
	for _, term := range queryTokens {
		if strings.Contains(candidateLower, term) {
			matched++
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func supplementalScore(rec SupplementalEvidenceRecord) float64 {
	if rec.ScoreBoost > 0 {
		return rec.ScoreBoost
	}
	return 0.5
}

func supplementalSummary(rec SupplementalEvidenceRecord) string {
	if rec.Summary != "" {
		return rec.Summary
	}
	text := strings.TrimSpace(rec.Text)
	if len(text) > 240 {
		return text[:240] + "..."
	}
	return text
}

func supplementalReferenceTerms(ref map[string]any) map[string]bool {
	terms := make(map[string]bool)
	if ref == nil || len(ref) == 0 {
		return terms
	}

	// Standard keys
	field := toString(ref["field"])
	if field != "" {
		terms[field] = true
	}
	field = toString(ref["key"])
	if field != "" {
		terms[field] = true
	}
	field = toString(ref["id"])
	if field != "" {
		terms[field] = true
	}

	return terms
}

func mixedEvidenceDedupeKey(result MixedEvidenceResult) string {
	field := result.RecordID
	if field != "" {
		return field
	}
	return ""
}

func mixedEvidenceRankScore(index int) float64 {
	// Higher index = lower score
	return float64(1.0 / (1.0 + float64(index)))
}

func sortMixedEvidenceResults(results []MixedEvidenceResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			if results[i].Source == results[j].Source {
				return results[i].order < results[j].order
			}
			return results[i].Source == "retrieval"
		}
		return results[i].score > results[j].score
	})
}

func cloneMapAny(raw any) map[string]any {
	src, _ := raw.(map[string]any)
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

// toInt safely converts any to int.
func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch typed := v.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

// BuildMixedEvidencePayload creates a payload for mixed evidence results suitable for task context.
func BuildMixedEvidencePayload(queryText, scope string, event any, results []MixedEvidenceResult) map[string]any {
	resultPayloads := make([]map[string]any, 0, len(results))
	// texts holds per-result labels (Summary preferred); summaryParts holds full content (Text preferred).
	texts := make([]string, 0, len(results))
	summaryParts := make([]string, 0, len(results))
	citationCount := 0
	for _, result := range results {
		resultPayloads = append(resultPayloads, map[string]any{
			"text":       result.Text,
			"summary":    result.Summary,
			"source":     result.Source,
			"record_id":  result.RecordID,
			"kind":       result.Kind,
			"reference":  result.Reference,
			"citations":  result.Citations,
			"anchors":    result.Anchors,
			"derivation": result.Derivation,
		})
		// texts: use Summary (title/label) as the short identifier, fall back to Text
		labelEntry := strings.TrimSpace(result.Summary)
		if labelEntry == "" || labelEntry == "<nil>" {
			labelEntry = strings.TrimSpace(result.Text)
		}
		if labelEntry != "" && labelEntry != "<nil>" {
			texts = append(texts, labelEntry)
		}
		// summary: use "Summary: Text" when both are set; otherwise fall back.
		label := strings.TrimSpace(result.Summary)
		contentEntry := strings.TrimSpace(result.Text)
		if contentEntry == "" || contentEntry == "<nil>" {
			contentEntry = label
		} else if label != "" && label != "<nil>" && label != contentEntry {
			contentEntry = label + ": " + contentEntry
		}
		if contentEntry != "" && contentEntry != "<nil>" {
			summaryParts = append(summaryParts, contentEntry)
		}
		citationCount += len(result.Citations)
	}

	// Build combined summary from full-text content entries
	summary := strings.Join(summaryParts, "\n\n")

	payload := map[string]any{
		"query":          queryText,
		"scope":          scope,
		"results":        resultPayloads,
		"citation_count": citationCount,
	}
	if summary != "" {
		payload["summary"] = summary
	}
	if len(texts) > 0 {
		payload["texts"] = texts
	}

	// Add event-specific fields if provided
	switch typedEvent := event.(type) {
	case RetrievalEvent:
		if typedEvent.QueryID != "" {
			payload["query_id"] = typedEvent.QueryID
		}
		if typedEvent.CacheTier != "" {
			payload["cache_tier"] = typedEvent.CacheTier
		}
	case map[string]any:
		for k, v := range typedEvent {
			payload[k] = v
		}
	}

	return payload
}

// MixedEvidencePayloadFromEnvelopes reconstructs a mixed evidence payload from memory record envelopes.
// This is the inverse of BuildMixedEvidencePayload, converting envelopes back to result format.
func MixedEvidencePayloadFromEnvelopes(queryText, scope string, envelopes []core.MemoryRecordEnvelope) map[string]any {
	resultPayloads := make([]map[string]any, 0, len(envelopes))
	for _, envelope := range envelopes {
		citations := make([]map[string]any, 0)
		if citationList, ok := envelope.Citations.([]map[string]any); ok {
			citations = citationList
		} else if citationList, ok := envelope.Citations.([]PackedCitation); ok {
			for _, c := range citationList {
				citations = append(citations, map[string]any{
					"doc_id":        c.DocID,
					"chunk_id":      c.ChunkID,
					"canonical_uri": c.CanonicalURI,
				})
			}
		}

		resultPayloads = append(resultPayloads, map[string]any{
			"text":      envelope.Text,
			"summary":   envelope.Summary,
			"source":    envelope.Source,
			"record_id": envelope.RecordID,
			"kind":      envelope.Kind,
			"reference": envelope.Reference,
			"citations": citations,
		})
	}
	return map[string]any{
		"query":   queryText,
		"scope":   scope,
		"results": resultPayloads,
	}
}

// SupplementalEvidenceFromDeclarativeMemory converts declarative memory records into supplemental evidence.
func SupplementalEvidenceFromDeclarativeMemory(records []memory.DeclarativeMemoryEntry) []SupplementalEvidenceRecord {
	result := make([]SupplementalEvidenceRecord, 0, len(records))
	for _, rec := range records {
		scoreBoost := 0.1
		if rec.Verified {
			scoreBoost = 0.3
		}
		result = append(result, SupplementalEvidenceRecord{
			Text:     rec.Content,
			Summary:  rec.Summary,
			Source:   "runtime_memory",
			RecordID: rec.RecordID,
			Kind:     string(rec.Kind),
			Reference: map[string]any{
				"uri": rec.ArtifactRef,
			},
			ScoreBoost: scoreBoost,
		})
	}
	return result
}

// SupplementalEvidenceFromProceduralMemory converts procedural memory records into supplemental evidence.
func SupplementalEvidenceFromProceduralMemory(records []memory.ProceduralMemoryEntry) []SupplementalEvidenceRecord {
	result := make([]SupplementalEvidenceRecord, 0, len(records))
	for _, rec := range records {
		scoreBoost := 0.15
		if rec.Verified {
			scoreBoost = 0.35
		}
		if rec.ReuseCount > 2 {
			scoreBoost += 0.1
		}
		result = append(result, SupplementalEvidenceRecord{
			Text:     rec.Description,
			Summary:  rec.Summary,
			Source:   "runtime_memory",
			RecordID: rec.RoutineID,
			Kind:     string(rec.Kind),
			Reference: map[string]any{
				"uri": rec.BodyRef,
			},
			ScoreBoost: scoreBoost,
		})
	}
	return result
}
