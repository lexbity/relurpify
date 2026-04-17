package agentstate

import (
	"github.com/lexcodex/relurpify/framework/core"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type refEntry struct {
	key    string
	target *[]string
}

// SemanticInputBundleFromState builds a semantic input bundle from direct state
// keys used by capability_direct_run tests and related prepass flows.
func SemanticInputBundleFromState(state *core.Context) eucloruntime.SemanticInputBundle {
	inputs := eucloruntime.SemanticInputBundle{}
	if state == nil {
		return inputs
	}

	table := []refEntry{
		{key: "pattern_refs", target: &inputs.PatternRefs},
		{key: "tension_refs", target: &inputs.TensionRefs},
		{key: "prospective_refs", target: &inputs.ProspectiveRefs},
		{key: "convergence_refs", target: &inputs.ConvergenceRefs},
		{key: "request_provenance_refs", target: &inputs.RequestProvenanceRefs},
		{key: "learning_interaction_refs", target: &inputs.LearningInteractionRefs},
	}

	for _, entry := range table {
		if refs := stringSliceFromState(state, entry.key); len(refs) > 0 {
			*entry.target = append(*entry.target, refs...)
		}
	}

	for _, prefix := range []string{"archaeology", "debug"} {
		for _, entry := range table {
			key := prefix + "." + entry.key
			if refs := stringSliceFromState(state, key); len(refs) > 0 {
				*entry.target = append(*entry.target, refs...)
			}
		}
	}

	if id, ok := stringFromState(state, "workflow_id"); ok {
		inputs.WorkflowID = id
	}
	if id, ok := stringFromState(state, "exploration_id"); ok {
		inputs.ExplorationID = id
	}

	return inputs
}

// stringSliceFromState extracts a string slice from state by key.
func stringSliceFromState(state *core.Context, key string) []string {
	return stringSliceFromStateKey(state, key)
}

// stringSliceFromStateKey extracts a string slice from a specific state key.
// Handles both []string and []any (from JSON/YAML parsing) formats.
func stringSliceFromStateKey(state *core.Context, key string) []string {
	if raw, ok := state.Get(key); ok && raw != nil {
		switch typed := raw.(type) {
		case []string:
			return append([]string(nil), typed...)
		case []any:
			var result []string
			for _, v := range typed {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		case string:
			if typed != "" {
				return []string{typed}
			}
		}
	}
	return nil
}

// stringFromState extracts a string value from state by key.
func stringFromState(state *core.Context, key string) (string, bool) {
	if raw, ok := state.Get(key); ok && raw != nil {
		if s, ok := raw.(string); ok {
			return s, true
		}
	}
	return "", false
}
