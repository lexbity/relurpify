package core

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

// ApplyTo merges the delta set into the parent context.
func (s *BranchDeltaSet) ApplyTo(parent *Context) error {
	if s == nil || parent == nil {
		return nil
	}
	merged := NewContext()
	applied := map[string]any{}
	applyMap := func(kind string, writes map[string]any, set func(string, any)) error {
		for key, value := range writes {
			compound := kind + ":" + key
			if existing, ok := applied[compound]; ok {
				if !reflect.DeepEqual(existing, value) {
					return mergeConflictError(kind, key, existing, value)
				}
				continue
			}
			applied[compound] = value
			set(key, value)
		}
		return nil
	}
	for _, entry := range s.entries {
		if err := applyMap("state", entry.delta.StateWrites, merged.Set); err != nil {
			return err
		}
		if err := applyMap("variable", entry.delta.SideEffects.VariableWrites, merged.SetVariable); err != nil {
			return err
		}
		if err := applyMap("knowledge", entry.delta.SideEffects.KnowledgeWrites, merged.SetKnowledge); err != nil {
			return err
		}
		if entry.delta.SideEffects.HistoryChanged || entry.delta.SideEffects.CompressedChanged || entry.delta.SideEffects.LogChanged || entry.delta.SideEffects.PhaseChanged {
			reason := "history/compression/log/phase changed"
			switch {
			case entry.delta.SideEffects.HistoryChanged:
				reason = "changed interaction history"
			case entry.delta.SideEffects.CompressedChanged:
				reason = "changed compressed context"
			case entry.delta.SideEffects.LogChanged:
				reason = "changed log state"
			case entry.delta.SideEffects.PhaseChanged:
				reason = "changed phase state"
			}
			return mergeConflictError("side-effects", entry.label, "non-mutating branch policy", reason)
		}
	}
	parent.Merge(merged)
	return nil
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

func diffContextSnapshots(base, current *ContextSnapshot) BranchContextDelta {
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
