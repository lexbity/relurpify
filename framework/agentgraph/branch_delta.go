package agentgraph

import (
	"fmt"
	"reflect"
	"strings"
)

// BranchContextSideEffects captures non-state writes performed by a branch.
type BranchContextSideEffects struct {
	VariableWrites    map[string]any `json:"variable_writes,omitempty" yaml:"variable_writes,omitempty"`
	KnowledgeWrites   map[string]any `json:"knowledge_writes,omitempty" yaml:"knowledge_writes,omitempty"`
	HistoryChanged    bool           `json:"history_changed,omitempty" yaml:"history_changed,omitempty"`
	CompressedChanged bool           `json:"compressed_changed,omitempty" yaml:"compressed_changed,omitempty"`
	LogChanged        bool           `json:"log_changed,omitempty" yaml:"log_changed,omitempty"`
	PhaseChanged      bool           `json:"phase_changed,omitempty" yaml:"phase_changed,omitempty"`
}

// BranchContextDelta records the set of changes produced by an isolated branch.
type BranchContextDelta struct {
	StateWrites map[string]any           `json:"state_writes,omitempty" yaml:"state_writes,omitempty"`
	SideEffects BranchContextSideEffects `json:"side_effects,omitempty" yaml:"side_effects,omitempty"`
}

// BranchDeltaSet accumulates multiple branch deltas and merges them with basic
// conflict detection.
type BranchDeltaSet struct {
	entries []branchDeltaEntry
}

type branchDeltaEntry struct {
	label string
	delta BranchContextDelta
}

// NewBranchDeltaSet creates a delta set sized for the expected branch count.
func NewBranchDeltaSet(size int) *BranchDeltaSet {
	if size < 0 {
		size = 0
	}
	return &BranchDeltaSet{entries: make([]branchDeltaEntry, 0, size)}
}

// Add records a branch delta under a human-readable label.
func (s *BranchDeltaSet) Add(label string, delta BranchContextDelta) {
	if s == nil {
		return
	}
	s.entries = append(s.entries, branchDeltaEntry{label: label, delta: delta})
}

// Entries returns the delta entries for inspection.
func (s *BranchDeltaSet) Entries() []branchDeltaEntry {
	if s == nil {
		return nil
	}
	return s.entries
}

func mergeConflictError(kind, key string, existing, value any) error {
	return &mergeConflict{kind: kind, key: key, existing: valueString(existing), value: valueString(value)}
}

type mergeConflict struct {
	kind     string
	key      string
	existing string
	value    string
}

func (e *mergeConflict) Error() string {
	return "parallel branch merge conflict: " + e.kind + " key " + e.key + " values " + e.existing + " vs " + e.value
}

func valueString(v any) string {
	return strings.TrimSpace(fmt.Sprint(v))
}

// ContextSnapshot captures a point-in-time view of context state for diffing.
// This is used by branch delta computation to detect changes.
type ContextSnapshot struct {
	State             map[string]any
	Variables         map[string]any
	Knowledge         map[string]any
	History           []any
	CompressedHistory []any
}

// DiffContextSnapshots compares two context snapshots and returns the delta.
func DiffContextSnapshots(base, current *ContextSnapshot) BranchContextDelta {
	delta := BranchContextDelta{
		StateWrites: make(map[string]any),
		SideEffects: BranchContextSideEffects{
			VariableWrites:  make(map[string]any),
			KnowledgeWrites: make(map[string]any),
		},
	}
	if base == nil {
		base = &ContextSnapshot{}
	}
	if current == nil {
		return delta
	}
	for key, value := range current.State {
		if base.State == nil || !reflect.DeepEqual(base.State[key], value) {
			delta.StateWrites[key] = value
		}
	}
	for key, value := range current.Variables {
		if base.Variables == nil || !reflect.DeepEqual(base.Variables[key], value) {
			delta.SideEffects.VariableWrites[key] = value
		}
	}
	for key, value := range current.Knowledge {
		if base.Knowledge == nil || !reflect.DeepEqual(base.Knowledge[key], value) {
			delta.SideEffects.KnowledgeWrites[key] = value
		}
	}
	if len(current.History) != len(base.History) {
		delta.SideEffects.HistoryChanged = true
	}
	if len(current.CompressedHistory) != len(base.CompressedHistory) {
		delta.SideEffects.CompressedChanged = true
	}
	return delta
}

// ApplyTo applies the accumulated deltas to a context, merging branch changes.
func (s *BranchDeltaSet) ApplyTo(parent *Context) error {
	if s == nil || parent == nil {
		return nil
	}

	// Track written keys for conflict detection
	writtenKeys := make(map[string]string) // key -> branch label that first wrote it

	for _, entry := range s.entries {
		// Check for history mutation (either via SideEffects or _history key)
		if entry.delta.SideEffects.HistoryChanged {
			return fmt.Errorf("branch %s changed interaction history; use a custom merge policy if this is intentional", entry.label)
		}

		// Check for conflicts and apply writes
		for key, value := range entry.delta.StateWrites {
			// Special handling for _history key - treat as history mutation
			if key == "_history" {
				return fmt.Errorf("branch %s changed interaction history; use a custom merge policy if this is intentional", entry.label)
			}
			if _, ok := writtenKeys[key]; ok {
				// Conflict detected - same key written by multiple branches
				existingValue := (*parent)[key]
				return mergeConflictError("state", key, existingValue, value)
			}
			writtenKeys[key] = entry.label
			(*parent)[key] = value
		}
	}
	return nil
}
