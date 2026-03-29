package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/search"
)

type ContextRuntime struct {
	Policy         *contextmgr.ContextPolicy
	Shared         *core.SharedContext
	State          ContextRuntimeState
	debugMessages  []string
	protectedPaths map[string]struct{}
}

type ContextRuntimeConfig struct {
	Config       *core.Config
	Model        core.LanguageModel
	MemoryStore  memory.MemoryStore
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

func BuildContextRuntime(task *core.Task, cfg ContextRuntimeConfig, mode ModeResolution, work UnitOfWork) *ContextRuntime {
	strategy, strategyName := selectContextStrategy(mode, work)
	preferences := buildContextPolicyPreferences(mode, work)
	spec := agentContextSpec(cfg.Config)
	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Strategy:      strategy,
		LanguageModel: cfg.Model,
		MemoryStore:   cfg.MemoryStore,
		IndexManager:  cfg.IndexManager,
		SearchEngine:  cfg.SearchEngine,
		Preferences:   preferences,
	}, spec)
	if policy != nil && policy.Budget != nil {
		system, tools, output := contextReservationsForWork(work)
		policy.Budget.SetReservations(system, tools, output)
	}
	state := ContextRuntimeState{
		ModeID:               mode.ModeID,
		ExecutorFamily:       work.ExecutorDescriptor.Family,
		StrategyName:         strategyName,
		PreferredDetail:      preferences.PreferredDetailLevel.String(),
		ProgressiveEnabled:   policy != nil && policy.ProgressiveEnabled,
		MinHistorySize:       preferences.MinHistorySize,
		CompressionThreshold: preferences.CompressionThreshold,
		CompactionEligible:   work.ContextBundle.CompactionEligible,
		RestoreRequired:      work.ContextBundle.RestoreRequired,
		ProtectedPaths:       contextProtectedPaths(task, work),
		UpdatedAt:            time.Now().UTC(),
	}
	if policy != nil && policy.Budget != nil {
		state.BudgetMaxTokens = policy.Budget.MaxTokens
		state.AvailableContextTokens = policy.Budget.AvailableForContext
		state.BudgetState = budgetStateLabel(policy.Budget.CheckBudget())
	}
	return &ContextRuntime{
		Policy:         policy,
		Shared:         core.NewSharedContext(core.NewContext(), policy.Budget, policy.Summarizer),
		State:          state,
		protectedPaths: stringSliceSet(state.ProtectedPaths),
	}
}

func (rt *ContextRuntime) Activate(task *core.Task, state *core.Context, model core.LanguageModel) ContextRuntimeState {
	if rt == nil || rt.Policy == nil || state == nil {
		return ContextRuntimeState{}
	}
	rt.State.InitialLoadAttempted = true
	if err := rt.Policy.InitialLoad(task); err != nil {
		rt.State.LastInitialLoadError = err.Error()
	} else {
		rt.State.InitialLoadCompleted = true
	}
	rt.Policy.RecordLatestInteraction(state, rt.recordDebug)
	rt.Policy.RecordGraphMemoryPublications(state, rt.recordDebug)
	rt.Policy.EnforceBudget(state, rt.Shared, model, nil, rt.recordDebug)
	rt.syncState()
	state.Set("euclo.context_runtime", rt.State)
	return rt.State
}

func (rt *ContextRuntime) HandleResult(state *core.Context, model core.LanguageModel, result *core.Result) ContextRuntimeState {
	if rt == nil || rt.Policy == nil || state == nil {
		return ContextRuntimeState{}
	}
	rt.Policy.RecordLatestInteraction(state, rt.recordDebug)
	rt.Policy.RecordGraphMemoryPublications(state, rt.recordDebug)
	rt.Policy.EnforceBudget(state, rt.Shared, model, nil, rt.recordDebug)
	rt.Policy.HandleSignals(state, rt.Shared, result)
	rt.State.SignalsHandled = true
	rt.syncState()
	state.Set("euclo.context_runtime", rt.State)
	return rt.State
}

func (rt *ContextRuntime) syncState() {
	if rt == nil || rt.Policy == nil || rt.Policy.Budget == nil {
		return
	}
	usage := rt.Policy.Budget.GetCurrentUsage()
	rt.State.BudgetMaxTokens = rt.Policy.Budget.MaxTokens
	rt.State.AvailableContextTokens = rt.Policy.Budget.AvailableForContext
	rt.State.BudgetState = budgetStateLabel(rt.Policy.Budget.CheckBudget())
	rt.State.ContextTokens = usage.ContextTokens
	rt.State.ContextUsagePercent = usage.ContextUsagePercent
	rt.State.ProgressiveEnabled = rt.Policy.ProgressiveEnabled
	rt.State.DebugMessages = append([]string(nil), rt.debugMessages...)
	rt.State.CompactionObserved = containsDebugMessage(rt.debugMessages, "compression", "context compact", "shared context compression", "demoted file context")
	rt.State.DemotionObserved = containsDebugMessage(rt.debugMessages, "demoted file context")
	rt.State.PruningObserved = containsDebugMessage(rt.debugMessages, "context pruning")
	rt.State.UpdatedAt = time.Now().UTC()
}

func (rt *ContextRuntime) recordDebug(format string, args ...interface{}) {
	if rt == nil {
		return
	}
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		return
	}
	rt.debugMessages = append(rt.debugMessages, message)
	if len(rt.debugMessages) > 12 {
		rt.debugMessages = rt.debugMessages[len(rt.debugMessages)-12:]
	}
}

func selectContextStrategy(mode ModeResolution, work UnitOfWork) (contextmgr.ContextStrategy, string) {
	switch {
	case work.ExecutorDescriptor.Family == ExecutorFamilyRewoo:
		return contextmgr.NewConservativeStrategy(), "conservative"
	case work.ExecutorDescriptor.Family == ExecutorFamilyPlanner:
		return contextmgr.NewConservativeStrategy(), "conservative"
	case mode.ModeID == "review" || work.ExecutorDescriptor.Family == ExecutorFamilyReflection:
		return contextmgr.NewConservativeStrategy(), "conservative"
	case mode.ModeID == "debug":
		return contextmgr.NewAggressiveStrategy(), "aggressive"
	default:
		return contextmgr.NewAdaptiveStrategy(), "adaptive"
	}
}

func buildContextPolicyPreferences(mode ModeResolution, work UnitOfWork) contextmgr.ContextPolicyPreferences {
	preferredDetail := parsePreferredDetail(work.ResolvedPolicy.ContextPolicy.PreferredDetail)
	preferences := contextmgr.ContextPolicyPreferences{
		PreferredDetailLevel: preferredDetail,
		MinHistorySize:       5,
		CompressionThreshold: 0.8,
	}
	switch {
	case work.ExecutorDescriptor.Family == ExecutorFamilyRewoo || (work.PlanBinding != nil && work.PlanBinding.IsLongRunning):
		preferences.MinHistorySize = 8
		preferences.CompressionThreshold = 0.72
	case work.ExecutorDescriptor.Family == ExecutorFamilyPlanner || mode.ModeID == "planning":
		preferences.MinHistorySize = 7
		preferences.CompressionThreshold = 0.75
	case mode.ModeID == "review" || work.ExecutorDescriptor.Family == ExecutorFamilyReflection:
		preferences.MinHistorySize = 6
		preferences.CompressionThreshold = 0.78
	case mode.ModeID == "debug":
		preferences.MinHistorySize = 4
		preferences.CompressionThreshold = 0.85
	}
	return preferences
}

func contextReservationsForWork(work UnitOfWork) (int, int, int) {
	switch {
	case work.ExecutorDescriptor.Family == ExecutorFamilyRewoo || (work.PlanBinding != nil && work.PlanBinding.IsLongRunning):
		return 900, 1800, 1200
	case work.ExecutorDescriptor.Family == ExecutorFamilyPlanner || work.ModeID == "planning":
		return 900, 1500, 1200
	case work.ModeID == "debug":
		return 700, 1600, 1000
	default:
		return 800, 1500, 1000
	}
}

func parsePreferredDetail(value string) contextmgr.DetailLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "minimal":
		return contextmgr.DetailMinimal
	case "concise":
		return contextmgr.DetailConcise
	case "full":
		return contextmgr.DetailFull
	case "signature", "signature_only":
		return contextmgr.DetailSignatureOnly
	default:
		return contextmgr.DetailDetailed
	}
}

func budgetStateLabel(state core.BudgetState) string {
	switch state {
	case core.BudgetWarning:
		return "warning"
	case core.BudgetNeedsCompression:
		return "needs_compression"
	case core.BudgetCritical:
		return "critical"
	default:
		return "ok"
	}
}

func contextProtectedPaths(task *core.Task, work UnitOfWork) []string {
	paths := append([]string(nil), work.ContextBundle.WorkspacePaths...)
	if work.PlanBinding != nil && work.PlanBinding.ActiveStepID != "" {
		paths = append(paths, work.PlanBinding.ActiveStepID)
	}
	if task != nil && task.Context != nil {
		if path := strings.TrimSpace(fmt.Sprint(task.Context["workspace"])); path != "" && path != "<nil>" {
			paths = append(paths, path)
		}
	}
	return uniqueStrings(paths)
}

func stringSliceSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func containsDebugMessage(messages []string, needles ...string) bool {
	for _, message := range messages {
		lower := strings.ToLower(message)
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

func agentContextSpec(cfg *core.Config) *core.AgentContextSpec {
	if cfg == nil || cfg.AgentSpec == nil {
		return nil
	}
	return &cfg.AgentSpec.Context
}
