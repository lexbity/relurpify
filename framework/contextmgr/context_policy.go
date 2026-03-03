package contextmgr

import (
	"context"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	"strings"
)

// ContextPolicyPreferences tune how the policy compresses and expands context.
type ContextPolicyPreferences struct {
	PreferredDetailLevel DetailLevel
	MinHistorySize       int
	CompressionThreshold float64
}

// ContextPolicyConfig bundles optional dependencies.
type ContextPolicyConfig struct {
	Budget              *core.ContextBudget
	ContextManager      *ContextManager
	Strategy            ContextStrategy
	Progressive         *ProgressiveLoader
	CompressionStrategy core.CompressionStrategy
	Summarizer          core.Summarizer
	Preferences         ContextPolicyPreferences
	IndexManager        *ast.IndexManager
}

// ContextPolicy centralizes strategy selection, progressive loading, and compression.
type ContextPolicy struct {
	Budget              *core.ContextBudget
	ContextManager      *ContextManager
	Strategy            ContextStrategy
	Progressive         *ProgressiveLoader
	CompressionStrategy core.CompressionStrategy
	Summarizer          core.Summarizer
	Preferences         ContextPolicyPreferences
	ProgressiveEnabled  bool
}

// NewContextPolicy builds a policy with sensible defaults.
func NewContextPolicy(cfg ContextPolicyConfig, spec *core.AgentContextSpec) *ContextPolicy {
	policy := &ContextPolicy{
		Budget:              cfg.Budget,
		ContextManager:      cfg.ContextManager,
		Strategy:            cfg.Strategy,
		Progressive:         cfg.Progressive,
		CompressionStrategy: cfg.CompressionStrategy,
		Summarizer:          cfg.Summarizer,
		Preferences:         cfg.Preferences,
		ProgressiveEnabled:  true,
	}
	if policy.Budget == nil {
		maxTokens := 8000
		if spec != nil && spec.MaxTokens > 0 {
			maxTokens = spec.MaxTokens
		}
		policy.Budget = core.NewContextBudget(maxTokens)
	}
	if policy.ContextManager == nil {
		policy.ContextManager = NewContextManager(policy.Budget)
	}
	if policy.CompressionStrategy == nil {
		policy.CompressionStrategy = core.NewSimpleCompressionStrategy()
	}
	if policy.Summarizer == nil {
		policy.Summarizer = &core.SimpleSummarizer{}
	}
	if policy.Strategy == nil {
		policy.Strategy = NewAdaptiveStrategy()
	}
	if policy.Progressive == nil {
		policy.Progressive = NewProgressiveLoader(policy.ContextManager, cfg.IndexManager, nil, policy.Budget, policy.Summarizer)
	}
	policy.ApplyAgentContextSpec(spec)
	return policy
}

// ApplyAgentContextSpec overlays explicit agent context settings.
func (p *ContextPolicy) ApplyAgentContextSpec(spec *core.AgentContextSpec) {
	if p == nil || spec == nil {
		return
	}
	hasOverrides := spec.MaxTokens > 0 ||
		spec.MaxFiles > 0 ||
		spec.IncludeGitHistory ||
		spec.IncludeDependencies ||
		spec.CompressionStrategy != "" ||
		spec.ProgressiveLoading
	if !hasOverrides {
		return
	}
	if spec.MaxTokens > 0 {
		p.Budget = core.NewContextBudget(spec.MaxTokens)
		p.ContextManager = NewContextManager(p.Budget)
		if p.Progressive != nil {
			p.Progressive = NewProgressiveLoader(p.ContextManager, p.Progressive.indexManager, p.Progressive.searchEngine, p.Budget, p.Summarizer)
		} else if p.ProgressiveEnabled {
			p.Progressive = NewProgressiveLoader(p.ContextManager, nil, nil, p.Budget, p.Summarizer)
		}
	}
	if spec.CompressionStrategy != "" {
		switch strings.ToLower(spec.CompressionStrategy) {
		case "summary", "hybrid":
			p.CompressionStrategy = core.NewSimpleCompressionStrategy()
		case "truncate":
			p.CompressionStrategy = core.NewSimpleCompressionStrategy()
		}
	}
	p.ProgressiveEnabled = spec.ProgressiveLoading
}

// InitialLoad executes the strategy's initial context request.
func (p *ContextPolicy) InitialLoad(task *core.Task) error {
	if p == nil || p.Progressive == nil || p.Strategy == nil {
		return nil
	}
	return p.Progressive.InitialLoad(task, p.Strategy)
}

// EnforceBudget manages compression and pruning for the current context.
func (p *ContextPolicy) EnforceBudget(state *core.Context, shared *core.SharedContext, model core.LanguageModel, tools []core.Tool, debugf func(string, ...interface{})) {
	if p == nil || p.Budget == nil {
		return
	}
	p.Budget.UpdateUsage(state, tools)
	budgetState := p.Budget.CheckBudget()
	if budgetState >= core.BudgetNeedsCompression && model != nil {
		compressed := false
		if shared != nil && p.Strategy != nil && p.CompressionStrategy != nil {
			if p.Strategy.ShouldCompress(shared) {
				keep := p.Preferences.MinHistorySize
				if keep <= 0 {
					keep = p.CompressionStrategy.KeepRecent()
				}
				if keep <= 0 {
					keep = 5
				}
				if err := shared.CompressHistory(keep, model, p.CompressionStrategy); err != nil {
					if debugf != nil {
						debugf("shared context compression failed: %v", err)
					}
				} else {
					compressed = true
				}
			}
		}
		if !compressed && p.CompressionStrategy != nil {
			if err := state.CompressHistory(p.CompressionStrategy.KeepRecent(), model, p.CompressionStrategy); err != nil {
				if debugf != nil {
					debugf("compression failed: %v", err)
				}
			} else {
				compressed = true
			}
		}
		if compressed {
			p.Budget.UpdateUsage(state, tools)
		}
	}
	if budgetState == core.BudgetCritical && p.ContextManager != nil {
		targetTokens := p.Budget.AvailableForContext / 4
		if targetTokens == 0 {
			targetTokens = 1
		}
		if err := p.ContextManager.MakeSpace(targetTokens); err != nil {
			if debugf != nil {
				debugf("context pruning failed: %v", err)
			}
		}
	}
}

// RecordLatestInteraction adds the newest interaction to the context manager.
func (p *ContextPolicy) RecordLatestInteraction(state *core.Context, debugf func(string, ...interface{})) {
	if p == nil || p.ContextManager == nil || state == nil {
		return
	}
	interaction, ok := state.LatestInteraction()
	if !ok {
		return
	}
	item := &core.InteractionContextItem{
		Interaction: interaction,
		Relevance:   1.0,
		PriorityVal: 1,
	}
	if err := p.ContextManager.AddItem(item); err != nil && debugf != nil {
		debugf("context item add failed: %v", err)
	}
}

// HandleSignals expands context when the strategy detects gaps or uncertainty.
func (p *ContextPolicy) HandleSignals(state *core.Context, shared *core.SharedContext, lastResult *core.Result) {
	if p == nil || p.Strategy == nil || p.Progressive == nil || !p.ProgressiveEnabled {
		return
	}
	if shared != nil && p.Strategy.ShouldExpandContext(shared, lastResult) {
		p.expandContextFromResult(lastResult)
	}
	if p.detectUncertainty(state) {
		p.handleUncertainty(state)
	}
}

func (p *ContextPolicy) expandContextFromResult(result *core.Result) {
	if result == nil || result.Data == nil || p.Progressive == nil {
		return
	}
	if file, ok := result.Data["file"].(string); ok && file != "" {
		_ = p.Progressive.DrillDown(file)
		return
	}
	if focus, ok := result.Data["focus_area"].(string); ok && focus != "" {
		_ = p.Progressive.LoadRelatedFiles(focus, 1)
	}
}

func (p *ContextPolicy) detectUncertainty(state *core.Context) bool {
	if state == nil {
		return false
	}
	history := state.History()
	if len(history) == 0 {
		return false
	}
	last := history[len(history)-1]
	content := strings.ToLower(last.Content)
	markers := []string{
		"not sure", "unclear", "need more information",
		"cannot determine", "insufficient context", "missing information",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func (p *ContextPolicy) handleUncertainty(state *core.Context) {
	if state == nil || p.Progressive == nil {
		return
	}
	history := state.History()
	if len(history) == 0 {
		return
	}
	last := history[len(history)-1]
	for _, file := range ExtractFileReferences(last.Content) {
		_ = p.Progressive.ExpandContext(file, DetailDetailed)
	}
	if len(ExtractSymbolReferences(last.Content)) > 0 {
		request := &ContextRequest{
			ASTQueries: []ASTQuery{
				{Type: ASTQueryListSymbols},
			},
		}
		_ = p.Progressive.ExecuteContextRequest(request, "symbol_lookup")
	}
}

// Diagnose provides a helper hook to route error analysis through the policy.
func (p *ContextPolicy) Diagnose(ctx context.Context, step core.PlanStep, err error, diagnosisFn func(context.Context, core.PlanStep, error) (string, error)) (string, error) {
	if diagnosisFn == nil {
		return "", nil
	}
	return diagnosisFn(ctx, step, err)
}
