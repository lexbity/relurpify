package agentgraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
)

// ContextReferenceKind classifies a mixed-evidence reference produced by graph
// memory publication helpers.
type ContextReferenceKind string

const (
	ContextReferenceKindFile     ContextReferenceKind = "file"
	ContextReferenceKindSymbol   ContextReferenceKind = "symbol"
	ContextReferenceKindAnchor   ContextReferenceKind = "anchor"
	ContextReferenceKindMemory   ContextReferenceKind = "memory"
	ContextReferenceKindExternal ContextReferenceKind = "external"
)

// ContextReference captures a lightweight publication reference used by graph
// memory helpers.
type ContextReference struct {
	Kind     ContextReferenceKind
	ID       string
	URI      string
	Version  string
	Detail   string
	Metadata map[string]string
}

// ArtifactRecord captures a durable graph artifact without binding graph to a
// concrete persistence implementation.
type ArtifactRecord struct {
	ArtifactID   string
	Kind         string
	ContentType  string
	StorageKind  string
	Summary      string
	RawText      string
	RawSizeBytes int64
	Metadata     map[string]any
	CreatedAt    time.Time
}

// MemoryRetriever returns bounded, compact memory retrieval results.
type MemoryRetriever interface {
	Retrieve(ctx context.Context, query string, limit int) ([]relurpctx.MemoryRecordEnvelope, error)
}

// PublishedMemoryRetriever can return the richer graph publication shape
// directly once callers are ready for it.
type PublishedMemoryRetriever interface {
	RetrievePublication(ctx context.Context, query string, limit int) (*MemoryRetrievalPublication, error)
}

// MemoryRetrievalPublication is the richer retrieval publication contract used
// by graph memory nodes once mixed-evidence consumers exist.
type MemoryRetrievalPublication struct {
	Query      string
	Results    []relurpctx.MemoryRecordEnvelope
	References []relurpctx.MemoryReference
	Payload    map[string]any
	Refs       []ContextReference
}

// StateHydrator restores selected durable references into active state.
type StateHydrator interface {
	Hydrate(ctx context.Context, refs []string) (map[string]any, error)
}

// BuildMemoryRetrievalPublication derives the richer graph publication shape
// from legacy envelope results.
func BuildMemoryRetrievalPublication(query string, results []relurpctx.MemoryRecordEnvelope, expectedClass relurpctx.MemoryClass) *MemoryRetrievalPublication {
	if len(results) == 0 {
		return &MemoryRetrievalPublication{
			Query:      query,
			Results:    []relurpctx.MemoryRecordEnvelope{},
			References: []relurpctx.MemoryReference{},
			Payload:    nil,
			Refs:       nil,
		}
	}
	normalized := results
	needsNormalization := false
	for _, record := range results {
		if record.MemoryClass == "" {
			needsNormalization = true
			break
		}
	}
	if needsNormalization {
		normalized = append([]relurpctx.MemoryRecordEnvelope(nil), results...)
		for i := range normalized {
			if normalized[i].MemoryClass == "" {
				normalized[i].MemoryClass = expectedClass
			}
		}
	}
	references := make([]relurpctx.MemoryReference, 0, len(results))
	for _, record := range normalized {
		references = append(references, relurpctx.MemoryReference{
			MemoryClass: record.MemoryClass,
			Scope:       record.Scope,
			RecordKey:   record.Key,
			Summary:     record.Summary,
		})
	}
	return &MemoryRetrievalPublication{
		Query:      query,
		Results:    normalized,
		References: references,
		Payload:    mixedEvidencePayloadFromEnvelopes(query, normalized),
		Refs:       contextReferencesFromEnvelopes(normalized, expectedClass),
	}
}

func mixedEvidencePayloadFromEnvelopes(query string, results []relurpctx.MemoryRecordEnvelope) map[string]any {
	if len(results) == 0 {
		return nil
	}
	texts := make([]string, 0, len(results))
	entries := make([]map[string]any, 0, len(results))
	citationCount := 0
	for _, result := range results {
		summary := strings.TrimSpace(result.Summary)
		text := strings.TrimSpace(result.Text)
		if summary == "" && text == "" {
			continue
		}
		if summary == "" {
			summary = text
		}
		texts = append(texts, summary)
		entry := map[string]any{
			"summary": summary,
		}
		if text != "" {
			entry["text"] = text
		}
		if source := strings.TrimSpace(result.Source); source != "" {
			entry["source"] = source
		}
		if recordID := strings.TrimSpace(result.RecordID); recordID != "" {
			entry["record_id"] = recordID
		} else if key := strings.TrimSpace(result.Key); key != "" {
			entry["record_id"] = key
		}
		if kind := strings.TrimSpace(result.Kind); kind != "" {
			entry["kind"] = kind
		}
		if result.Reference != nil {
			entry["reference"] = result.Reference
		}
		switch typed := result.Citations.(type) {
		case []map[string]any:
			if len(typed) > 0 {
				entry["citations"] = typed
				citationCount += len(typed)
			}
		case []any:
			if len(typed) > 0 {
				entry["citations"] = typed
				citationCount += len(typed)
			}
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}
	return map[string]any{
		"query":          query,
		"texts":          texts,
		"results":        entries,
		"summary":        strings.Join(texts, "\n\n"),
		"result_size":    len(entries),
		"citation_count": citationCount,
	}
}

func contextReferencesFromEnvelopes(results []relurpctx.MemoryRecordEnvelope, expectedClass relurpctx.MemoryClass) []ContextReference {
	if len(results) == 0 {
		return nil
	}
	refs := make([]ContextReference, 0, len(results))
	seen := make(map[contextReferenceKey]struct{}, len(results))
	for _, result := range results {
		ref := contextReferenceFromEnvelope(result, expectedClass)
		if ref == nil {
			continue
		}
		key := contextReferenceKey{kind: ref.Kind, id: ref.ID, uri: ref.URI}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, *ref)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

type contextReferenceKey struct {
	kind ContextReferenceKind
	id   string
	uri  string
}

func contextReferenceFromEnvelope(result relurpctx.MemoryRecordEnvelope, expectedClass relurpctx.MemoryClass) *ContextReference {
	values, ok := result.Reference.(map[string]any)
	if ok && len(values) > 0 {
		ref := &ContextReference{
			Kind:    ContextReferenceKind(trimmedAnyString(values["kind"])),
			ID:      trimmedAnyString(values["id"]),
			URI:     trimmedAnyString(values["uri"]),
			Version: trimmedAnyString(values["version"]),
			Detail:  trimmedAnyString(values["detail"]),
		}
		if ref.Kind == "" {
			ref.Kind = defaultContextReferenceKind(result, expectedClass)
		}
		recordID := trimmedAnyString(values["record_id"])
		source := trimmedAnyString(values["source"])
		kind := trimmedAnyString(values["kind"])
		if recordID != "" || source != "" || kind != "" {
			metadata := make(map[string]string, 3)
			if recordID != "" {
				metadata["record_id"] = recordID
			}
			if source != "" {
				metadata["source"] = source
			}
			if kind != "" {
				metadata["kind"] = kind
			}
			ref.Metadata = metadata
		}
		if ref.ID != "" || ref.URI != "" {
			return ref
		}
	}
	key := strings.TrimSpace(result.RecordID)
	if key == "" {
		key = strings.TrimSpace(result.Key)
	}
	if key == "" {
		return nil
	}
	return &ContextReference{
		Kind:   defaultContextReferenceKind(result, expectedClass),
		ID:     key,
		Detail: strings.TrimSpace(result.Kind),
		Metadata: map[string]string{
			"memory_class": string(nonEmptyMemoryClass(result.MemoryClass, expectedClass)),
			"source":       strings.TrimSpace(result.Source),
		},
	}
}

func defaultContextReferenceKind(result relurpctx.MemoryRecordEnvelope, expectedClass relurpctx.MemoryClass) ContextReferenceKind {
	if strings.TrimSpace(result.Source) == "retrieval" {
		return ContextReferenceKind("retrieval_evidence")
	}
	return ContextReferenceKind("runtime_memory")
}

func nonEmptyMemoryClass(class relurpctx.MemoryClass, fallback relurpctx.MemoryClass) relurpctx.MemoryClass {
	if strings.TrimSpace(string(class)) != "" {
		return class
	}
	return fallback
}

func trimmedAnyString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		formatted := strings.TrimSpace(fmt.Sprint(typed))
		if formatted == "<nil>" {
			return ""
		}
		return formatted
	}
}
