package core

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

type AgentDefinition = agentspec.AgentDefinition
type AgentSemanticContext = agentspec.AgentSemanticContext
type AgentContextChunk = agentspec.AgentContextChunk
type SkillStepTemplate = agentspec.SkillStepTemplate
type AgentReviewApprovalRules = agentspec.AgentReviewApprovalRules
type AgentPermissionLevel = agentspec.AgentPermissionLevel
type AgentMode = agentspec.AgentMode
type ToolPolicy = agentspec.ToolPolicy
type CapabilityPolicy = agentspec.CapabilityPolicy
type CapabilityInsertionPolicy = agentspec.CapabilityInsertionPolicy
type CapabilityExposure = agentspec.CapabilityExposure
type CapabilityExposurePolicy = agentspec.CapabilityExposurePolicy
type ProviderPolicy = agentspec.ProviderPolicy

var ErrNotAgentDefinition = agentspec.ErrNotAgentDefinition

func LoadAgentDefinition(path string) (*AgentDefinition, error) {
	return agentspec.LoadAgentDefinition(path)
}

const (
	AgentPermissionAllow AgentPermissionLevel = agentspec.AgentPermissionAllow
	AgentPermissionDeny  AgentPermissionLevel = agentspec.AgentPermissionDeny
	AgentPermissionAsk   AgentPermissionLevel = agentspec.AgentPermissionAsk

	AgentModePrimary AgentMode = agentspec.AgentModePrimary
	AgentModeSub     AgentMode = agentspec.AgentModeSub
	AgentModeSystem  AgentMode = agentspec.AgentModeSystem

	CapabilityExposureHidden      CapabilityExposure = agentspec.CapabilityExposureHidden
	CapabilityExposureInspectable CapabilityExposure = agentspec.CapabilityExposureInspectable
	CapabilityExposureCallable    CapabilityExposure = agentspec.CapabilityExposureCallable
)

type KeyFact struct {
	Type      string  `json:"type,omitempty" yaml:"type,omitempty"`
	Content   string  `json:"content,omitempty" yaml:"content,omitempty"`
	Relevance float64 `json:"relevance,omitempty" yaml:"relevance,omitempty"`
}

type Interaction struct {
	ID        int            `json:"id,omitempty" yaml:"id,omitempty"`
	Role      string         `json:"role,omitempty" yaml:"role,omitempty"`
	Content   string         `json:"content,omitempty" yaml:"content,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
}

type CompressedContext struct {
	Summary          string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	KeyFacts         []KeyFact `json:"key_facts,omitempty" yaml:"key_facts,omitempty"`
	OriginalTokens   int       `json:"original_tokens,omitempty" yaml:"original_tokens,omitempty"`
	CompressedTokens int       `json:"compressed_tokens,omitempty" yaml:"compressed_tokens,omitempty"`
}

type CompressionStrategy interface {
	Compress(interactions []Interaction, llm LanguageModel) (*CompressedContext, error)
	ShouldCompress(ctx *Context, budget *ArtifactBudget) bool
	EstimateTokens(cc *CompressedContext) int
	KeepRecent() int
}

type DetailLevel string

const (
	DetailFull     DetailLevel = "full"
	DetailSummary  DetailLevel = "summary"
	DetailBodyOnly DetailLevel = "body-only"
	DetailMetadata DetailLevel = "metadata"
	DetailMinimal  DetailLevel = "minimal"
)

type FileContext struct {
	Path       string         `json:"path,omitempty" yaml:"path,omitempty"`
	Language   string         `json:"language,omitempty" yaml:"language,omitempty"`
	Level      DetailLevel    `json:"level,omitempty" yaml:"level,omitempty"`
	Content    string         `json:"content,omitempty" yaml:"content,omitempty"`
	RawContent string         `json:"raw_content,omitempty" yaml:"raw_content,omitempty"`
	Summary    string         `json:"summary,omitempty" yaml:"summary,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type Context struct {
	mu                sync.RWMutex
	state             map[string]any
	variables         map[string]any
	knowledge         map[string]any
	history           []ContextHistoryEntry
	compressedHistory []CompressedContextHistoryEntry
	data              map[string]any
	handles           map[string]string
	handleScopes      map[string]string
	registry          *ObjectRegistry
	baseline          *ContextSnapshot
}

type ContextSnapshot struct {
	State             map[string]any                  `json:"state,omitempty" yaml:"state,omitempty"`
	Variables         map[string]any                  `json:"variables,omitempty" yaml:"variables,omitempty"`
	Knowledge         map[string]any                  `json:"knowledge,omitempty" yaml:"knowledge,omitempty"`
	History           []ContextHistoryEntry           `json:"history,omitempty" yaml:"history,omitempty"`
	CompressedHistory []CompressedContextHistoryEntry `json:"compressed_history,omitempty" yaml:"compressed_history,omitempty"`
	Data              map[string]any                  `json:"data,omitempty" yaml:"data,omitempty"`
}

type ContextHistoryEntry struct {
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
}

type CompressedContextHistoryEntry struct {
	Content          string `json:"content,omitempty" yaml:"content,omitempty"`
	CompressedTokens int    `json:"compressed_tokens,omitempty" yaml:"compressed_tokens,omitempty"`
}

type TaskType string

const (
	TaskTypeAnalysis         TaskType = "analysis"
	TaskTypePlanning         TaskType = "planning"
	TaskTypeCodeGeneration   TaskType = "code-generation"
	TaskTypeCodeModification TaskType = "code-modification"
	TaskTypeReview           TaskType = "review"
)

type Task struct {
	ID          string            `json:"id,omitempty" yaml:"id,omitempty"`
	Type        TaskType          `json:"type,omitempty" yaml:"type,omitempty"`
	Instruction string            `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	Context     map[string]any    `json:"context,omitempty" yaml:"context,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type Result struct {
	NodeID   string         `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Success  bool           `json:"success" yaml:"success"`
	Data     map[string]any `json:"data,omitempty" yaml:"data,omitempty"`
	Error    error          `json:"error,omitempty" yaml:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type CapabilitySelector struct {
	ID                          string                      `json:"id,omitempty" yaml:"id,omitempty"`
	Name                        string                      `json:"name,omitempty" yaml:"name,omitempty"`
	Kind                        CapabilityKind              `json:"kind,omitempty" yaml:"kind,omitempty"`
	RuntimeFamilies             []CapabilityRuntimeFamily   `json:"runtime_families,omitempty" yaml:"runtime_families,omitempty"`
	Tags                        []string                    `json:"tags,omitempty" yaml:"tags,omitempty"`
	ExcludeTags                 []string                    `json:"exclude_tags,omitempty" yaml:"exclude_tags,omitempty"`
	SourceScopes                []CapabilityScope           `json:"source_scopes,omitempty" yaml:"source_scopes,omitempty"`
	TrustClasses                []TrustClass                `json:"trust_classes,omitempty" yaml:"trust_classes,omitempty"`
	RiskClasses                 []RiskClass                 `json:"risk_classes,omitempty" yaml:"risk_classes,omitempty"`
	EffectClasses               []EffectClass               `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	CoordinationRoles           []CoordinationRole          `json:"coordination_roles,omitempty" yaml:"coordination_roles,omitempty"`
	CoordinationTaskTypes       []string                    `json:"coordination_task_types,omitempty" yaml:"coordination_task_types,omitempty"`
	CoordinationExecutionModes  []CoordinationExecutionMode `json:"coordination_execution_modes,omitempty" yaml:"coordination_execution_modes,omitempty"`
	CoordinationLongRunning     *bool                       `json:"coordination_long_running,omitempty" yaml:"coordination_long_running,omitempty"`
	CoordinationDirectInsertion *bool                       `json:"coordination_direct_insertion,omitempty" yaml:"coordination_direct_insertion,omitempty"`
}

type SkillCapabilitySelector = agentspec.SkillCapabilitySelector

func NewContext() *Context {
	return &Context{
		state:        map[string]any{},
		variables:    map[string]any{},
		knowledge:    map[string]any{},
		data:         map[string]any{},
		handles:      map[string]string{},
		handleScopes: map[string]string{},
		registry:     NewObjectRegistry(),
	}
}

func NewContextFromSnapshot(snapshot *ContextSnapshot, _ any) *Context {
	if snapshot == nil {
		return NewContext()
	}
	ctx := NewContext()
	ctx.state = cloneAnyMap(snapshot.State)
	ctx.variables = cloneAnyMap(snapshot.Variables)
	ctx.knowledge = cloneAnyMap(snapshot.Knowledge)
	ctx.data = cloneAnyMap(snapshot.Data)
	ctx.history = append([]ContextHistoryEntry(nil), snapshot.History...)
	ctx.compressedHistory = append([]CompressedContextHistoryEntry(nil), snapshot.CompressedHistory...)
	for key, value := range snapshot.Data {
		ctx.data[key] = value
	}
	return ctx
}

func (c *Context) ensure() {
	if c != nil {
		if c.state == nil {
			c.state = map[string]any{}
		}
		if c.variables == nil {
			c.variables = map[string]any{}
		}
		if c.knowledge == nil {
			c.knowledge = map[string]any{}
		}
		if c.data == nil {
			c.data = map[string]any{}
		}
	}
}

func (c *Context) Set(key string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	trimmed := strings.TrimSpace(key)
	c.state[trimmed] = value
	c.data[trimmed] = value
}

func (c *Context) SetVariable(key string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.variables[strings.TrimSpace(key)] = value
}

func (c *Context) GetVariable(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.variables[strings.TrimSpace(key)]
	return value, ok
}

func (c *Context) SetKnowledge(key string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	c.knowledge[strings.TrimSpace(key)] = value
}

func (c *Context) GetKnowledge(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.knowledge[strings.TrimSpace(key)]
	return value, ok
}

func (c *Context) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.state[strings.TrimSpace(key)]
	if !ok {
		value, ok = c.data[strings.TrimSpace(key)]
	}
	return value, ok
}

func (c *Context) GetString(key string) string {
	if value, ok := c.Get(key); ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func (c *Context) AddInteraction(role, content string, metadata map[string]any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	entry := Interaction{
		ID:        len(c.history) + 1,
		Role:      strings.TrimSpace(role),
		Content:   strings.TrimSpace(content),
		Timestamp: time.Now().UTC(),
	}
	if len(metadata) > 0 {
		entry.Metadata = cloneAnyMap(metadata)
	}
	c.history = append(c.history, ContextHistoryEntry{Content: entry.Role + ": " + entry.Content})
	c.data["history"] = c.history
	c.state["history"] = c.history
	c.variables["history"] = c.history
}

func (c *Context) History() []Interaction {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Interaction, 0, len(c.history))
	for i, entry := range c.history {
		parts := strings.SplitN(entry.Content, ": ", 2)
		role := ""
		content := entry.Content
		if len(parts) == 2 {
			role = parts[0]
			content = parts[1]
		}
		out = append(out, Interaction{
			ID:        i + 1,
			Role:      role,
			Content:   content,
			Timestamp: time.Time{},
		})
	}
	return out
}

func (c *Context) GetFullHistory() ([]CompressedContextHistoryEntry, []Interaction) {
	if c == nil {
		return nil, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	keepRecent := 5
	recentStart := len(c.history) - keepRecent
	if recentStart < 0 {
		recentStart = 0
	}
	compressed := append([]CompressedContextHistoryEntry(nil), c.compressedHistory...)
	for _, entry := range c.history[:recentStart] {
		compressed = append(compressed, CompressedContextHistoryEntry{
			Content:          entry.Content,
			CompressedTokens: estimateTextTokens(entry.Content),
		})
	}
	recent := make([]Interaction, 0, len(c.history[recentStart:]))
	for i, entry := range c.history[recentStart:] {
		parts := strings.SplitN(entry.Content, ": ", 2)
		role := ""
		content := entry.Content
		if len(parts) == 2 {
			role = parts[0]
			content = parts[1]
		}
		recent = append(recent, Interaction{
			ID:      recentStart + i + 1,
			Role:    role,
			Content: content,
		})
	}
	return compressed, recent
}

func (c *Context) TrimHistory(keep int) {
	if c == nil {
		return
	}
	if keep < 0 {
		keep = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.history) <= keep {
		return
	}
	trimmed := append([]ContextHistoryEntry(nil), c.history[:len(c.history)-keep]...)
	for _, entry := range trimmed {
		c.compressedHistory = append(c.compressedHistory, CompressedContextHistoryEntry{
			Content:          entry.Content,
			CompressedTokens: estimateTextTokens(entry.Content),
		})
	}
	c.history = append([]ContextHistoryEntry(nil), c.history[len(c.history)-keep:]...)
}

func (c *Context) AppendCompressedContext(compressed CompressedContext) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := CompressedContextHistoryEntry{
		Content:          compressed.Summary,
		CompressedTokens: compressed.CompressedTokens,
	}
	if entry.CompressedTokens <= 0 {
		entry.CompressedTokens = estimateTextTokens(compressed.Summary)
	}
	if strings.TrimSpace(entry.Content) != "" {
		c.compressedHistory = append(c.compressedHistory, entry)
	}
}

func (c *Context) SetHandle(key string, value any) string {
	return c.setHandle(key, value, "")
}

func (c *Context) SetHandleScoped(key string, value any, scope string) string {
	return c.setHandle(key, value, scope)
}

func (c *Context) setHandle(key string, value any, scope string) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	if c.registry == nil {
		c.registry = NewObjectRegistry()
	}
	trimmed := strings.TrimSpace(key)
	var handle string
	if scope != "" {
		handle = c.registry.RegisterScoped(scope, value)
		c.handleScopes[trimmed] = scope
	} else {
		handle = c.registry.Register(value)
		delete(c.handleScopes, trimmed)
	}
	c.handles[trimmed] = handle
	c.state[trimmed] = handle
	c.data[trimmed] = handle
	return handle
}

func (c *Context) GetHandle(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	handle := c.handles[strings.TrimSpace(key)]
	registry := c.registry
	c.mu.RUnlock()
	if handle == "" || registry == nil {
		return nil, false
	}
	return registry.Lookup(handle)
}

func (c *Context) ClearHandleScope(scope string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.registry != nil {
		c.registry.ClearScope(scope)
	}
	for key, keyScope := range c.handleScopes {
		if keyScope != scope {
			continue
		}
		delete(c.handles, key)
		delete(c.handleScopes, key)
		delete(c.state, key)
		delete(c.data, key)
	}
}

func (c *Context) Registry() *ObjectRegistry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.registry == nil {
		c.registry = NewObjectRegistry()
	}
	return c.registry
}

func (c *Context) Clone() *Context {
	if c == nil {
		return NewContext()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	cloned := NewContext()
	for key, value := range c.state {
		cloned.state[key] = value
		cloned.data[key] = value
	}
	for key, value := range c.variables {
		cloned.variables[key] = value
	}
	for key, value := range c.knowledge {
		cloned.knowledge[key] = value
	}
	cloned.history = append([]ContextHistoryEntry(nil), c.history...)
	cloned.compressedHistory = append([]CompressedContextHistoryEntry(nil), c.compressedHistory...)
	cloned.handles = make(map[string]string, len(c.handles))
	for key, handle := range c.handles {
		cloned.handles[key] = handle
	}
	cloned.handleScopes = make(map[string]string, len(c.handleScopes))
	for key, scope := range c.handleScopes {
		cloned.handleScopes[key] = scope
	}
	if c.registry != nil {
		cloned.registry = c.registry.Clone()
	}
	cloned.baseline = c.snapshotLocked()
	return cloned
}

func (c *Context) Snapshot() *ContextSnapshot {
	if c == nil {
		return &ContextSnapshot{State: map[string]any{}, Variables: map[string]any{}, Knowledge: map[string]any{}, Data: map[string]any{}}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &ContextSnapshot{
		State:             cloneAnyMap(c.state),
		Variables:         cloneAnyMap(c.variables),
		Knowledge:         cloneAnyMap(c.knowledge),
		History:           append([]ContextHistoryEntry(nil), c.history...),
		CompressedHistory: append([]CompressedContextHistoryEntry(nil), c.compressedHistory...),
		Data:              cloneAnyMap(c.data),
	}
}

func (c *Context) Restore(snapshot *ContextSnapshot) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if snapshot == nil {
		return nil
	}
	c.state = cloneAnyMap(snapshot.State)
	c.variables = cloneAnyMap(snapshot.Variables)
	c.knowledge = cloneAnyMap(snapshot.Knowledge)
	c.history = append([]ContextHistoryEntry(nil), snapshot.History...)
	c.compressedHistory = append([]CompressedContextHistoryEntry(nil), snapshot.CompressedHistory...)
	c.data = cloneAnyMap(snapshot.Data)
	if c.state == nil {
		c.state = map[string]any{}
	}
	if c.variables == nil {
		c.variables = map[string]any{}
	}
	if c.knowledge == nil {
		c.knowledge = map[string]any{}
	}
	if c.data == nil {
		c.data = map[string]any{}
	}
	if c.handles == nil {
		c.handles = map[string]string{}
	}
	if c.handleScopes == nil {
		c.handleScopes = map[string]string{}
	}
	c.baseline = cloneContextSnapshot(snapshot)
	return nil
}

func (c *Context) snapshotLocked() *ContextSnapshot {
	if c == nil {
		return &ContextSnapshot{State: map[string]any{}, Variables: map[string]any{}, Knowledge: map[string]any{}, Data: map[string]any{}}
	}
	return &ContextSnapshot{
		State:             cloneAnyMap(c.state),
		Variables:         cloneAnyMap(c.variables),
		Knowledge:         cloneAnyMap(c.knowledge),
		History:           append([]ContextHistoryEntry(nil), c.history...),
		CompressedHistory: append([]CompressedContextHistoryEntry(nil), c.compressedHistory...),
		Data:              cloneAnyMap(c.data),
	}
}

func (c *Context) StateSnapshot() map[string]any {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneAnyMap(c.state)
}

func (c *Context) Merge(other *Context) {
	if c == nil || other == nil {
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()
	for key, value := range other.state {
		c.state[key] = value
		c.data[key] = value
	}
	for key, value := range other.variables {
		c.variables[key] = value
	}
	for key, value := range other.knowledge {
		c.knowledge[key] = value
	}
	c.history = append(c.history, other.history...)
	c.compressedHistory = append(c.compressedHistory, other.compressedHistory...)
	for key, value := range other.handles {
		c.handles[key] = value
	}
	for key, value := range other.handleScopes {
		c.handleScopes[key] = value
	}
	if other.registry != nil {
		if c.registry == nil {
			c.registry = other.registry.Clone()
		} else {
			for handle, value := range other.registry.items {
				c.registry.items[handle] = value
			}
			for scope, handles := range other.registry.scopes {
				if c.registry.scopes[scope] == nil {
					c.registry.scopes[scope] = map[string]struct{}{}
				}
				for handle := range handles {
					c.registry.scopes[scope][handle] = struct{}{}
				}
			}
			for handle, scope := range other.registry.handleToScope {
				c.registry.handleToScope[handle] = scope
			}
		}
	}
}

func (c *Context) BranchDelta() BranchContextDelta {
	if c == nil {
		return BranchContextDelta{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	current := c.snapshotLocked()
	base := c.baseline
	if base == nil {
		base = &ContextSnapshot{State: map[string]any{}, Variables: map[string]any{}, Knowledge: map[string]any{}, Data: map[string]any{}}
	}
	return diffContextSnapshots(base, current)
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneAnyValue(item)
		}
		return cloned
	default:
		return value
	}
}

func CloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cloned := *task
	if task.Context != nil {
		cloned.Context = make(map[string]any, len(task.Context))
		for key, value := range task.Context {
			cloned.Context[key] = value
		}
	}
	if task.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(task.Metadata))
		for key, value := range task.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}

func CapabilitySelectorFromAgentSpec(selector agentspec.CapabilitySelector) CapabilitySelector {
	out := CapabilitySelector{
		ID:                          selector.ID,
		Name:                        selector.Name,
		Kind:                        CapabilityKind(selector.Kind),
		Tags:                        append([]string{}, selector.Tags...),
		ExcludeTags:                 append([]string{}, selector.ExcludeTags...),
		CoordinationTaskTypes:       append([]string{}, selector.CoordinationTaskTypes...),
		CoordinationLongRunning:     cloneBoolPtr(selector.CoordinationLongRunning),
		CoordinationDirectInsertion: cloneBoolPtr(selector.CoordinationDirectInsertion),
	}
	if selector.RuntimeFamilies != nil {
		out.RuntimeFamilies = make([]CapabilityRuntimeFamily, 0, len(selector.RuntimeFamilies))
		for _, value := range selector.RuntimeFamilies {
			out.RuntimeFamilies = append(out.RuntimeFamilies, CapabilityRuntimeFamily(value))
		}
	}
	if selector.SourceScopes != nil {
		out.SourceScopes = make([]CapabilityScope, 0, len(selector.SourceScopes))
		for _, value := range selector.SourceScopes {
			out.SourceScopes = append(out.SourceScopes, CapabilityScope(value))
		}
	}
	if selector.TrustClasses != nil {
		out.TrustClasses = make([]TrustClass, 0, len(selector.TrustClasses))
		for _, value := range selector.TrustClasses {
			out.TrustClasses = append(out.TrustClasses, TrustClass(value))
		}
	}
	if selector.RiskClasses != nil {
		out.RiskClasses = make([]RiskClass, 0, len(selector.RiskClasses))
		for _, value := range selector.RiskClasses {
			out.RiskClasses = append(out.RiskClasses, RiskClass(value))
		}
	}
	if selector.EffectClasses != nil {
		out.EffectClasses = make([]EffectClass, 0, len(selector.EffectClasses))
		for _, value := range selector.EffectClasses {
			out.EffectClasses = append(out.EffectClasses, EffectClass(value))
		}
	}
	if selector.CoordinationRoles != nil {
		out.CoordinationRoles = make([]CoordinationRole, 0, len(selector.CoordinationRoles))
		for _, value := range selector.CoordinationRoles {
			out.CoordinationRoles = append(out.CoordinationRoles, CoordinationRole(value))
		}
	}
	if selector.CoordinationExecutionModes != nil {
		out.CoordinationExecutionModes = make([]CoordinationExecutionMode, 0, len(selector.CoordinationExecutionModes))
		for _, value := range selector.CoordinationExecutionModes {
			out.CoordinationExecutionModes = append(out.CoordinationExecutionModes, CoordinationExecutionMode(value))
		}
	}
	return out
}

func EffectiveAllowedCapabilitySelectors(spec *agentspec.AgentRuntimeSpec) []CapabilitySelector {
	if spec == nil || len(spec.AllowedCapabilities) == 0 {
		return nil
	}
	out := make([]CapabilitySelector, 0, len(spec.AllowedCapabilities))
	for _, selector := range spec.AllowedCapabilities {
		out = append(out, CapabilitySelectorFromAgentSpec(selector))
	}
	return out
}

func ValidateProviderPolicy(policy ProviderPolicy) error {
	return agentspec.ValidateProviderPolicy(policy)
}

func RuntimeSafetySpecFromAgentSpec(input *agentspec.RuntimeSafetySpec) *agentspec.RuntimeSafetySpec {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

func RuntimeSafetySpecToCore(input *agentspec.RuntimeSafetySpec) *RuntimeSafetySpec {
	if input == nil {
		return nil
	}
	return &RuntimeSafetySpec{
		MaxCallsPerCapability:     input.MaxCallsPerCapability,
		MaxCallsPerProvider:       input.MaxCallsPerProvider,
		MaxBytesPerSession:        input.MaxBytesPerSession,
		MaxOutputTokensSession:    input.MaxOutputTokensSession,
		MaxSubprocessesPerSession: input.MaxSubprocessesPerSession,
		MaxNetworkRequestsSession: input.MaxNetworkRequestsSession,
		RedactSensitiveMetadata:   input.RedactSensitiveMetadata,
	}
}

func cloneBoolPtr(input *bool) *bool {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}

func StringSliceFromContext(ctx *Context, key string) []string {
	if ctx == nil {
		return nil
	}
	raw, ok := ctx.Get(key)
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{text}
	}
}

func cloneContextSnapshot(snapshot *ContextSnapshot) *ContextSnapshot {
	if snapshot == nil {
		return nil
	}
	return &ContextSnapshot{
		State:             cloneAnyMap(snapshot.State),
		Variables:         cloneAnyMap(snapshot.Variables),
		Knowledge:         cloneAnyMap(snapshot.Knowledge),
		History:           append([]ContextHistoryEntry(nil), snapshot.History...),
		CompressedHistory: append([]CompressedContextHistoryEntry(nil), snapshot.CompressedHistory...),
		Data:              cloneAnyMap(snapshot.Data),
	}
}
