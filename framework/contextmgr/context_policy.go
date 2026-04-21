package contextmgr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
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
	LanguageModel       core.LanguageModel
	Preferences         ContextPolicyPreferences
	IndexManager        *ast.IndexManager
	SearchEngine        *search.SearchEngine
	MemoryStore         memory.MemoryStore

	// PrecomputedChunks, when non-empty, are injected into the
	// ProgressiveLoader before InitialLoad runs. BKC session warmup
	// chunks should be passed here.
	PrecomputedChunks []ContextChunk
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
		if cfg.LanguageModel != nil {
			policy.Summarizer = NewLLMSummarizer(cfg.LanguageModel)
		} else {
			policy.Summarizer = &core.SimpleSummarizer{}
		}
	}
	if policy.Strategy == nil {
		policy.Strategy = NewAdaptiveStrategy()
	}
	if policy.Progressive == nil {
		policy.Progressive = NewProgressiveLoader(policy.ContextManager, cfg.IndexManager, cfg.SearchEngine, cfg.MemoryStore, policy.Budget, policy.Summarizer)
	}
	// Inject pre-computed chunks before InitialLoad runs
	if len(cfg.PrecomputedChunks) > 0 && policy.Progressive != nil {
		_ = policy.Progressive.InjectPrecomputedChunks(cfg.PrecomputedChunks)
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
		spec.ProgressiveLoading != nil
	if !hasOverrides {
		return
	}
	if spec.MaxTokens > 0 {
		p.Budget = core.NewContextBudget(spec.MaxTokens)
		p.ContextManager = NewContextManager(p.Budget)
		if p.Progressive != nil {
			p.Progressive = NewProgressiveLoader(p.ContextManager, p.Progressive.indexManager, p.Progressive.searchEngine, p.Progressive.memoryStore, p.Budget, p.Summarizer)
		} else if p.ProgressiveEnabled {
			p.Progressive = NewProgressiveLoader(p.ContextManager, nil, nil, nil, p.Budget, p.Summarizer)
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
	if spec.ProgressiveLoading != nil {
		p.ProgressiveEnabled = *spec.ProgressiveLoading
	}
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
	if budgetState >= core.BudgetNeedsCompression && p.Progressive != nil && p.ProgressiveEnabled {
		targetTokens := p.demotionTarget(budgetState)
		protected := protectedFileSet(state)
		if freed, err := p.Progressive.DemoteToFree(targetTokens, protected); err != nil {
			if debugf != nil {
				debugf("context demotion skipped: %v", err)
			}
		} else if freed > 0 && debugf != nil {
			debugf("demoted file context and freed %d tokens", freed)
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

func (p *ContextPolicy) demotionTarget(state core.BudgetState) int {
	if p == nil || p.Budget == nil {
		return 0
	}
	switch state {
	case core.BudgetCritical:
		if p.Budget.AvailableForContext/4 > 0 {
			return p.Budget.AvailableForContext / 4
		}
	case core.BudgetNeedsCompression:
		if p.Budget.AvailableForContext/10 > 0 {
			return p.Budget.AvailableForContext / 10
		}
	}
	return 1
}

func protectedFileSet(state *core.Context) map[string]struct{} {
	protected := make(map[string]struct{})
	if state == nil {
		return protected
	}
	addPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		protected[path] = struct{}{}
	}
	if raw, ok := state.Get("architect.current_step"); ok && raw != nil {
		switch step := raw.(type) {
		case core.PlanStep:
			for _, path := range step.Files {
				addPath(path)
			}
		case *core.PlanStep:
			if step != nil {
				for _, path := range step.Files {
					addPath(path)
				}
			}
		}
	}
	if raw, ok := state.Get("react.last_tool_result"); ok && raw != nil {
		if values, ok := raw.(map[string]interface{}); ok {
			if path := extractPathFromToolResult(values); path != "" {
				addPath(path)
			}
		}
	}
	return protected
}

func extractPathFromToolResult(values map[string]interface{}) string {
	if len(values) == 0 {
		return ""
	}
	if path, ok := values["path"]; ok {
		return strings.TrimSpace(fmt.Sprint(path))
	}
	for _, value := range values {
		nested, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if data, ok := nested["data"].(map[string]interface{}); ok {
			if path, ok := data["path"]; ok {
				return strings.TrimSpace(fmt.Sprint(path))
			}
		}
	}
	return ""
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
	p.RecordGraphMemoryPublications(state, debugf)
}

// RecordGraphMemoryPublications ingests richer graph memory publication state
// into the managed context set so reference-capable memory items are available
// to any prompt/runtime flow using ContextManager.
func (p *ContextPolicy) RecordGraphMemoryPublications(state *core.Context, debugf func(string, ...interface{})) {
	if p == nil || p.ContextManager == nil || state == nil {
		return
	}
	existing := graphMemoryItemKeySet(p.ContextManager.GetItemsByType(core.ContextTypeMemory))
	if existing == nil {
		existing = make(map[string]struct{})
	}
	for _, publication := range graphMemoryContextItems(state) {
		key := graphMemoryItemKey(publication)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		if err := p.ContextManager.AddItem(publication); err != nil && debugf != nil {
			debugf("graph memory context item add failed: %v", err)
			continue
		}
		existing[key] = struct{}{}
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

func graphMemoryContextItems(state *core.Context) []*core.MemoryContextItem {
	if state == nil {
		return nil
	}
	now := time.Now().UTC()
	items := make([]*core.MemoryContextItem, 0)
	items = append(items, memoryItemsFromGraphPublication(
		state,
		"graph.declarative_memory_payload",
		"graph.declarative_memory_refs",
		core.ContextReferenceRuntimeMemory,
		now,
	)...)
	items = append(items, memoryItemsFromGraphPublication(
		state,
		"graph.procedural_memory_payload",
		"graph.procedural_memory_refs",
		core.ContextReferenceRuntimeMemory,
		now,
	)...)
	if len(items) == 0 {
		return nil
	}
	return items
}

func memoryItemsFromGraphPublication(state *core.Context, payloadKey, refsKey string, defaultKind core.ContextReferenceKind, now time.Time) []*core.MemoryContextItem {
	if state == nil {
		return nil
	}
	var items []*core.MemoryContextItem
	if raw, ok := state.Get(payloadKey); ok && raw != nil {
		if payload, ok := raw.(map[string]any); ok {
			if results, ok := payload["results"].([]map[string]any); ok {
				for _, result := range results {
					item := memoryItemFromPublicationResult(result, defaultKind, now)
					if item != nil {
						items = append(items, item)
					}
				}
			}
		}
	}
	if len(items) > 0 {
		return items
	}
	if raw, ok := state.Get(refsKey); ok && raw != nil {
		if refs, ok := raw.([]core.ContextReference); ok {
			for _, ref := range refs {
				ref := ref
				label := strings.TrimSpace(ref.ID)
				if label == "" {
					label = strings.TrimSpace(ref.URI)
				}
				if label == "" {
					continue
				}
				items = append(items, &core.MemoryContextItem{
					Source:       "graph_memory",
					Summary:      label,
					Reference:    &ref,
					LastAccessed: now,
					Relevance:    0.8,
					PriorityVal:  1,
				})
			}
		}
	}
	return items
}

func memoryItemFromPublicationResult(result map[string]any, defaultKind core.ContextReferenceKind, now time.Time) *core.MemoryContextItem {
	summary := strings.TrimSpace(fmt.Sprint(result["summary"]))
	content := strings.TrimSpace(fmt.Sprint(result["text"]))
	if summary == "" || summary == "<nil>" {
		summary = content
	}
	if content == "<nil>" {
		content = ""
	}
	if summary == "" {
		return nil
	}
	ref := publicationReference(result, defaultKind)
	return &core.MemoryContextItem{
		Source:       strings.TrimSpace(fmt.Sprint(result["source"])),
		Content:      content,
		Summary:      summary,
		Reference:    ref,
		LastAccessed: now,
		Relevance:    0.85,
		PriorityVal:  1,
	}
}

func publicationReference(result map[string]any, defaultKind core.ContextReferenceKind) *core.ContextReference {
	raw, _ := result["reference"].(map[string]any)
	ref := &core.ContextReference{
		Kind:    defaultKind,
		ID:      strings.TrimSpace(fmt.Sprint(result["record_id"])),
		Detail:  strings.TrimSpace(fmt.Sprint(result["kind"])),
		URI:     "",
		Version: "",
	}
	if len(raw) > 0 {
		if kind := strings.TrimSpace(fmt.Sprint(raw["kind"])); kind != "" && kind != "<nil>" {
			ref.Kind = core.ContextReferenceKind(kind)
		}
		if id := strings.TrimSpace(fmt.Sprint(raw["id"])); id != "" && id != "<nil>" {
			ref.ID = id
		}
		if uri := strings.TrimSpace(fmt.Sprint(raw["uri"])); uri != "" && uri != "<nil>" {
			ref.URI = uri
		}
		if version := strings.TrimSpace(fmt.Sprint(raw["version"])); version != "" && version != "<nil>" {
			ref.Version = version
		}
		if detail := strings.TrimSpace(fmt.Sprint(raw["detail"])); detail != "" && detail != "<nil>" {
			ref.Detail = detail
		}
	}
	if ref.ID == "" && ref.URI == "" {
		return nil
	}
	return ref
}

func graphMemoryItemExists(items []ContextItem, candidate *core.MemoryContextItem) bool {
	if candidate == nil {
		return true
	}
	candidateKey := graphMemoryItemKey(candidate)
	if candidateKey == "" {
		return false
	}
	for _, item := range items {
		existing, ok := item.(*core.MemoryContextItem)
		if !ok {
			continue
		}
		if graphMemoryItemKey(existing) == candidateKey {
			return true
		}
	}
	return false
}

func graphMemoryItemKeySet(items []ContextItem) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	keys := make(map[string]struct{}, len(items))
	for _, item := range items {
		existing, ok := item.(*core.MemoryContextItem)
		if !ok {
			continue
		}
		if key := graphMemoryItemKey(existing); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func graphMemoryItemKey(item *core.MemoryContextItem) string {
	if item == nil {
		return ""
	}
	if item.Reference != nil {
		return string(item.Reference.Kind) + "|" + item.Reference.ID + "|" + item.Reference.URI
	}
	return strings.TrimSpace(item.Source) + "|" + strings.TrimSpace(item.Summary)
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
