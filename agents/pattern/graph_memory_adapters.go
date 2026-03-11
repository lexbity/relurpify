package pattern

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

type scopedMemoryRetriever struct {
	store       memory.MemoryStore
	scope       memory.MemoryScope
	memoryClass core.MemoryClass
}

func (r scopedMemoryRetriever) Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error) {
	if r.store == nil {
		return nil, nil
	}
	switch r.memoryClass {
	case core.MemoryClassDeclarative:
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
