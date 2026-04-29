package blackboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

type blackboardScopedMemoryRetriever struct {
	store       *memory.WorkingMemoryStore
	taskID      string
	keyPrefix   string
	memoryClass core.MemoryClass
}

// RetrievalServiceProvider is a placeholder interface for retrieval services.
// This interface is temporarily stubbed out as the retrieval package is being rebuilt.
type blackboardRetrievalServiceProvider interface {
	// RetrievalService() retrieval.RetrieverService
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
	q := strings.TrimSpace(query)
	prefix := strings.TrimSpace(r.keyPrefix)
	if q != "" {
		if prefix == "" {
			prefix = q
		} else if !strings.HasPrefix(prefix, q) {
			prefix = q
		}
	}
	records, err := r.store.Retrieve(ctx, memory.MemoryQuery{
		TaskID:    strings.TrimSpace(r.taskID),
		KeyPrefix: prefix,
		Class:     r.memoryClass,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]core.MemoryRecordEnvelope, 0, len(records))
	for _, record := range records {
		entry := record.Entry
		summary, text := summarizeMemoryEntry(entry.Value)
		out = append(out, core.MemoryRecordEnvelope{
			Key:         record.Key,
			MemoryClass: r.memoryClass,
			Scope:       record.TaskID,
			Summary:     summary,
			Text:        text,
			Source:      "working_memory",
			RecordID:    record.Key,
			Reference: map[string]any{
				"task_id": record.TaskID,
				"key":     record.Key,
			},
		})
	}
	return graph.BuildMemoryRetrievalPublication(q, out, r.memoryClass), nil
}

func hydrateBlackboardFromMemory(state *contextdata.Envelope, bb *Blackboard) int {
	if state == nil || bb == nil {
		return 0
	}
	added := 0
	if raw, ok := envelopeGet(state, "graph.declarative_memory_payload"); ok {
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
	} else if raw, ok := state.GetWorkingValue("graph.declarative_memory"); ok {
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
	if raw, ok := state.GetWorkingValue("graph.procedural_memory_payload"); ok {
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
	} else if raw, ok := state.GetWorkingValue("graph.procedural_memory"); ok {
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

func summarizeMemoryEntry(value any) (summary string, text string) {
	switch typed := value.(type) {
	case nil:
		return "", ""
	case string:
		s := strings.TrimSpace(typed)
		return s, s
	case fmt.Stringer:
		s := strings.TrimSpace(typed.String())
		return s, s
	case []string:
		joined := strings.Join(typed, ", ")
		return truncateMemorySummary(joined), joined
	case []any:
		joined := fmt.Sprint(typed)
		return truncateMemorySummary(joined), joined
	case map[string]any:
		joined := fmt.Sprint(typed)
		return truncateMemorySummary(joined), joined
	default:
		joined := fmt.Sprint(value)
		return truncateMemorySummary(joined), joined
	}
}

func truncateMemorySummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 240 {
		return value
	}
	return value[:240] + "...(truncated)"
}

func publishPersistenceCandidates(state *contextdata.Envelope, bb *Blackboard, controller ControllerState, metrics Metrics) {
	if state == nil || bb == nil {
		return
	}
	goal := ""
	if len(bb.Goals) > 0 {
		goal = strings.TrimSpace(bb.Goals[0])
	}
	envelopeSet(state, contextKeyPersistenceSummary, map[string]any{
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
	envelopeSet(state, contextKeyPersistenceDecision, map[string]any{
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
	envelopeSet(state, contextKeyPersistenceRoutine, routine)
}
