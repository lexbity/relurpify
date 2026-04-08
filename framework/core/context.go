package core

import (
	"fmt"
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

// MergeConflictRecord tracks a conflict that occurred during context merge.
type MergeConflictRecord struct {
	Key             string    `json:"key"`
	ConflictArea    string    `json:"conflict_area"` // "state", "variables", or "knowledge"
	LosingValueHash string    `json:"losing_value_hash"`
	Timestamp       time.Time `json:"timestamp"`
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
	parentState       map[string]interface{}
	variables         map[string]interface{}
	parentVariables   map[string]interface{}
	knowledge         map[string]interface{}
	parentKnowledge   map[string]interface{}
	history           []Interaction
	compressedHistory []CompressedContext
	compressionLog    []CompressionEvent
	mergeConflicts    []MergeConflictRecord
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
		parentState:       nil,
		variables:         make(map[string]interface{}),
		parentVariables:   nil,
		knowledge:         make(map[string]interface{}),
		parentKnowledge:   nil,
		history:           make([]Interaction, 0),
		compressedHistory: make([]CompressedContext, 0),
		compressionLog:    make([]CompressionEvent, 0),
		mergeConflicts:    make([]MergeConflictRecord, 0),
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
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.state[key]
	if ok {
		return v, true
	}
	if c.parentState != nil {
		v, ok = c.parentState[key]
		if ok {
			v = deepCopyValue(v)
			c.state[key] = v
		}
	}
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
	return deepCopyMap(c.materializedStateLocked())
}

// Set stores a value in the shared state.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state[key] = value
	c.dirtyState[key] = struct{}{}
}

// GetVariable returns a temporary variable.
func (c *Context) GetVariable(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.variables[key]
	if ok {
		return v, true
	}
	if c.parentVariables != nil {
		v, ok = c.parentVariables[key]
		if ok {
			v = deepCopyValue(v)
			c.variables[key] = v
		}
	}
	return v, ok
}

// SetVariable stores a variable for scratch usage.
func (c *Context) SetVariable(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.variables[key] = value
	c.dirtyVariables[key] = struct{}{}
}
