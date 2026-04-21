package classify

import (
	"strings"

	"codeburg.org/lexbit/relurpify/named/rex/envelope"
)

// Classification captures task shape and risk used by rex routing.
type Classification struct {
	ReadOnly               bool
	MutationCapable        bool
	DeterministicPreferred bool
	RecoveryHeavy          bool
	LongRunningManaged     bool
	Intent                 string
	RiskLevel              string
	ReasonCodes            []string
}

// Classify derives a route-friendly classification from an Envelope.
func Classify(env envelope.Envelope) Classification {
	text := strings.ToLower(strings.TrimSpace(env.Instruction))
	result := Classification{
		ReadOnly:        !env.EditPermitted,
		MutationCapable: env.EditPermitted,
		Intent:          "analysis",
		RiskLevel:       "medium",
	}
	if containsAny(text, "plan", "architecture", "design") {
		result.Intent = "planning"
	}
	if containsAny(text, "review", "findings", "audit") {
		result.Intent = "review"
		result.ReadOnly = true
		result.MutationCapable = false
	}
	if containsAny(text, "pipeline", "stage", "schema", "structured") {
		result.DeterministicPreferred = true
		result.ReasonCodes = append(result.ReasonCodes, "deterministic_stages")
	}
	if containsAny(text, "resume", "recover", "checkpoint", "retry") || env.ResumedRoute != "" || env.WorkflowID != "" {
		result.RecoveryHeavy = true
		result.ReasonCodes = append(result.ReasonCodes, "recovery")
	}
	if containsAny(text, "monitor", "watch", "background", "continuous", "loop") {
		result.LongRunningManaged = true
		result.ReasonCodes = append(result.ReasonCodes, "managed")
	}
	if containsAny(text, "edit", "implement", "patch", "fix", "refactor", "write") && env.EditPermitted {
		result.Intent = "mutation"
		result.ReadOnly = false
		result.MutationCapable = true
	}
	if result.ReadOnly {
		result.RiskLevel = "low"
	}
	if result.LongRunningManaged || result.MutationCapable {
		result.RiskLevel = "high"
	}
	return result
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
