package ports

// CoordinationSpecView is a governance-owned view of an agent's coordination
// spec, computed by EffectiveCoordination on the capability side.
type CoordinationSpecView struct {
	MaxDelegationDepth        int
	AllowRemoteDelegation     bool
	AllowBackgroundDelegation bool
	RequireApprovalCrossTrust bool
	DelegationTargetSelectors []CapabilitySelectorView
}
