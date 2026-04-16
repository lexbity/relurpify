package euclo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (eucloruntime.TaskEnvelope, eucloruntime.TaskClassification, euclotypes.ModeResolution, euclotypes.ExecutionProfileSelection, eucloruntime.UnitOfWork) {
	envelope := eucloruntime.NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, a.ModeRegistry)
	profile := eucloruntime.SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	skillPolicy := eucloruntime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), mode, profile)
	semanticInputs := a.semanticInputBundle(task, state, mode)
	work := euclowork.BuildUnitOfWork(task, state, envelope, classification, mode, profile, a.ModeRegistry, semanticInputs, skillPolicy, eucloruntime.WorkUnitExecutorDescriptor{})
	return envelope, classification, mode, profile, work
}

func (a *Agent) seedRuntimeState(state *core.Context, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection, work eucloruntime.UnitOfWork) {
	if state == nil {
		return
	}
	history := []eucloruntime.UnitOfWorkHistoryEntry(nil)
	if raw, ok := euclostate.GetUnitOfWorkHistory(state); ok {
		history = append(history, raw...)
	}
	if len(history) == 0 {
		if existing, ok := euclostate.GetUnitOfWork(state); ok && existing.ID != "" {
			history = eucloruntime.UpdateUnitOfWorkHistory(history, existing, existing.UpdatedAt)
		}
	}

	// Use typed accessors for core state keys
	euclostate.SetEnvelope(state, envelope)
	euclostate.SetClassification(state, classification)
	euclostate.SetMode(state, mode.ModeID)
	euclostate.SetExecutionProfile(state, profile.ProfileID)
	euclostate.SetUnitOfWork(state, work)
	euclostate.SetUnitOfWorkHistory(state, eucloruntime.UpdateUnitOfWorkHistory(history, work, work.UpdatedAt))

	// Legacy direct keys for fields not yet migrated to typed accessors
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.semantic_inputs", work.SemanticInputs)
	state.Set("euclo.resolved_execution_policy", work.ResolvedPolicy)
	state.Set("euclo.executor_descriptor", work.ExecutorDescriptor)
	state.Set("euclo.unit_of_work_id", work.ID)
	state.Set("euclo.root_unit_of_work_id", work.RootID)
	state.Set("euclo.unit_of_work_transition", work.TransitionState)

	// Phase 9: Store user recipe signals for classification
	if len(a.userRecipeSignals) > 0 {
		state.Set("euclo.user_recipe_signals", a.userRecipeSignals)
	}
}

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

// classifyCapabilityIntent performs capability-level classification using Tier 1 (static keywords),
// Tier 2 (LLM semantic), and Tier 3 (fallback). Result is stored in state for
// NormalizeTaskEnvelope to pick up on the second runtimeState pass.
func (a *Agent) classifyCapabilityIntent(ctx context.Context, task *core.Task, state *core.Context) error {
	if state == nil {
		return nil
	}
	// Idempotent: already classified for this invocation.
	if raw, ok := state.Get("euclo.pre_classified_capability_sequence"); ok && raw != nil {
		return nil
	}

	modeID := state.GetString("euclo.mode")
	if modeID == "" {
		return nil // mode not yet established; skip gracefully
	}

	classifier := &eucloruntime.CapabilityIntentClassifier{
		Registry:      euclorelurpic.DefaultRegistry(),
		ExtraKeywords: a.capabilityKeywordsFromManifest(),
		Model:         a.Environment.Model, // Phase 3: Tier 2 enabled when model available
	}

	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}

	result, err := classifier.Classify(ctx, instruction, modeID)
	if err != nil {
		return fmt.Errorf("euclo capability classification: %w", err)
	}

	euclostate.SetPreClassifiedCapabilitySequence(state, result.Sequence)
	euclostate.SetClassificationSource(state, result.Source)
	euclostate.SetClassificationMeta(state, result.Meta)
	euclostate.SetCapabilitySequenceOperator(state, result.Operator)
	return nil
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
