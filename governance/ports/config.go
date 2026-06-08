package ports

// ManifestView is the governance-owned view of an agent manifest.
// It provides only the fields governance needs for policy evaluation.
type ManifestView struct {
	ID                  string
	AllowedCapabilities []CapabilitySelectorView
	GlobalPolicies      map[string]string
}

// ManifestSnapshotView is the governance-owned view of a resolved manifest snapshot.
type ManifestSnapshotView interface {
	Manifest() ManifestView
	AgentSpec() SpecView
}

// SandboxPolicyView is the governance-owned view of a sandbox policy.
type SandboxPolicyView struct {
	ProtectedPaths []string
}

// SecurityBundleView is the governance-owned view of a security bundle.
type SecurityBundleView struct {
	Sandbox SandboxPolicyView
}

// AgentContractView is the governance-owned view of an effective agent contract.
type AgentContractView struct {
	AgentID   string
	AgentSpec SpecView
	Rules     []PolicyRuleView
}

// PolicyRuleView is the governance-owned view of a policy rule.
type PolicyRuleView struct {
	ID       string
	Priority int
	Effect   string
}
