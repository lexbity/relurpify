package policy

// PolicyDecision represents the outcome of policy evaluation.
type PolicyDecision struct {
	MutationPermitted bool
	HITLRequired      bool
	VerificationRequired bool
	ReasonCodes       []string
}

// PolicyContext provides context for policy evaluation.
type PolicyContext struct {
	FamilyID          string
	EditPermitted     bool
	RequiresVerification bool
	RiskLevel         string
	WorkspaceScopes   []string
}
