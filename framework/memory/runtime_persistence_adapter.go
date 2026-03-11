package memory

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type graphRuntimePersistenceAdapter struct {
	store RuntimeMemoryStore
}

// AdaptRuntimeStoreForGraph bridges a structured runtime memory store into the
// graph-local persistence interfaces without introducing a package cycle.
func AdaptRuntimeStoreForGraph(store RuntimeMemoryStore) graph.RuntimePersistenceStore {
	if store == nil {
		return nil
	}
	return graphRuntimePersistenceAdapter{store: store}
}

func (a graphRuntimePersistenceAdapter) PutDeclarative(ctx context.Context, record graph.DeclarativeRecord) error {
	return a.store.PutDeclarative(ctx, DeclarativeMemoryRecord{
		RecordID:    record.RecordID,
		Scope:       MemoryScope(record.Scope),
		Kind:        DeclarativeMemoryKind(record.Kind),
		Title:       record.Title,
		Content:     record.Content,
		Summary:     record.Summary,
		TaskID:      record.TaskID,
		WorkflowID:  record.WorkflowID,
		ArtifactRef: record.ArtifactRef,
		Tags:        append([]string{}, record.Tags...),
		Metadata:    cloneMapAny(record.Metadata),
		Verified:    record.Verified,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	})
}

func (a graphRuntimePersistenceAdapter) SearchDeclarative(ctx context.Context, query graph.DeclarativeQuery) ([]graph.DeclarativeRecord, error) {
	kinds := make([]DeclarativeMemoryKind, 0, len(query.Kinds))
	for _, kind := range query.Kinds {
		kinds = append(kinds, DeclarativeMemoryKind(kind))
	}
	records, err := a.store.SearchDeclarative(ctx, DeclarativeMemoryQuery{
		Query:  query.Query,
		Scope:  MemoryScope(query.Scope),
		Kinds:  kinds,
		TaskID: query.TaskID,
		Limit:  query.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]graph.DeclarativeRecord, 0, len(records))
	for _, record := range records {
		out = append(out, graph.DeclarativeRecord{
			RecordID:    record.RecordID,
			Scope:       string(record.Scope),
			Kind:        graph.DeclarativeKind(record.Kind),
			Title:       record.Title,
			Content:     record.Content,
			Summary:     record.Summary,
			TaskID:      record.TaskID,
			WorkflowID:  record.WorkflowID,
			ArtifactRef: record.ArtifactRef,
			Tags:        append([]string{}, record.Tags...),
			Metadata:    cloneMapAny(record.Metadata),
			Verified:    record.Verified,
			CreatedAt:   record.CreatedAt,
			UpdatedAt:   record.UpdatedAt,
		})
	}
	return out, nil
}

func (a graphRuntimePersistenceAdapter) PutProcedural(ctx context.Context, record graph.ProceduralRecord) error {
	return a.store.PutProcedural(ctx, ProceduralMemoryRecord{
		RoutineID:              record.RoutineID,
		Scope:                  MemoryScope(record.Scope),
		Kind:                   ProceduralMemoryKind(record.Kind),
		Name:                   record.Name,
		Description:            record.Description,
		Summary:                record.Summary,
		TaskID:                 record.TaskID,
		WorkflowID:             record.WorkflowID,
		BodyRef:                record.BodyRef,
		InlineBody:             record.InlineBody,
		CapabilityDependencies: append([]core.CapabilitySelector{}, record.CapabilityDependencies...),
		VerificationMetadata:   cloneMapAny(record.VerificationMetadata),
		PolicySnapshotID:       record.PolicySnapshotID,
		Verified:               record.Verified,
		Version:                record.Version,
		ReuseCount:             record.ReuseCount,
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	})
}

func (a graphRuntimePersistenceAdapter) SearchProcedural(ctx context.Context, query graph.ProceduralQuery) ([]graph.ProceduralRecord, error) {
	kinds := make([]ProceduralMemoryKind, 0, len(query.Kinds))
	for _, kind := range query.Kinds {
		kinds = append(kinds, ProceduralMemoryKind(kind))
	}
	records, err := a.store.SearchProcedural(ctx, ProceduralMemoryQuery{
		Query:          query.Query,
		Scope:          MemoryScope(query.Scope),
		Kinds:          kinds,
		TaskID:         query.TaskID,
		CapabilityName: query.CapabilityName,
		Limit:          query.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]graph.ProceduralRecord, 0, len(records))
	for _, record := range records {
		out = append(out, graph.ProceduralRecord{
			RoutineID:              record.RoutineID,
			Scope:                  string(record.Scope),
			Kind:                   graph.ProceduralKind(record.Kind),
			Name:                   record.Name,
			Description:            record.Description,
			Summary:                record.Summary,
			TaskID:                 record.TaskID,
			WorkflowID:             record.WorkflowID,
			BodyRef:                record.BodyRef,
			InlineBody:             record.InlineBody,
			CapabilityDependencies: append([]core.CapabilitySelector{}, record.CapabilityDependencies...),
			VerificationMetadata:   cloneMapAny(record.VerificationMetadata),
			PolicySnapshotID:       record.PolicySnapshotID,
			Verified:               record.Verified,
			Version:                record.Version,
			ReuseCount:             record.ReuseCount,
			CreatedAt:              record.CreatedAt,
			UpdatedAt:              record.UpdatedAt,
		})
	}
	return out, nil
}

func cloneMapAny(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
