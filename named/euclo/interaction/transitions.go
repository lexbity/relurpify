package interaction

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// TransitionTrigger describes what caused a mode transition.
type TransitionTrigger string

const (
	TriggerUserRequest         TransitionTrigger = "user_request"
	TriggerVerificationFailure TransitionTrigger = "verification_failure"
	TriggerScopeExpansion      TransitionTrigger = "scope_expansion"
	TriggerEvidenceRequired    TransitionTrigger = "evidence_required"
	TriggerConstraintViolation TransitionTrigger = "constraint_violation"
	TriggerPhaseCompletion     TransitionTrigger = "phase_completion"
)

// TransitionRule defines a single transition between modes.
type TransitionRule struct {
	FromMode      string
	ToMode        string
	Trigger       TransitionTrigger
	ArtifactCarry []euclotypes.ArtifactKind
	Description   string
	// Condition is an optional predicate. If non-nil, the rule only fires when true.
	Condition func(state map[string]any, artifacts *ArtifactBundle) bool
}

// TransitionRuleSet holds all transition rules and evaluates them.
type TransitionRuleSet struct {
	rules []TransitionRule
}

// NewTransitionRuleSet creates an empty rule set.
func NewTransitionRuleSet() *TransitionRuleSet {
	return &TransitionRuleSet{}
}

// Add appends a rule to the set.
func (rs *TransitionRuleSet) Add(rule TransitionRule) {
	rs.rules = append(rs.rules, rule)
}

// Evaluate finds the first matching rule for the given mode and trigger.
// Returns nil if no rule matches.
func (rs *TransitionRuleSet) Evaluate(fromMode string, trigger TransitionTrigger, state map[string]any, artifacts *ArtifactBundle) *TransitionRule {
	for i := range rs.rules {
		r := &rs.rules[i]
		if r.FromMode != fromMode && r.FromMode != "*" {
			continue
		}
		if r.Trigger != trigger {
			continue
		}
		if r.Condition != nil && !r.Condition(state, artifacts) {
			continue
		}
		return r
	}
	return nil
}

// RulesFrom returns all rules originating from the given mode.
func (rs *TransitionRuleSet) RulesFrom(fromMode string) []TransitionRule {
	var out []TransitionRule
	for _, r := range rs.rules {
		if r.FromMode == fromMode || r.FromMode == "*" {
			out = append(out, r)
		}
	}
	return out
}

// RulesTo returns all rules targeting the given mode.
func (rs *TransitionRuleSet) RulesTo(toMode string) []TransitionRule {
	var out []TransitionRule
	for _, r := range rs.rules {
		if r.ToMode == toMode {
			out = append(out, r)
		}
	}
	return out
}

// All returns all rules.
func (rs *TransitionRuleSet) All() []TransitionRule {
	return rs.rules
}

// ──────────────────────────────────────────────────────────────
// Canonical transition rules
// ──────────────────────────────────────────────────────────────

// DefaultTransitionRules returns the canonical set of mode transition rules.
func DefaultTransitionRules() *TransitionRuleSet {
	rs := NewTransitionRuleSet()

	// code → debug: verification failure threshold.
	rs.Add(TransitionRule{
		FromMode:      "code",
		ToMode:        "debug",
		Trigger:       TriggerVerificationFailure,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore, euclotypes.ArtifactKindEditIntent, euclotypes.ArtifactKindVerification},
		Description:   "Verification failed repeatedly — switch to investigation",
		Condition: func(state map[string]any, _ *ArtifactBundle) bool {
			count, _ := state["verify.failure_count"].(int)
			return count >= 2
		},
	})

	// code → planning: scope expansion.
	rs.Add(TransitionRule{
		FromMode:      "code",
		ToMode:        "planning",
		Trigger:       TriggerScopeExpansion,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		Description:   "This is bigger than expected — plan before proceeding",
		Condition: func(state map[string]any, _ *ArtifactBundle) bool {
			scope, _ := state["understand.proposal"].(ProposalContent)
			return len(scope.Scope) > 5
		},
	})

	// code → planning: user request.
	rs.Add(TransitionRule{
		FromMode:      "code",
		ToMode:        "planning",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		Description:   "User requested planning before code",
	})

	// debug → code: user request or fix proposal accepted.
	rs.Add(TransitionRule{
		FromMode:      "debug",
		ToMode:        "code",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindExplore},
		Description:   "Root cause identified — apply the fix",
	})

	// debug → code: fix proposal accepted (phase completion).
	rs.Add(TransitionRule{
		FromMode:      "debug",
		ToMode:        "code",
		Trigger:       TriggerPhaseCompletion,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindExplore},
		Description:   "Fix proposal accepted — apply the fix",
		Condition: func(state map[string]any, _ *ArtifactBundle) bool {
			resp, _ := state["propose_fix.response"].(string)
			return resp == "apply"
		},
	})

	// planning → code: plan committed.
	rs.Add(TransitionRule{
		FromMode:      "planning",
		ToMode:        "code",
		Trigger:       TriggerPhaseCompletion,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
		Description:   "Plan ready — execute it",
	})

	// planning → code: user request.
	rs.Add(TransitionRule{
		FromMode:      "planning",
		ToMode:        "code",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
		Description:   "User requested to start coding",
	})

	// tdd → code: user request.
	rs.Add(TransitionRule{
		FromMode:      "tdd",
		ToMode:        "code",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindVerification},
		Description:   "Switch to implementation with test constraint",
	})

	// review → code: user request.
	rs.Add(TransitionRule{
		FromMode:      "review",
		ToMode:        "code",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
		Description:   "Apply review findings as fixes",
	})

	// any → planning: user request.
	rs.Add(TransitionRule{
		FromMode:      "*",
		ToMode:        "planning",
		Trigger:       TriggerUserRequest,
		ArtifactCarry: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		Description:   "Step back and plan",
	})

	return rs
}

// ──────────────────────────────────────────────────────────────
// Return transition stack
// ──────────────────────────────────────────────────────────────

// TransitionFrame captures the state of a mode before transition,
// enabling return to the previous mode after the target mode completes.
type TransitionFrame struct {
	Mode             string
	Phase            string
	InteractionState map[string]any
	ReturnArtifacts  []euclotypes.ArtifactKind
}

// TransitionStack is a LIFO stack of transition frames for round-trip transitions.
type TransitionStack struct {
	stack []TransitionFrame
}

// NewTransitionStack creates an empty stack.
func NewTransitionStack() *TransitionStack {
	return &TransitionStack{}
}

// Push saves a transition frame onto the stack.
func (ts *TransitionStack) Push(frame TransitionFrame) {
	ts.stack = append(ts.stack, frame)
}

// Pop removes and returns the top frame, or nil if empty.
func (ts *TransitionStack) Pop() *TransitionFrame {
	if len(ts.stack) == 0 {
		return nil
	}
	top := ts.stack[len(ts.stack)-1]
	ts.stack = ts.stack[:len(ts.stack)-1]
	return &top
}

// Peek returns the top frame without removing it, or nil if empty.
func (ts *TransitionStack) Peek() *TransitionFrame {
	if len(ts.stack) == 0 {
		return nil
	}
	return &ts.stack[len(ts.stack)-1]
}

// Depth returns the number of frames on the stack.
func (ts *TransitionStack) Depth() int {
	return len(ts.stack)
}

// IsEmpty returns true when the stack has no frames.
func (ts *TransitionStack) IsEmpty() bool {
	return len(ts.stack) == 0
}

// CollectReturnArtifacts filters the given bundle for artifacts that should
// be carried back to the return mode, as specified by the top frame's
// ReturnArtifacts list.
func (ts *TransitionStack) CollectReturnArtifacts(bundle *ArtifactBundle) []euclotypes.Artifact {
	top := ts.Peek()
	if top == nil || bundle == nil {
		return nil
	}
	var out []euclotypes.Artifact
	for _, kind := range top.ReturnArtifacts {
		out = append(out, bundle.OfKind(kind)...)
	}
	return out
}
