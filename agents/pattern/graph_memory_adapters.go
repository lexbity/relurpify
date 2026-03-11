package pattern

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type scopedMemoryRetriever struct {
	store       memory.MemoryStore
	scope       memory.MemoryScope
	memoryClass core.MemoryClass
}

type retrievalServiceProvider interface {
	RetrievalService() retrieval.RetrieverService
}

func (r scopedMemoryRetriever) Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error) {
	if r.store == nil {
		return nil, nil
	}
	switch r.memoryClass {
	case core.MemoryClassDeclarative:
		if provider, ok := r.store.(retrievalServiceProvider); ok {
			if service := provider.RetrievalService(); service != nil {
				blocks, _, err := service.Retrieve(ctx, retrieval.RetrievalQuery{
					Text:      query,
					Scope:     string(r.scope),
					MaxTokens: 400,
					Limit:     limit,
				})
				if err != nil {
					return nil, err
				}
				if envelopes := retrievalEnvelopes(blocks, r.memoryClass, r.scope); len(envelopes) > 0 {
					return envelopes, nil
				}
			}
		}
		if store, ok := r.store.(memory.DeclarativeMemoryStore); ok {
			records, err := store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
				Query: query,
				Scope: r.scope,
				Limit: limit,
			})
			if err != nil {
				return nil, err
			}
			return declarativeEnvelopes(records), nil
		}
	case core.MemoryClassProcedural:
		if store, ok := r.store.(memory.ProceduralMemoryStore); ok {
			records, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
				Query: query,
				Scope: r.scope,
				Limit: limit,
			})
			if err != nil {
				return nil, err
			}
			return proceduralEnvelopes(records), nil
		}
	}
	records, err := r.store.Search(ctx, query, r.scope)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		summary := ""
		if raw, ok := record.Value["summary"]; ok && raw != nil {
			summary = stringify(raw)
		} else if raw, ok := record.Value["type"]; ok && raw != nil {
			summary = stringify(raw)
		}
		envelope := core.MemoryRecordEnvelope{
			Key:         record.Key,
			MemoryClass: r.memoryClass,
			Scope:       string(record.Scope),
			Summary:     summary,
		}
		if raw, ok := record.Value["memory_class"]; ok && raw != nil {
			switch core.MemoryClass(stringify(raw)) {
			case core.MemoryClassWorking, core.MemoryClassDeclarative, core.MemoryClassProcedural:
				envelope.MemoryClass = core.MemoryClass(stringify(raw))
			}
		}
		out = append(out, envelope)
	}
	return out, nil
}

func retrievalEnvelopes(blocks []core.ContentBlock, memoryClass core.MemoryClass, scope memory.MemoryScope) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, len(blocks))
	for _, block := range blocks {
		structured, ok := block.(core.StructuredContentBlock)
		if !ok {
			continue
		}
		payload, ok := structured.Data.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(stringify(payload["text"]))
		if text == "" {
			continue
		}
		key := ""
		if citations, ok := payload["citations"].([]retrieval.PackedCitation); ok && len(citations) > 0 {
			key = firstNonEmpty(citations[0].DocID, citations[0].ChunkID, citations[0].CanonicalURI)
		}
		if key == "" {
			key = "retrieval:" + strings.TrimSpace(text)
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         key,
			MemoryClass: memoryClass,
			Scope:       string(scope),
			Summary:     summarizeText(text, 240),
		})
	}
	return out
}

func declarativeEnvelopes(records []memory.DeclarativeMemoryRecord) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		summary := record.Summary
		if summary == "" {
			summary = record.Title
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         record.RecordID,
			MemoryClass: core.MemoryClassDeclarative,
			Scope:       string(record.Scope),
			Summary:     summary,
		})
	}
	return out
}

func proceduralEnvelopes(records []memory.ProceduralMemoryRecord) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		summary := record.Summary
		if summary == "" {
			summary = record.Name
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         record.RoutineID,
			MemoryClass: core.MemoryClassProcedural,
			Scope:       string(record.Scope),
			Summary:     summary,
		})
	}
	return out
}

func stringify(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func summarizeText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}
