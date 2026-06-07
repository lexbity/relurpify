package state

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// GetRouteContinuation retrieves the active route continuation record.
func GetRouteContinuation(env *contextdata.Envelope) (*euclotypes.RouteContinuation, bool) {
	return contextdata.GetTyped[*euclotypes.RouteContinuation](env, KeyRouteContinuation)
}

// SetRouteContinuation stores the active route continuation record.
func SetRouteContinuation(env *contextdata.Envelope, c *euclotypes.RouteContinuation) {
	contextdata.SetTyped(env, KeyRouteContinuation, c)
}

// GetRouteCandidateCount returns the number of candidates considered during dispatch.
func GetRouteCandidateCount(env *contextdata.Envelope) (int, bool) {
	return contextdata.GetTyped[int](env, KeyRouteCandidateCount)
}

// SetRouteCandidateCount stores the candidate count from the most recent dispatch.
func SetRouteCandidateCount(env *contextdata.Envelope, n int) {
	contextdata.SetTyped(env, KeyRouteCandidateCount, n)
}

// GetRouteFallbackTaken reports whether the dispatcher used a fallback route.
func GetRouteFallbackTaken(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyRouteFallbackTaken)
	return v
}

// SetRouteFallbackTaken records whether the dispatcher used a fallback route.
func SetRouteFallbackTaken(env *contextdata.Envelope, taken bool) {
	contextdata.SetTyped(env, KeyRouteFallbackTaken, taken)
}

// GetRouteFallbackID returns the ID of the fallback route that was taken, if any.
func GetRouteFallbackID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyRouteFallbackID)
	return v
}

// SetRouteFallbackID records the fallback route ID.
func SetRouteFallbackID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyRouteFallbackID, id)
}

// GetRouteSkillFilter returns the skill filter name applied during dispatch.
func GetRouteSkillFilter(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyRouteSkillFilter)
	return v
}

// SetRouteSkillFilter stores the skill filter name from the dispatch result.
func SetRouteSkillFilter(env *contextdata.Envelope, name string) {
	contextdata.SetTyped(env, KeyRouteSkillFilter, name)
}

// GetRouteOutcome returns the route dispatch outcome string.
func GetRouteOutcome(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyRouteOutcome)
	return v
}

// SetRouteOutcome stores the dispatch outcome.
func SetRouteOutcome(env *contextdata.Envelope, outcome string) {
	contextdata.SetTyped(env, KeyRouteOutcome, outcome)
}

// GetRouteTelemetryOff reports whether route telemetry has been suppressed by the caller.
func GetRouteTelemetryOff(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyRouteTelemetryOff)
	return v
}

// SetRouteTelemetryOff sets the caller's telemetry suppression flag.
func SetRouteTelemetryOff(env *contextdata.Envelope, off bool) {
	contextdata.SetTyped(env, KeyRouteTelemetryOff, off)
}

// GetSkillFilter returns the caller-provided skill filter hint.
func GetSkillFilter(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeySkillFilter)
	return v
}

// SetSkillFilter stores the caller-provided skill filter hint.
func SetSkillFilter(env *contextdata.Envelope, filter string) {
	contextdata.SetTyped(env, KeySkillFilter, filter)
}
