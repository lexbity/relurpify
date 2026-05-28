package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// GetClarificationNextThoughtRecipeID returns the next thoughtrecipe to run after
// a clarification step completes.
func GetClarificationNextThoughtRecipeID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyClarificationNextThoughtRecipeID)
	return v
}

// SetClarificationNextThoughtRecipeID stores the next thoughtrecipe for the
// clarification handoff.
func SetClarificationNextThoughtRecipeID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyClarificationNextThoughtRecipeID, id)
}

// GetClarificationUnresolved reports whether the clarification loop could not
// find a handoff target.
func GetClarificationUnresolved(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyClarificationUnresolved)
	return v
}

// SetClarificationUnresolved marks that the clarification handoff target is missing.
func SetClarificationUnresolved(env *contextdata.Envelope, unresolved bool) {
	contextdata.SetTyped(env, KeyClarificationUnresolved, unresolved)
}

// GetClarificationUnresolvedReason returns the human-readable reason the
// clarification loop was unresolved.
func GetClarificationUnresolvedReason(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyClarificationUnresolvedReason)
	return v
}

// SetClarificationUnresolvedReason stores the reason string.
func SetClarificationUnresolvedReason(env *contextdata.Envelope, reason string) {
	contextdata.SetTyped(env, KeyClarificationUnresolvedReason, reason)
}

// GetClarificationGateResult returns the gate result payload written by the
// clarification capability.
func GetClarificationGateResult(env *contextdata.Envelope) (map[string]any, bool) {
	return contextdata.GetTyped[map[string]any](env, KeyClarificationGateResult)
}

// SetClarificationGateResult stores the gate result payload.
func SetClarificationGateResult(env *contextdata.Envelope, result map[string]any) {
	contextdata.SetTyped(env, KeyClarificationGateResult, result)
}
