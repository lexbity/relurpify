package euclo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
)

func (a *Agent) ensureWorkflowRun(ctx context.Context, task *core.Task, state *core.Context) {
	if a == nil || state == nil {
		return
	}
	store := a.workflowStore()
	if store == nil {
		return
	}
	_, _, _ = euclorestore.EnsureWorkflowRun(ctx, store, task, state)
}

func (a *Agent) ensureDeferralPlan(task *core.Task, state *core.Context) {
	if a == nil || a.GuidanceBroker == nil {
		return
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		workflowID = "session"
	}
	if a.DeferralPlan == nil || a.DeferralPlan.WorkflowID != workflowID {
		now := time.Now().UTC()
		a.DeferralPlan = &guidance.DeferralPlan{
			ID:         fmt.Sprintf("deferral-%d", now.UnixNano()),
			WorkflowID: workflowID,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	a.GuidanceBroker.SetDeferralPlan(a.DeferralPlan)
	a.registerDeferralsResolveRoutine()
}

func (a *Agent) registerDeferralsResolveRoutine() {
	if a == nil || a.BehaviorDispatcher == nil {
		return
	}
	a.BehaviorDispatcher.RegisterSupporting(&localbehavior.DeferralsResolveRoutine{
		DeferralPlan:   a.DeferralPlan,
		GuidanceBroker: a.GuidanceBroker,
	})
}

func (a *Agent) registerLearningPromoteRoutine() {
	if a == nil || a.BehaviorDispatcher == nil {
		return
	}
	a.BehaviorDispatcher.RegisterSupporting(&localbehavior.LearningPromoteRoutine{
		LearningService: a.learningService(),
		WorkflowResolver: func(state *core.Context) (string, string) {
			return workflowIDFromState(state), explorationIDFromState(state)
		},
	})
}

// capabilityKeywordsFromManifest returns user-configured capability keywords from manifest.
// Reads from the "euclo" extension in agent.manifest.yaml Extensions field.
func (a *Agent) capabilityKeywordsFromManifest() map[string][]string {
	if a == nil || a.Config == nil || a.Config.AgentSpec == nil {
		return nil
	}

	extensions := a.Config.AgentSpec.Extensions
	if len(extensions) == 0 {
		return nil
	}

	eucloRaw, ok := extensions[EucloExtensionKey]
	if !ok || eucloRaw == nil {
		return nil
	}

	// Try to parse as EucloManifestExtension
	ext, err := parseEucloExtension(eucloRaw)
	if err != nil {
		// Log error but don't fail - fall back to built-in keywords
		return nil
	}

	return capabilityKeywordsFromManifest(ext)
}

// parseEucloExtension converts raw extension data (from YAML) into EucloManifestExtension.
// Handles both map[string]any and EucloManifestExtension types.
func parseEucloExtension(raw any) (*EucloManifestExtension, error) {
	if raw == nil {
		return nil, fmt.Errorf("nil euclo extension")
	}

	// If already the right type, return it
	if ext, ok := raw.(*EucloManifestExtension); ok {
		return ext, nil
	}

	// Try to convert from map[string]any (typical YAML parsing result)
	if m, ok := raw.(map[string]any); ok {
		ext := &EucloManifestExtension{
			CapabilityKeywords: make(map[string][]string),
		}

		if kwRaw, ok := m["capability_keywords"]; ok && kwRaw != nil {
			if kwMap, ok := kwRaw.(map[string]any); ok {
				for capID, wordsRaw := range kwMap {
					if words, ok := wordsRaw.([]any); ok {
						for _, w := range words {
							if s, ok := w.(string); ok {
								ext.CapabilityKeywords[capID] = append(ext.CapabilityKeywords[capID], s)
							}
						}
					}
				}
			}
		}

		return ext, nil
	}

	return nil, fmt.Errorf("unable to parse euclo extension from type %T", raw)
}
