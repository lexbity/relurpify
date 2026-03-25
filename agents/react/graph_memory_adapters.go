package react

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
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
	publication, err := r.RetrievePublication(ctx, query, limit)
	if err != nil || publication == nil {
		return nil, err
	}
	return publication.Results, nil
}

func (r scopedMemoryRetriever) RetrievePublication(ctx context.Context, query string, limit int) (*graph.MemoryRetrievalPublication, error) {
	if r.store == nil {
		return graph.BuildMemoryRetrievalPublication(strings.TrimSpace(query), nil, r.memoryClass), nil
	}
	switch r.memoryClass {
	case core.MemoryClassDeclarative:
		var blocks []core.ContentBlock
		if provider, ok := r.store.(retrievalServiceProvider); ok {
			if service := provider.RetrievalService(); service != nil {
				retrieved, _, err := service.Retrieve(ctx, retrieval.RetrievalQuery{
					Text:      query,
					Scope:     string(r.scope),
					MaxTokens: 400,
					Limit:     limit,
				})
				if err != nil {
					return nil, err
				}
				blocks = retrieved
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
			// Start with retrieval results (if available) - retrieval is preferred
			out := make([]core.MemoryRecordEnvelope, 0)
			if len(blocks) > 0 {
				if retrievalEnvelopes := retrieval.MemoryEnvelopes(blocks, r.memoryClass, string(r.scope)); len(retrievalEnvelopes) > 0 {
					out = append(out, retrievalEnvelopes...)
				}
			}
			// Then add memory store results
			for _, record := range records {
				out = append(out, core.MemoryRecordEnvelope{
					Key:         record.RecordID,
					MemoryClass: r.memoryClass,
					Text:        record.Content,
					Summary:     record.Summary,
					Scope:       string(record.Scope),
					Source:      "declarative",
					RecordID:    record.RecordID,
				})
			}
			if len(out) > 0 {
				return graph.BuildMemoryRetrievalPublication(query, out, r.memoryClass), nil
			}
			return graph.BuildMemoryRetrievalPublication(query, nil, r.memoryClass), nil
		}
		if envelopes := retrieval.MemoryEnvelopes(blocks, r.memoryClass, string(r.scope)); len(envelopes) > 0 {
			return graph.BuildMemoryRetrievalPublication(query, envelopes, r.memoryClass), nil
		}
	case core.MemoryClassProcedural:
		var blocks []core.ContentBlock
		if provider, ok := r.store.(retrievalServiceProvider); ok {
			if service := provider.RetrievalService(); service != nil {
				retrieved, _, err := service.Retrieve(ctx, retrieval.RetrievalQuery{
					Text:      query,
					Scope:     string(r.scope),
					MaxTokens: 400,
					Limit:     limit,
				})
				if err != nil {
					return nil, err
				}
				blocks = retrieved
			}
		}
		if store, ok := r.store.(memory.ProceduralMemoryStore); ok {
			records, err := store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
				Query: query,
				Scope: r.scope,
				Limit: limit,
			})
			if err != nil {
				return nil, err
			}
			// Start with retrieval results (if available) - retrieval is preferred
			out := make([]core.MemoryRecordEnvelope, 0)
			if len(blocks) > 0 {
				if retrievalEnvelopes := retrieval.MemoryEnvelopes(blocks, r.memoryClass, string(r.scope)); len(retrievalEnvelopes) > 0 {
					out = append(out, retrievalEnvelopes...)
				}
			}
			// Then add memory store results
			for _, record := range records {
				out = append(out, core.MemoryRecordEnvelope{
					Key:         record.RoutineID,
					MemoryClass: r.memoryClass,
					Text:        record.Description,
					Summary:     record.Summary,
					Scope:       string(record.Scope),
					Source:      "procedural",
					RecordID:    record.RoutineID,
				})
			}
			if len(out) > 0 {
				return graph.BuildMemoryRetrievalPublication(query, out, r.memoryClass), nil
			}
			return graph.BuildMemoryRetrievalPublication(query, nil, r.memoryClass), nil
		}
		if envelopes := retrieval.MemoryEnvelopes(blocks, r.memoryClass, string(r.scope)); len(envelopes) > 0 {
			return graph.BuildMemoryRetrievalPublication(query, envelopes, r.memoryClass), nil
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
	return graph.BuildMemoryRetrievalPublication(query, out, r.memoryClass), nil
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
