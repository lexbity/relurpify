package classification

import "strings"

// NormalizeClassString converts snake_case class strings to the canonical
// kebab-case vocabulary used by EffectClass, TrustClass, and RiskClass.
//
// Manifests and config files legitimately declare classes in snake_case
// (e.g. "process_spawn", "filesystem_mutation"), while the canonical constants
// in this package, capability/agentspec, and governance/risk are kebab-case
// ("process-spawn", "filesystem-mutation"). This function is applied at the
// manifest/config load boundary so that every downstream comparison — policy
// matching, risk classification, write detection — operates on canonical
// constants and never silently fails to match.
//
// The function is pure and total (it never panics on any input) and idempotent:
//
//	NormalizeClassString(NormalizeClassString(s)) == NormalizeClassString(s)
func NormalizeClassString(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
}
