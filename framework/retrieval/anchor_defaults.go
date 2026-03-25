package retrieval

// DefaultPolicyTerms are terms that carry authorization, permission, or governance meaning.
var DefaultPolicyTerms = []string{
	"verified", "approved", "denied", "trusted", "untrusted",
	"restricted", "allowed", "forbidden", "revoked", "suspended",
	"validated", "authorized", "unauthorized", "granted", "rejected",
}

// DefaultIdentityTerms are terms that name entities, roles, or ownership.
var DefaultIdentityTerms = []string{
	"owner", "delegator", "agent", "provider", "tenant",
	"actor", "principal", "session", "user", "admin",
	"maintainer", "reviewer", "author",
}

// DefaultCommitmentTerms are terms that represent promises, contracts, or obligations.
var DefaultCommitmentTerms = []string{
	"guaranteed", "required", "must", "shall", "will",
	"promised", "committed", "obligated", "ensure", "maintain",
	"support", "provide", "deliver", "should",
}

// DefaultTechnicalTerms are terms with domain-specific meaning that differs from common usage.
var DefaultTechnicalTerms = []string{
	"deprecated", "legacy", "experimental", "stable", "production",
	"alpha", "beta", "rc", "release", "snapshot",
	"snapshot", "nightly", "canary",
}

// DefaultAnchorTerms is a map from anchor class to default terms for that class.
var DefaultAnchorTerms = map[string][]string{
	"policy":      DefaultPolicyTerms,
	"identity":    DefaultIdentityTerms,
	"commitment":  DefaultCommitmentTerms,
	"technical":   DefaultTechnicalTerms,
}

// AnchorDetectionConfig controls whether and how anchors are auto-detected during ingestion.
type AnchorDetectionConfig struct {
	Enabled         bool
	CorpusScope     string
	Classes         map[string]bool // which anchor classes to auto-detect
	CustomTerms     map[string][]string // custom term sets per class
}

// DefaultAnchorDetectionConfig returns a config with auto-detection disabled by default.
func DefaultAnchorDetectionConfig(corpusScope string) *AnchorDetectionConfig {
	return &AnchorDetectionConfig{
		Enabled:     false,
		CorpusScope: corpusScope,
		Classes: map[string]bool{
			"policy":     false,
			"identity":   false,
			"commitment": false,
			"technical":  false,
		},
		CustomTerms: make(map[string][]string),
	}
}

// GetTermsForClass returns the terms to search for a given anchor class.
// Prefers custom terms if defined, falls back to default terms.
func (c *AnchorDetectionConfig) GetTermsForClass(class string) []string {
	if custom, ok := c.CustomTerms[class]; ok && len(custom) > 0 {
		return custom
	}
	return DefaultAnchorTerms[class]
}

// EnableClass enables auto-detection for a specific anchor class.
func (c *AnchorDetectionConfig) EnableClass(class string) {
	if c.Classes != nil {
		c.Classes[class] = true
	}
}

// DisableClass disables auto-detection for a specific anchor class.
func (c *AnchorDetectionConfig) DisableClass(class string) {
	if c.Classes != nil {
		c.Classes[class] = false
	}
}

// IsClassEnabled returns true if a class is enabled for auto-detection.
func (c *AnchorDetectionConfig) IsClassEnabled(class string) bool {
	if !c.Enabled {
		return false
	}
	if c.Classes == nil {
		return false
	}
	enabled, ok := c.Classes[class]
	return ok && enabled
}
