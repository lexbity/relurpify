package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Interaction captures a single turn of conversation or observation. Storing a
// timestamp and arbitrary metadata lets agents replay past reasoning, render
// transcripts, or build features like “explain how we got here” without needing
// to re-run the original tools/LLM calls.
type Interaction struct {
	ID        int                    `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CompressionEvent tracks compression actions applied to the context.
type CompressionEvent struct {
	Timestamp               time.Time `json:"timestamp"`
	InteractionsCompressed  int       `json:"interactions_compressed"`
	TokensSaved             int       `json:"tokens_saved"`
	CompressionMethod       string    `json:"compression_method"`
	StartInteractionID      int       `json:"start_interaction_id"`
	EndInteractionID        int       `json:"end_interaction_id"`
	CompressedSummaryTokens int       `json:"compressed_summary_tokens"`
}

// Context acts as the in-memory “blackboard” shared by nodes inside a graph.
// It separates information into three buckets:
//   - state: durable facts that should be visible to all downstream nodes
//   - variables: transient scratch data used by a single node/branch
//   - knowledge: derived/global insights cached for reuse
//
// The structure embeds a RWMutex because multiple goroutines (parallel graph
// branches) can touch it concurrently.
type Context struct {
	mu                sync.RWMutex
	state             map[string]interface{}
	variables         map[string]interface{}
	knowledge         map[string]interface{}
	history           []Interaction
	compressedHistory []CompressedContext
	compressionLog    []CompressionEvent
	interactionIDCtr  int
	phase             string
	maxHistory        int
	maxSnapshot       int
	registry          *ObjectRegistry
	stateShared       bool
	variablesShared   bool
	knowledgeShared   bool
	historyShared     bool
	compressedShared  bool
	logShared         bool
	dirtyState        map[string]struct{}
	dirtyVariables    map[string]struct{}
	dirtyKnowledge    map[string]struct{}
	historyDirty      bool
	compressedDirty   bool
	compressionDirty  bool
	phaseDirty        bool
}

// NewContext builds an empty execution context with sensible history limits so
// runaway tool chatter does not balloon memory usage.
func NewContext() *Context {
	return &Context{
		state:             make(map[string]interface{}),
		variables:         make(map[string]interface{}),
		knowledge:         make(map[string]interface{}),
		history:           make([]Interaction, 0),
		compressedHistory: make([]CompressedContext, 0),
		compressionLog:    make([]CompressionEvent, 0),
		phase:             "planning",
		maxHistory:        200,
		maxSnapshot:       32,
		registry:          NewObjectRegistry(),
		dirtyState:        make(map[string]struct{}),
		dirtyVariables:    make(map[string]struct{}),
		dirtyKnowledge:    make(map[string]struct{}),
	}
}

// SetExecutionPhase stores the current execution phase.
func (c *Context) SetExecutionPhase(phase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase = phase
	c.phaseDirty = true
}

// ExecutionPhase returns the current phase.
func (c *Context) ExecutionPhase() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.phase
}

// Get retrieves a value from the shared state.
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.state[key]
	return v, ok
}

// GetString retrieves a string value from the shared state.
func (c *Context) GetString(key string) string {
	value, _ := c.Get(key)
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// StateSnapshot returns a deep copy of graph-visible state for validation or debugging.
func (c *Context) StateSnapshot() map[string]interface{} {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return deepCopyMap(c.state)
}

// Set stores a value in the shared state.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureStateWritableLocked()
	c.state[key] = value
	c.dirtyState[key] = struct{}{}
}

// GetVariable returns a temporary variable.
func (c *Context) GetVariable(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.variables[key]
	return v, ok
}

// SetVariable stores a variable for scratch usage.
func (c *Context) SetVariable(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureVariablesWritableLocked()
	c.variables[key] = value
	c.dirtyVariables[key] = struct{}{}
}

// Merge copies another context into the current one. It is primarily used when
// parallel graph branches finish executing: each goroutine works on a clone and
// the winning data is merged back in the parent context to keep side effects
// deterministic.
func (c *Context) Merge(other *Context) {
	if other == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	type interactionKey struct {
		ID        int
		Role      string
		Content   string
		Timestamp time.Time
	}
	type compressedKey struct {
		Start       int
		End         int
		Compressed  time.Time
		OriginalTok int
	}
	type compressionEventKey struct {
		Start       int
		End         int
		Method      string
		Timestamp   time.Time
		SavedTokens int
	}

	existingInteractions := make(map[interactionKey]struct{}, len(c.history))
	for _, interaction := range c.history {
		existingInteractions[interactionKey{
			ID:        interaction.ID,
			Role:      interaction.Role,
			Content:   interaction.Content,
			Timestamp: interaction.Timestamp,
		}] = struct{}{}
	}

	c.ensureStateWritableLocked()
	c.ensureVariablesWritableLocked()
	c.ensureKnowledgeWritableLocked()
	c.ensureHistoryWritableLocked()
	c.ensureCompressedWritableLocked()
	c.ensureLogWritableLocked()

	for k, v := range other.state {
		c.state[k] = v
		c.dirtyState[k] = struct{}{}
	}
	for k, v := range other.variables {
		c.variables[k] = v
		c.dirtyVariables[k] = struct{}{}
	}
	for k, v := range other.knowledge {
		c.knowledge[k] = v
		c.dirtyKnowledge[k] = struct{}{}
	}
	for _, interaction := range other.history {
		key := interactionKey{
			ID:        interaction.ID,
			Role:      interaction.Role,
			Content:   interaction.Content,
			Timestamp: interaction.Timestamp,
		}
		if _, ok := existingInteractions[key]; ok {
			continue
		}
		existingInteractions[key] = struct{}{}
		c.history = append(c.history, interaction)
		c.historyDirty = true
	}

	existingCompressed := make(map[compressedKey]struct{}, len(c.compressedHistory))
	for _, cc := range c.compressedHistory {
		existingCompressed[compressedKey{
			Start:       cc.StartInteractionID,
			End:         cc.EndInteractionID,
			Compressed:  cc.CompressedAt,
			OriginalTok: cc.OriginalTokens,
		}] = struct{}{}
	}
	for _, cc := range other.compressedHistory {
		key := compressedKey{
			Start:       cc.StartInteractionID,
			End:         cc.EndInteractionID,
			Compressed:  cc.CompressedAt,
			OriginalTok: cc.OriginalTokens,
		}
		if _, ok := existingCompressed[key]; ok {
			continue
		}
		existingCompressed[key] = struct{}{}
		c.compressedHistory = append(c.compressedHistory, cc)
		c.compressedDirty = true
	}

	existingEvents := make(map[compressionEventKey]struct{}, len(c.compressionLog))
	for _, event := range c.compressionLog {
		existingEvents[compressionEventKey{
			Start:       event.StartInteractionID,
			End:         event.EndInteractionID,
			Method:      event.CompressionMethod,
			Timestamp:   event.Timestamp,
			SavedTokens: event.TokensSaved,
		}] = struct{}{}
	}
	for _, event := range other.compressionLog {
		key := compressionEventKey{
			Start:       event.StartInteractionID,
			End:         event.EndInteractionID,
			Method:      event.CompressionMethod,
			Timestamp:   event.Timestamp,
			SavedTokens: event.TokensSaved,
		}
		if _, ok := existingEvents[key]; ok {
			continue
		}
		existingEvents[key] = struct{}{}
		c.compressionLog = append(c.compressionLog, event)
		c.compressionDirty = true
	}
	if other.interactionIDCtr > c.interactionIDCtr {
		c.interactionIDCtr = other.interactionIDCtr
	}
	c.smartTruncateHistoryLocked()
}

// Clone returns a clone of the context suitable for speculative work in
// separate goroutines. Map-backed state is deep-copied so nested mutations in a
// branch cannot leak back into the parent; history-oriented slices remain
// copy-on-write because they are append-dominated and much larger in practice.
func (c *Context) Clone() *Context {
	c.mu.Lock()
	defer c.mu.Unlock()

	clone := NewContext()
	clone.state = deepCopyMap(c.state)
	clone.variables = deepCopyMap(c.variables)
	clone.knowledge = deepCopyMap(c.knowledge)
	clone.history = c.history
	clone.compressedHistory = c.compressedHistory
	clone.compressionLog = c.compressionLog
	clone.interactionIDCtr = c.interactionIDCtr
	clone.phase = c.phase
	clone.maxHistory = c.maxHistory
	clone.maxSnapshot = c.maxSnapshot
	clone.registry = c.registry
	clone.stateShared = false
	clone.variablesShared = false
	clone.knowledgeShared = false
	clone.historyShared = true
	clone.compressedShared = true
	clone.logShared = true
	c.historyShared = true
	c.compressedShared = true
	c.logShared = true
	clone.resetDirtyTrackingLocked()
	return clone
}

// ContextSnapshot is a serializable snapshot of Context.
type ContextSnapshot struct {
	State                map[string]interface{} `json:"state"`
	Variables            map[string]interface{} `json:"variables"`
	Knowledge            map[string]interface{} `json:"knowledge"`
	History              []Interaction          `json:"history"`
	CompressedHistory    []CompressedContext    `json:"compressed_history,omitempty"`
	CompressionLog       []CompressionEvent     `json:"compression_log,omitempty"`
	InteractionIDCounter int                    `json:"interaction_id_counter"`
	Phase                string                 `json:"phase"`
}

// Snapshot captures the context for rollback.
func (c *Context) Snapshot() *ContextSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshot := &ContextSnapshot{
		State:                deepCopyMap(c.state),
		Variables:            deepCopyMap(c.variables),
		Knowledge:            deepCopyMap(c.knowledge),
		History:              append([]Interaction(nil), c.history...),
		CompressedHistory:    append([]CompressedContext(nil), c.compressedHistory...),
		CompressionLog:       append([]CompressionEvent(nil), c.compressionLog...),
		InteractionIDCounter: c.interactionIDCtr,
		Phase:                c.phase,
	}
	return snapshot
}

// Restore puts the context back to a snapshot. The method intentionally
// overwrites every section instead of mutating in place to avoid sharing map
// references with stale snapshots.
func (c *Context) Restore(snapshot *ContextSnapshot) error {
	if snapshot == nil {
		return errors.New("nil snapshot")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = snapshot.State
	c.variables = snapshot.Variables
	c.knowledge = snapshot.Knowledge
	c.history = snapshot.History
	c.compressedHistory = snapshot.CompressedHistory
	c.compressionLog = snapshot.CompressionLog
	c.interactionIDCtr = snapshot.InteractionIDCounter
	c.phase = snapshot.Phase
	c.stateShared = false
	c.variablesShared = false
	c.knowledgeShared = false
	c.historyShared = false
	c.compressedShared = false
	c.logShared = false
	c.resetDirtyTrackingLocked()
	c.smartTruncateHistoryLocked()
	return nil
}

// MarshalJSON ensures the context is serializable.
func (c *Context) MarshalJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.Marshal(&ContextSnapshot{
		State:                c.state,
		Variables:            c.variables,
		Knowledge:            c.knowledge,
		History:              c.history,
		CompressedHistory:    c.compressedHistory,
		CompressionLog:       c.compressionLog,
		InteractionIDCounter: c.interactionIDCtr,
		Phase:                c.phase,
	})
}

// UnmarshalJSON supports loading context from disk.
func (c *Context) UnmarshalJSON(data []byte) error {
	var snapshot ContextSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	return c.Restore(&snapshot)
}

// Registry returns the shared object registry used for non-serializable data.
func (c *Context) Registry() *ObjectRegistry {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registry
}

// SetHandle stores a registry handle in the context state and returns it.
func (c *Context) SetHandle(key string, value interface{}) string {
	if c == nil {
		return ""
	}
	handle := ""
	if registry := c.Registry(); registry != nil {
		handle = registry.Register(value)
	}
	c.Set(key, handle)
	return handle
}

// SetHandleScoped stores a registry handle scoped for cleanup.
func (c *Context) SetHandleScoped(key string, value interface{}, scope string) string {
	if c == nil {
		return ""
	}
	handle := ""
	if registry := c.Registry(); registry != nil {
		handle = registry.RegisterScoped(scope, value)
	}
	c.Set(key, handle)
	return handle
}

// GetHandle resolves a registry handle stored in the context state.
func (c *Context) GetHandle(key string) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	raw, _ := c.Get(key)
	handle, ok := raw.(string)
	if !ok || handle == "" {
		return nil, false
	}
	if registry := c.Registry(); registry != nil {
		return registry.Lookup(handle)
	}
	return nil, false
}

// ClearHandleScope removes all scoped handles from the registry.
func (c *Context) ClearHandleScope(scope string) {
	if c == nil || scope == "" {
		return
	}
	if registry := c.Registry(); registry != nil {
		registry.ClearScope(scope)
	}
}

// NewContextFromSnapshot rebuilds a context from a serializable snapshot while
// preserving the shared object registry for non-serializable handles.
func NewContextFromSnapshot(snapshot *ContextSnapshot, registry *ObjectRegistry) *Context {
	ctx := NewContext()
	if snapshot != nil {
		_ = ctx.Restore(snapshot)
	}
	if registry != nil {
		ctx.registry = registry
	}
	return ctx
}

// DirtyContextDelta summarizes mutations applied since the context was
// created, cloned, restored, or explicitly reset.
type DirtyContextDelta struct {
	StateValues       map[string]interface{}
	VariableValues    map[string]interface{}
	KnowledgeValues   map[string]interface{}
	HistoryChanged    bool
	CompressedChanged bool
	LogChanged        bool
	PhaseChanged      bool
}

// DirtyDelta reports the currently tracked mutations.
func (c *Context) DirtyDelta() DirtyContextDelta {
	if c == nil {
		return DirtyContextDelta{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	delta := DirtyContextDelta{
		StateValues:       make(map[string]interface{}, len(c.dirtyState)),
		VariableValues:    make(map[string]interface{}, len(c.dirtyVariables)),
		KnowledgeValues:   make(map[string]interface{}, len(c.dirtyKnowledge)),
		HistoryChanged:    c.historyDirty,
		CompressedChanged: c.compressedDirty,
		LogChanged:        c.compressionDirty,
		PhaseChanged:      c.phaseDirty,
	}
	for key := range c.dirtyState {
		delta.StateValues[key] = deepCopyValue(c.state[key])
	}
	for key := range c.dirtyVariables {
		delta.VariableValues[key] = deepCopyValue(c.variables[key])
	}
	for key := range c.dirtyKnowledge {
		delta.KnowledgeValues[key] = deepCopyValue(c.knowledge[key])
	}
	return delta
}

func (c *Context) resetDirtyTrackingLocked() {
	c.dirtyState = make(map[string]struct{})
	c.dirtyVariables = make(map[string]struct{})
	c.dirtyKnowledge = make(map[string]struct{})
	c.historyDirty = false
	c.compressedDirty = false
	c.compressionDirty = false
	c.phaseDirty = false
}

func (c *Context) ensureStateWritableLocked() {
	if !c.stateShared {
		return
	}
	c.state = shallowCopyMap(c.state)
	c.stateShared = false
}

func (c *Context) ensureVariablesWritableLocked() {
	if !c.variablesShared {
		return
	}
	c.variables = shallowCopyMap(c.variables)
	c.variablesShared = false
}

func (c *Context) ensureKnowledgeWritableLocked() {
	if !c.knowledgeShared {
		return
	}
	c.knowledge = shallowCopyMap(c.knowledge)
	c.knowledgeShared = false
}

func (c *Context) ensureHistoryWritableLocked() {
	if !c.historyShared {
		return
	}
	c.history = append([]Interaction(nil), c.history...)
	c.historyShared = false
}

func (c *Context) ensureCompressedWritableLocked() {
	if !c.compressedShared {
		return
	}
	c.compressedHistory = append([]CompressedContext(nil), c.compressedHistory...)
	c.compressedShared = false
}

func (c *Context) ensureLogWritableLocked() {
	if !c.logShared {
		return
	}
	c.compressionLog = append([]CompressionEvent(nil), c.compressionLog...)
	c.logShared = false
}

func shallowCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

func deepCopyValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return deepCopyMap(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = deepCopyValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case map[string]string:
		out := make(map[string]string, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	case map[string]int:
		out := make(map[string]int, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	default:
		return value
	}
}

// AddInteraction appends to the conversation history.
func (c *Context) AddInteraction(role, content string, metadata map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureHistoryWritableLocked()
	id := c.interactionIDCtr
	c.interactionIDCtr++
	c.history = append(c.history, Interaction{
		ID:        id,
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
	c.historyDirty = true
	c.smartTruncateHistoryLocked()
}

// History returns the accumulated conversation history.
func (c *Context) History() []Interaction {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Interaction(nil), c.history...)
}

// TrimHistory keeps only the most recent interactions.
func (c *Context) TrimHistory(keep int) {
	if keep <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.history) <= keep {
		return
	}
	c.ensureHistoryWritableLocked()
	start := len(c.history) - keep
	c.history = append([]Interaction(nil), c.history[start:]...)
	c.historyDirty = true
}

// LatestInteraction returns the most recent interaction if any.
func (c *Context) LatestInteraction() (Interaction, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.history) == 0 {
		return Interaction{}, false
	}
	return c.history[len(c.history)-1], true
}

// smartTruncateHistoryLocked keeps the conversation history bounded while still
// preserving the very first message (usually the task instruction). The oldest
// middle portion is dropped so that downstream reasoning retains enough
// context without exhausting memory.
func (c *Context) smartTruncateHistoryLocked() {
	if len(c.history) <= c.maxHistory {
		return
	}
	start := len(c.history) - c.maxHistory
	c.history = append(c.history[:1], c.history[start:]...)
}

// SetKnowledge stores derived information available to all nodes.
func (c *Context) SetKnowledge(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureKnowledgeWritableLocked()
	c.knowledge[key] = value
	c.dirtyKnowledge[key] = struct{}{}
}

// GetKnowledge retrieves derived info.
func (c *Context) GetKnowledge(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.knowledge[key]
	return val, ok
}

// CompressHistory summarizes older interactions while keeping the recent tail.
func (c *Context) CompressHistory(keepRecentCount int, llm LanguageModel, strategy CompressionStrategy) error {
	if strategy == nil {
		return fmt.Errorf("compression strategy required")
	}
	c.mu.RLock()
	if len(c.history) <= keepRecentCount {
		c.mu.RUnlock()
		return nil
	}
	compressibleCount := len(c.history) - keepRecentCount
	toCompress := append([]Interaction(nil), c.history[:compressibleCount]...)
	startID := toCompress[0].ID
	endID := toCompress[len(toCompress)-1].ID
	c.mu.RUnlock()

	compressed, err := strategy.Compress(toCompress, llm)
	if err != nil {
		return err
	}
	if compressed.OriginalTokens == 0 {
		compressed.OriginalTokens = estimateTokens(toCompress)
	}
	if compressed.CompressedTokens == 0 {
		compressed.CompressedTokens = strategy.EstimateTokens(compressed)
	}
	compressed.StartInteractionID = startID
	compressed.EndInteractionID = endID

	event := CompressionEvent{
		Timestamp:               time.Now().UTC(),
		InteractionsCompressed:  len(toCompress),
		TokensSaved:             compressed.OriginalTokens - compressed.CompressedTokens,
		CompressionMethod:       fmt.Sprintf("%T", strategy),
		StartInteractionID:      startID,
		EndInteractionID:        endID,
		CompressedSummaryTokens: compressed.CompressedTokens,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if compressibleCount > len(c.history) {
		compressibleCount = len(c.history)
	}
	c.ensureHistoryWritableLocked()
	c.ensureCompressedWritableLocked()
	c.ensureLogWritableLocked()
	c.history = append([]Interaction(nil), c.history[compressibleCount:]...)
	c.compressedHistory = append(c.compressedHistory, *compressed)
	c.compressionLog = append(c.compressionLog, event)
	c.historyDirty = true
	c.compressedDirty = true
	c.compressionDirty = true
	return nil
}

// GetFullHistory returns compressed segments plus current tail.
func (c *Context) GetFullHistory() ([]CompressedContext, []Interaction) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]CompressedContext(nil), c.compressedHistory...), append([]Interaction(nil), c.history...)
}

// AppendCompressedContext appends a compressed history entry.
func (c *Context) AppendCompressedContext(cc CompressedContext) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureCompressedWritableLocked()
	c.compressedHistory = append(c.compressedHistory, cc)
	c.compressedDirty = true
}

// GetContextForLLM renders the context as a string suitable for prompts.
func (c *Context) GetContextForLLM() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var sb strings.Builder
	if len(c.compressedHistory) > 0 {
		sb.WriteString("=== Previous Context (Compressed) ===\n")
		for _, cc := range c.compressedHistory {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", cc.Summary))
			sb.WriteString("Key Facts:\n")
			for _, fact := range cc.KeyFacts {
				sb.WriteString(fmt.Sprintf("  - [%s] %s\n", fact.Type, fact.Content))
			}
			sb.WriteString("\n")
		}
	}
	if len(c.history) > 0 {
		sb.WriteString("=== Recent Interactions ===\n")
		for _, interaction := range c.history {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", interaction.Role, interaction.Content))
		}
	}
	return sb.String()
}

// GetCompressionStats aggregates compression metrics.
func (c *Context) GetCompressionStats() CompressionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	totalInteractions := 0
	totalTokensSaved := 0
	for _, event := range c.compressionLog {
		totalInteractions += event.InteractionsCompressed
		totalTokensSaved += event.TokensSaved
	}
	return CompressionStats{
		TotalInteractionsCompressed: totalInteractions,
		TotalTokensSaved:            totalTokensSaved,
		CompressionEvents:           len(c.compressionLog),
		CurrentHistorySize:          len(c.history),
		CompressedChunks:            len(c.compressedHistory),
	}
}

// CompressionStats summarizes compression activity.
type CompressionStats struct {
	TotalInteractionsCompressed int
	TotalTokensSaved            int
	CompressionEvents           int
	CurrentHistorySize          int
	CompressedChunks            int
}
