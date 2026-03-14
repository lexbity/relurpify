package blackboard

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type blackboardScopedMemoryRetriever struct {
	store       memory.MemoryStore
	scope       memory.MemoryScope
	memoryClass core.MemoryClass
}

type blackboardRetrievalServiceProvider interface {
	RetrievalService() retrieval.RetrieverService
}

func (r blackboardScopedMemoryRetriever) Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error) {
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
			if len(records) == 0 && strings.TrimSpace(query) != "" {
				records, err = store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
					Scope: r.scope,
					Limit: limit,
				})
				if err != nil {
					return nil, err
				}
			}
			if len(records) > 0 {
				return blackboardDeclarativeEnvelopes(records), nil
			}
		}
		if provider, ok := r.store.(blackboardRetrievalServiceProvider); ok {
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
				if envelopes := blackboardRetrievalEnvelopes(blocks, r.memoryClass, r.scope); len(envelopes) > 0 {
					return envelopes, nil
				}
			}
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
			if len(records) == 0 && strings.TrimSpace(query) != "" {
				records, err = store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
					Scope: r.scope,
					Limit: limit,
				})
				if err != nil {
					return nil, err
				}
			}
			return blackboardProceduralEnvelopes(records), nil
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
			summary = fmt.Sprint(raw)
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         record.Key,
			MemoryClass: r.memoryClass,
			Scope:       string(record.Scope),
			Summary:     strings.TrimSpace(summary),
		})
	}
	return out, nil
}

func blackboardRuntimeStore(store memory.MemoryStore) graph.RuntimePersistenceStore {
	if runtimeStore, ok := store.(memory.RuntimeMemoryStore); ok {
		return memory.AdaptRuntimeStoreForGraph(runtimeStore)
	}
	return nil
}

func hydrateBlackboardFromMemory(state *core.Context, bb *Blackboard) int {
	if state == nil || bb == nil {
		return 0
	}
	added := 0
	if raw, ok := state.Get("graph.declarative_memory"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]core.MemoryRecordEnvelope); ok {
				for _, record := range results {
					if bb.AddFact("memory:"+record.Key, strings.TrimSpace(record.Summary), "memory:declarative") {
						added++
					}
				}
			}
		}
	}
	if raw, ok := state.Get("graph.procedural_memory"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]core.MemoryRecordEnvelope); ok {
				for _, record := range results {
					id := "memory:routine:" + strings.TrimSpace(record.Key)
					description := strings.TrimSpace(record.Summary)
					if description == "" {
						description = "consider learned routine " + strings.TrimSpace(record.Key)
					}
					if err := bb.AddHypothesis(id, description, 0.55, "memory:procedural"); err == nil {
						added++
					}
				}
			}
		}
	}
	return added
}

func publishPersistenceCandidates(state *core.Context, bb *Blackboard, controller ControllerState, metrics Metrics) {
	if state == nil || bb == nil {
		return
	}
	goal := ""
	if len(bb.Goals) > 0 {
		goal = strings.TrimSpace(bb.Goals[0])
	}
	state.Set(contextKeyPersistenceSummary, map[string]any{
		"summary": summaryText(bb, metrics),
		"result": map[string]any{
			"goal":           goal,
			"termination":    controller.Termination,
			"last_source":    controller.LastSource,
			"goal_satisfied": controller.GoalSatisfied,
			"counts": map[string]any{
				"facts":      metrics.FactCount,
				"issues":     metrics.IssueCount,
				"pending":    metrics.PendingCount,
				"completed":  metrics.CompletedCount,
				"artifacts":  metrics.ArtifactCount,
				"verified":   metrics.VerifiedCount,
				"hypothesis": metrics.HypothesisCount,
			},
		},
		"verified": controller.GoalSatisfied,
	})

	decisionSummary := ""
	if controller.GoalSatisfied {
		decisionSummary = fmt.Sprintf("blackboard satisfied %q after %d cycles", goal, controller.Cycle)
	} else if strings.TrimSpace(controller.Termination) != "" {
		decisionSummary = fmt.Sprintf("blackboard terminated with %s after %d cycles", controller.Termination, controller.Cycle)
	}
	state.Set(contextKeyPersistenceDecision, map[string]any{
		"summary":  decisionSummary,
		"decision": map[string]any{"termination": controller.Termination, "cycle": controller.Cycle, "goal": goal},
		"verified": controller.GoalSatisfied,
	})

	routine := map[string]any{}
	if controller.GoalSatisfied && metrics.CompletedCount > 0 && metrics.VerifiedCount > 0 {
		routine = map[string]any{
			"name":        "blackboard execution routine",
			"summary":     fmt.Sprintf("Use the blackboard execution loop to satisfy %q with iterative analysis, planning, execution, and verification.", goal),
			"description": fmt.Sprintf("Successful blackboard run for %q terminated with %s after %d cycles.", goal, controller.Termination, controller.Cycle),
			"inline_body": "1. Gather durable and task-local facts.\n2. Analyze issues and formulate next actions.\n3. Execute queued actions through framework capabilities.\n4. Verify produced artifacts before completion.",
			"verified":    true,
		}
	}
	state.Set(contextKeyPersistenceRoutine, routine)
}

func blackboardRetrievalEnvelopes(blocks []core.ContentBlock, memoryClass core.MemoryClass, scope memory.MemoryScope) []core.MemoryRecordEnvelope {
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
		text := strings.TrimSpace(fmt.Sprint(payload["text"]))
		if text == "" {
			continue
		}
		key := strings.TrimSpace(fmt.Sprint(payload["id"]))
		if key == "" {
			key = "retrieval:" + text
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         key,
			MemoryClass: memoryClass,
			Scope:       string(scope),
			Summary:     summarizeMemoryText(text, 240),
		})
	}
	return out
}

func blackboardDeclarativeEnvelopes(records []memory.DeclarativeMemoryRecord) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		key := strings.TrimSpace(record.RecordID)
		if key == "" {
			key = firstNonEmptyMemoryString(record.Title, record.Summary)
		}
		summary := strings.TrimSpace(record.Summary)
		if summary == "" {
			summary = strings.TrimSpace(record.Title)
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         key,
			MemoryClass: core.MemoryClassDeclarative,
			Scope:       string(record.Scope),
			Summary:     summary,
		})
	}
	return out
}

func blackboardProceduralEnvelopes(records []memory.ProceduralMemoryRecord) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		key := strings.TrimSpace(record.RoutineID)
		if key == "" {
			key = firstNonEmptyMemoryString(record.Name, record.Summary)
		}
		summary := strings.TrimSpace(record.Summary)
		if summary == "" {
			summary = strings.TrimSpace(record.Name)
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         key,
			MemoryClass: core.MemoryClassProcedural,
			Scope:       string(record.Scope),
			Summary:     summary,
		})
	}
	return out
}

func summarizeMemoryText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func firstNonEmptyMemoryString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
