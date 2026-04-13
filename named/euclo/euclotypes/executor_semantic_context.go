package euclotypes

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// ExecutorSemanticContext is the pre-resolved semantic context bundle
// passed to executors before execution begins. All fields are optional;
// an empty bundle is valid and represents a cold-start with no semantic
// preloading.
//
// This type carries resolved content, not references. It is assembled
// by the session trigger and ExecutorFactory upstream of executor
// construction. Executors must not perform their own assembly of this
// bundle at runtime.
type ExecutorSemanticContext struct {
	core.AgentSemanticContext

	// Patterns is a slice of resolved pattern summaries from archaeo.
	// These are the actual pattern descriptions, not just IDs.
	Patterns []SemanticFindingSummary

	// Tensions is a slice of resolved tension summaries from archaeo.
	// Tensions with Severity == "high" or "medium" should surface as
	// initial issues in the Blackboard.
	Tensions []SemanticFindingSummary

	// LearningInteractions is a slice of resolved learning interaction
	// summaries. These carry confirmed design decisions and user-stated
	// constraints.
	LearningInteractions []SemanticFindingSummary

	// ActivePlanSummary is the resolved text summary of the current
	// living plan version, if one exists. Empty string if no active plan.
	ActivePlanSummary string
}

// SemanticFindingSummary is a compact representation of a semantic finding
// (pattern, tension, or learning interaction) suitable for injection into
// the blackboard or context.
type SemanticFindingSummary struct {
	ID       string
	Title    string
	Summary  string
	Kind     string
	Status   string
	Severity string
}

// IsEmpty returns true when the bundle contains no pre-resolved content.
func (e ExecutorSemanticContext) IsEmpty() bool {
	return e.AgentSemanticContext.IsEmpty() &&
		len(e.Patterns) == 0 &&
		len(e.Tensions) == 0 &&
		len(e.LearningInteractions) == 0 &&
		e.ActivePlanSummary == ""
}
