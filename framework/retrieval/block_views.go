package retrieval

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// MemoryEnvelopes converts retrieval evidence blocks into compact memory envelopes.
func MemoryEnvelopes(blocks []core.ContentBlock, memoryClass core.MemoryClass, scope string) []core.MemoryRecordEnvelope {
	// Convert mixed evidence results to memory envelopes
	results := MixedEvidenceResultsFromBlocks(blocks)
	envelopes := make([]core.MemoryRecordEnvelope, 0, len(results))
	for _, result := range results {
		summary := result.Summary
		if strings.TrimSpace(summary) == "" || summary == "<nil>" {
			summary = result.Text
			if len(summary) > 240 {
				summary = summary[:240] + "..."
			}
		}
		envelope := core.MemoryRecordEnvelope{
			Key: firstEnvelopeKey(map[string]any{
				"record_id": result.RecordID,
				"id":        result.RecordID,
				"reference": result.Reference,
				"citations": result.Citations,
			}),
			MemoryClass: memoryClass,
			Text:        result.Text,
			Summary:     summary,
			Scope:       scope,
			Source:      result.Source,
			RecordID:    result.RecordID,
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

func firstEnvelopeKey(payload map[string]any) string {
	rawRef, _ := payload["reference"].(map[string]any)
	for _, raw := range []any{
		payload["record_id"],
		payload["id"],
		rawRef["id"],
		rawRef["uri"],
	} {
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	if citations, ok := payload["citations"].([]PackedCitation); ok && len(citations) > 0 {
		for _, value := range []string{citations[0].DocID, citations[0].ChunkID, citations[0].CanonicalURI} {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}
