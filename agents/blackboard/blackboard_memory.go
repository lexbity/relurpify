package blackboard

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	publication, err := r.RetrievePublication(ctx, query, limit)
	if err != nil || publication == nil {
		return nil, err
	}
	return publication.Results, nil
}

func (r blackboardScopedMemoryRetriever) RetrievePublication(ctx context.Context, query string, limit int) (*graph.MemoryRetrievalPublication, error) {
	if r.store == nil {
		return graph.BuildMemoryRetrievalPublication(strings.TrimSpace(query), nil, r.memoryClass), nil
	}
	switch r.memoryClass {
	case core.MemoryClassDeclarative:
		var blocks []core.ContentBlock
		if provider, ok := r.store.(blackboardRetrievalServiceProvider); ok {
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
			if len(records) == 0 && strings.TrimSpace(query) != "" {
				records, err = store.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
					Scope: r.scope,
					Limit: limit,
				})
				if err != nil {
					return nil, err
				}
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
		if provider, ok := r.store.(blackboardRetrievalServiceProvider); ok {
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
			if len(records) == 0 && strings.TrimSpace(query) != "" {
				records, err = store.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
					Scope: r.scope,
					Limit: limit,
				})
				if err != nil {
					return nil, err
				}
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
			summary = fmt.Sprint(raw)
		}
		out = append(out, core.MemoryRecordEnvelope{
			Key:         record.Key,
			MemoryClass: r.memoryClass,
			Scope:       string(record.Scope),
			Summary:     strings.TrimSpace(summary),
		})
	}
	return graph.BuildMemoryRetrievalPublication(query, out, r.memoryClass), nil
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
	if raw, ok := state.Get("graph.declarative_memory_payload"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]map[string]any); ok {
				for _, record := range results {
					key := strings.TrimSpace(fmt.Sprint(record["record_id"]))
					if key == "" {
						key = strings.TrimSpace(fmt.Sprint(record["summary"]))
					}
					summary := strings.TrimSpace(fmt.Sprint(record["summary"]))
					if summary == "" {
						summary = strings.TrimSpace(fmt.Sprint(record["text"]))
					}
					if summary == "" {
						continue
					}
					if bb.AddFact("memory:"+key, summary, "memory:"+strings.TrimSpace(fmt.Sprint(record["source"]))) {
						// Stamp derivation and origin on the newly added fact
						if len(bb.Facts) > 0 {
							lastFact := &bb.Facts[len(bb.Facts)-1]
							origin := core.OriginDerivation("memory")
							recordID := strings.TrimSpace(fmt.Sprint(record["record_id"]))
							derived := origin.Derive("memory_store", "memory", 0.0, fmt.Sprintf("record_id=%s", recordID))
							lastFact.Derivation = &derived
							// Add origin tracking
							lastFact.Origin = &FactOrigin{
								SourceSystem: strings.TrimSpace(fmt.Sprint(record["source"])),
								RecordID:     recordID,
								Derivation:   &derived,
								CapturedAt:   time.Now().UTC(),
							}
						}
						added++
					}
				}
			}
		}
	} else if raw, ok := state.Get("graph.declarative_memory"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]core.MemoryRecordEnvelope); ok {
				for _, record := range results {
					if bb.AddFact("memory:"+record.Key, strings.TrimSpace(record.Summary), "memory:declarative") {
						// Stamp derivation and origin on the newly added fact
						if len(bb.Facts) > 0 {
							lastFact := &bb.Facts[len(bb.Facts)-1]
							origin := core.OriginDerivation("memory")
							derived := origin.Derive("memory_store", "memory", 0.0, fmt.Sprintf("record_key=%s", record.Key))
							lastFact.Derivation = &derived
							// Add origin tracking
							lastFact.Origin = &FactOrigin{
								SourceSystem: "declarative",
								RecordID:     record.RecordID,
								Scope:        record.Scope,
								Kind:         "declarative",
								Derivation:   &derived,
								CapturedAt:   time.Now().UTC(),
							}
						}
						added++
					}
				}
			}
		}
	}
	if raw, ok := state.Get("graph.procedural_memory_payload"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]map[string]any); ok {
				for _, record := range results {
					key := strings.TrimSpace(fmt.Sprint(record["record_id"]))
					if key == "" {
						key = strings.TrimSpace(fmt.Sprint(record["summary"]))
					}
					description := strings.TrimSpace(fmt.Sprint(record["summary"]))
					if description == "" {
						description = strings.TrimSpace(fmt.Sprint(record["text"]))
					}
					if description == "" {
						description = "consider learned routine " + key
					}
					if err := bb.AddHypothesis("memory:routine:"+key, description, 0.55, "memory:"+strings.TrimSpace(fmt.Sprint(record["source"]))); err == nil {
						// Stamp derivation and origin on the newly added hypothesis
						if len(bb.Hypotheses) > 0 {
							lastHyp := &bb.Hypotheses[len(bb.Hypotheses)-1]
							origin := core.OriginDerivation("memory")
							recordID := strings.TrimSpace(fmt.Sprint(record["record_id"]))
							derived := origin.Derive("memory_store", "memory", 0.0, fmt.Sprintf("record_id=%s", recordID))
							lastHyp.Derivation = &derived
							// Add origin tracking
							lastHyp.Origin = &FactOrigin{
								SourceSystem: strings.TrimSpace(fmt.Sprint(record["source"])),
								RecordID:     recordID,
								Derivation:   &derived,
								CapturedAt:   time.Now().UTC(),
							}
						}
						added++
					}
				}
			}
		}
	} else if raw, ok := state.Get("graph.procedural_memory"); ok {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]core.MemoryRecordEnvelope); ok {
				for _, record := range results {
					id := "memory:routine:" + strings.TrimSpace(record.Key)
					description := strings.TrimSpace(record.Summary)
					if description == "" {
						description = "consider learned routine " + strings.TrimSpace(record.Key)
					}
					if err := bb.AddHypothesis(id, description, 0.55, "memory:procedural"); err == nil {
						// Stamp derivation and origin on the newly added hypothesis
						if len(bb.Hypotheses) > 0 {
							lastHyp := &bb.Hypotheses[len(bb.Hypotheses)-1]
							origin := core.OriginDerivation("memory")
							derived := origin.Derive("memory_store", "memory", 0.0, fmt.Sprintf("record_key=%s", record.Key))
							lastHyp.Derivation = &derived
							// Add origin tracking
							lastHyp.Origin = &FactOrigin{
								SourceSystem: "procedural",
								RecordID:     record.RecordID,
								Scope:        record.Scope,
								Kind:         "procedural",
								Derivation:   &derived,
								CapturedAt:   time.Now().UTC(),
							}
						}
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
