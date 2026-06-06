package policy

// RiskLevel models the qualitative risk assessment used by the human-in-the-loop
// approval flow. It is owned here (not in authorization or capability) so the
// permission-manager port the capability domain depends on can name the same
// type the authorization implementation produces, without either domain
// importing the other.
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
)

// GrantScope defines the lifecycle of an approval grant.
type GrantScope string

const (
	GrantScopeOneTime     GrantScope = "one_time"
	GrantScopeSession     GrantScope = "session"
	GrantScopePersistent  GrantScope = "persistent"
	GrantScopeConditional GrantScope = "conditional"
	GrantScopeTask        GrantScope = "task"
)
