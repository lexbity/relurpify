// Package risk defines RiskClass — a governance judgment about how dangerous
// a capability is — and provides Classify, the sole producer of risk values.
//
// Per Q1: RiskClass is owned by governance (it is a derived judgment over
// facts, not a self-declared fact). Classify converts (EffectClass set ×
// CapabilityScope) into a risk classification, applying a per-scope minimum
// risk floor.
//
// Trust boundary (critical): EffectClass/CapabilityScope are claims for
// workspace/provider/remote scopes and ground truth only for builtin.
// Classify MUST apply a scope floor: any non-builtin scope raises the
// minimum risk regardless of declared effects (e.g. a remote tool declaring
// only read-only is still treated as at least sessioned/untrusted).
package risk

import "codeburg.org/lexbit/relurpify/governance/classification"

// RiskClass classifies the risk of a capability.
type RiskClass string

const (
	RiskClassReadOnly     RiskClass = "read-only"
	RiskClassDestructive  RiskClass = "destructive"
	RiskClassExecute      RiskClass = "execute"
	RiskClassNetwork      RiskClass = "network"
	RiskClassCredentialed RiskClass = "credentialed"
	RiskClassExfiltration RiskClass = "exfiltration-sensitive"
	RiskClassSessioned    RiskClass = "sessioned"
)

// Classify computes risk classifications from a set of declared effects and
// an operational scope. It is the sole producer of risk values in the system.
//
// The scope floor guarantees non-builtin scopes always receive at minimum
// the risk corresponding to their untrusted nature. A remote tool claiming
// only read-only effects is still at least sessioned.
func Classify(effects []classification.EffectClass, scope classification.CapabilityScope) []RiskClass {
	set := make(map[RiskClass]struct{})

	for _, eff := range effects {
		switch eff {
		case classification.EffectClassFilesystemMutation:
			set[RiskClassDestructive] = struct{}{}
		case classification.EffectClassProcessSpawn:
			set[RiskClassExecute] = struct{}{}
		case classification.EffectClassNetworkEgress:
			set[RiskClassNetwork] = struct{}{}
			set[RiskClassExfiltration] = struct{}{}
		case classification.EffectClassCredentialUse:
			set[RiskClassCredentialed] = struct{}{}
		case classification.EffectClassExternalState:
			set[RiskClassNetwork] = struct{}{}
			set[RiskClassExfiltration] = struct{}{}
		case classification.EffectClassSessionCreation:
			set[RiskClassSessioned] = struct{}{}
		}
	}

	if len(set) == 0 {
		set[RiskClassReadOnly] = struct{}{}
	}

	// Scope floor: non-builtin scopes always receive minimum risk classes
	// regardless of declared effects. These are additive — they guarantee
	// the floor risk is present even when effects produce higher-ranked risks.
	switch scope {
	case classification.CapabilityScopeWorkspace:
		set[RiskClassSessioned] = struct{}{}
	case classification.CapabilityScopeProvider:
		set[RiskClassCredentialed] = struct{}{}
	case classification.CapabilityScopeRemote:
		set[RiskClassSessioned] = struct{}{}
		set[RiskClassNetwork] = struct{}{}
		set[RiskClassExfiltration] = struct{}{}
	}

	return sortRiskClasses(set)
}

// sortOrder ranks risk classes for deterministic output ordering.
var sortOrder = []RiskClass{
	RiskClassReadOnly,
	RiskClassDestructive,
	RiskClassExecute,
	RiskClassNetwork,
	RiskClassCredentialed,
	RiskClassExfiltration,
	RiskClassSessioned,
}

func sortRiskClasses(set map[RiskClass]struct{}) []RiskClass {
	if len(set) == 0 {
		return nil
	}
	out := make([]RiskClass, 0, len(set))
	for _, r := range sortOrder {
		if _, ok := set[r]; ok {
			out = append(out, r)
		}
	}
	return out
}
