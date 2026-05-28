package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// GetTaskEnvelopeEditPermitted reports whether task envelope edits are allowed.
// Defaults to true if the key has not been explicitly set.
func GetTaskEnvelopeEditPermitted(env *contextdata.Envelope) bool {
	v, ok := contextdata.GetTyped[bool](env, KeyTaskEnvelopeEditPermitted)
	if !ok {
		return true
	}
	return v
}

// SetTaskEnvelopeEditPermitted stores whether task envelope edits are permitted.
func SetTaskEnvelopeEditPermitted(env *contextdata.Envelope, permitted bool) {
	contextdata.SetTyped(env, KeyTaskEnvelopeEditPermitted, permitted)
}

// GetPolicyRiskLevel returns the policy risk level string ("low", "medium", "high").
// Defaults to "low" if not set.
func GetPolicyRiskLevel(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyPolicyRiskLevel)
	if v == "" {
		return "low"
	}
	return v
}

// SetPolicyRiskLevel stores the policy risk level.
func SetPolicyRiskLevel(env *contextdata.Envelope, level string) {
	contextdata.SetTyped(env, KeyPolicyRiskLevel, level)
}

// GetPolicyVerificationRequired reports whether the policy gate requires human verification.
func GetPolicyVerificationRequired(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyPolicyVerificationRequired)
	return v
}

// SetPolicyVerificationRequired stores whether human verification is required.
func SetPolicyVerificationRequired(env *contextdata.Envelope, required bool) {
	contextdata.SetTyped(env, KeyPolicyVerificationRequired, required)
}

// SeedPolicyDefaults writes the policy default values only if each key is absent.
// This mirrors the idiomatic "if not set, seed" pattern used in graph.go.
func SeedPolicyDefaults(env *contextdata.Envelope) {
	if _, ok := contextdata.GetTyped[bool](env, KeyTaskEnvelopeEditPermitted); !ok {
		SetTaskEnvelopeEditPermitted(env, true)
	}
	if _, ok := contextdata.GetTyped[string](env, KeyPolicyRiskLevel); !ok {
		SetPolicyRiskLevel(env, "low")
	}
	if _, ok := contextdata.GetTyped[bool](env, KeyPolicyVerificationRequired); !ok {
		SetPolicyVerificationRequired(env, false)
	}
}
