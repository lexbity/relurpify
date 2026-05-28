package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// Execution kind values written to KeyExecutionKind.
const (
	ExecutionKindCapability    = "capability"
	ExecutionKindThoughtRecipe = "thoughtrecipe"
)

// GetExecutionKind returns "capability" or "thoughtrecipe" depending on which
// executor is currently active.
func GetExecutionKind(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyExecutionKind)
	return v
}

// SetExecutionKind stores the active execution kind.
func SetExecutionKind(env *contextdata.Envelope, kind string) {
	contextdata.SetTyped(env, KeyExecutionKind, kind)
}

// GetExecutionCapabilityID returns the ID of the capability being executed, if any.
func GetExecutionCapabilityID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyExecutionCapabilityID)
	return v
}

// SetExecutionCapabilityID stores the capability ID being executed.
func SetExecutionCapabilityID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyExecutionCapabilityID, id)
}

// GetExecutionCompleted reports whether the current executor finished successfully.
func GetExecutionCompleted(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyExecutionCompleted)
	return v
}

// SetExecutionCompleted records whether the executor finished successfully.
func SetExecutionCompleted(env *contextdata.Envelope, ok bool) {
	contextdata.SetTyped(env, KeyExecutionCompleted, ok)
}

// GetExecutionThoughtRecipeID returns the ID of the thought recipe being executed, if any.
func GetExecutionThoughtRecipeID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyExecutionThoughtRecipe)
	return v
}

// SetExecutionThoughtRecipeID stores the thought recipe ID being executed.
func SetExecutionThoughtRecipeID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyExecutionThoughtRecipe, id)
}

// GetDone reports whether the euclo execution cycle has completed.
func GetDone(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyDone)
	return v
}

// SetDone marks the euclo execution cycle as complete.
func SetDone(env *contextdata.Envelope, done bool) {
	contextdata.SetTyped(env, KeyDone, done)
}

// GetCapabilityClassified reports whether capability classification has been performed.
func GetCapabilityClassified(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyCapabilityClassified)
	return v
}

// SetCapabilityClassified records whether capability classification was performed.
func SetCapabilityClassified(env *contextdata.Envelope, classified bool) {
	contextdata.SetTyped(env, KeyCapabilityClassified, classified)
}

// GetForkBranch returns the active fork branch identifier.
func GetForkBranch(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyForkBranch)
	return v
}

// SetForkBranch stores the active fork branch identifier.
func SetForkBranch(env *contextdata.Envelope, branch string) {
	contextdata.SetTyped(env, KeyForkBranch, branch)
}

// GetExecutionMerged reports whether the merge node has run.
func GetExecutionMerged(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyExecutionMerged)
	return v
}

// SetExecutionMerged marks that the parallel merge node has completed.
func SetExecutionMerged(env *contextdata.Envelope, merged bool) {
	contextdata.SetTyped(env, KeyExecutionMerged, merged)
}

// GetFamilySelected returns the resolved family name (derived from the family_selection map).
func GetFamilySelected(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyFamilySelected)
	return v
}

// SetFamilySelected stores the resolved family name.
func SetFamilySelected(env *contextdata.Envelope, family string) {
	contextdata.SetTyped(env, KeyFamilySelected, family)
}

// GetStreamRequested reports whether a stream result was present at intake.
func GetStreamRequested(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyStreamRequested)
	return v
}

// SetStreamRequested records whether a stream was detected at intake.
func SetStreamRequested(env *contextdata.Envelope, requested bool) {
	contextdata.SetTyped(env, KeyStreamRequested, requested)
}
