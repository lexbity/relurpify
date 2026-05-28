package state

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
)

// Envelope-based working-memory accessors for the rex agent.
//
// These provide compile-time type safety over raw SetWorkingValue/GetWorkingValue
// calls. Every function in this file corresponds to a key defined in
// named/rex/rexkeys/ and catalogued in devdocs/ref/envelope-key-registry.md.

// EnvelopeWorkflowID returns the rex workflow ID from the envelope's working memory.
func EnvelopeWorkflowID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexWorkflowID)
	return strings.TrimSpace(v)
}

// SetEnvelopeWorkflowID stores the rex workflow ID in the envelope's working memory.
func SetEnvelopeWorkflowID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, rexkeys.RexWorkflowID, strings.TrimSpace(id))
}

// EnvelopeRunID returns the rex run ID from the envelope's working memory.
func EnvelopeRunID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexRunID)
	return strings.TrimSpace(v)
}

// SetEnvelopeRunID stores the rex run ID in the envelope's working memory.
func SetEnvelopeRunID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, rexkeys.RexRunID, strings.TrimSpace(id))
}

// ResumedRoute returns the route ID from a prior session that rex should resume.
// Returns "" if absent.
func ResumedRoute(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexRoute)
	return strings.TrimSpace(v)
}

// SetResumedRoute stores the route ID to resume in the next execution.
func SetResumedRoute(env *contextdata.Envelope, routeID string) {
	contextdata.SetTyped(env, rexkeys.RexRoute, strings.TrimSpace(routeID))
}

// ArtifactKinds returns the list of artifact kind strings that will be persisted
// for this rex execution.
func ArtifactKinds(env *contextdata.Envelope) []string {
	v, _ := contextdata.GetTyped[[]string](env, rexkeys.RexArtifactKinds)
	return v
}

// SetArtifactKinds stores the list of artifact kind strings to persist.
func SetArtifactKinds(env *contextdata.Envelope, kinds []string) {
	contextdata.SetTyped(env, rexkeys.RexArtifactKinds, kinds)
}

// FMPLineageID returns the FMP lineage ID from the envelope, if present.
func FMPLineageID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexFMPLineageID)
	return v
}

// FMPAttemptID returns the FMP attempt ID from the envelope, if present.
func FMPAttemptID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexFMPAttemptID)
	return v
}

// EventType returns the server-assigned rex event type.
// TRUSTED — must never be written from caller-controlled payloads.
func EventType(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexEventType)
	return v
}

// EventID returns the server-assigned rex event ID.
// TRUSTED — must never be written from caller-controlled payloads.
func EventID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexEventID)
	return v
}

// EventPartition returns the server-assigned rex event partition.
func EventPartition(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexEventPartition)
	return v
}

// EventIngressOrigin returns the server-assigned ingress origin label.
func EventIngressOrigin(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexEventIngressOrigin)
	return v
}

// AdmissionTenantID returns the server-assigned admission tenant ID.
// TRUSTED — must never be written from caller-controlled payloads.
func AdmissionTenantID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexAdmissionTenantID)
	return v
}

// WorkloadClass returns the server-assigned workload class.
// TRUSTED — must never be written from caller-controlled payloads.
func WorkloadClass(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.RexWorkloadClass)
	return v
}

// GatewaySessionID returns the gateway session ID from the envelope.
func GatewaySessionID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.GatewaySessionID)
	return v
}

// GatewayTenantID returns the gateway tenant ID from the envelope.
func GatewayTenantID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, rexkeys.GatewayTenantID)
	return v
}
