package thoughtrecipes

// StandardAliases maps well-known recipe alias names to their underlying
// state keys. Recipe authors use these names in inherit/capture declarations.
var StandardAliases = map[string]string{
	"explore_findings":      "pipeline.explore",
	"analysis_result":       "pipeline.analyze",
	"plan_output":           "pipeline.plan",
	"code_changes":          "pipeline.code",
	"verify_result":         "pipeline.verify",
	"final_output":          "pipeline.final_output",
	"review_findings":       "euclo.review_findings",
	"root_cause":            "euclo.root_cause",
	"root_cause_candidates": "euclo.root_cause_candidates",
	"regression_analysis":   "euclo.regression_analysis",
	"verification_summary":  "euclo.verification_summary",
	"debug_investigation":   "euclo.debug_investigation_summary",
	"repair_readiness":      "euclo.debug_repair_readiness",
	"plan_candidates":       "euclo.plan_candidates",
}

// AliasResolver resolves alias names to state keys, combining standard aliases
// and recipe-local custom aliases.
type AliasResolver struct {
	custom map[string]string // recipe-local aliases from global.context.aliases
}

// NewAliasResolver constructs a resolver combining standard and custom aliases.
// Custom aliases that shadow standard aliases emit a warning.
func NewAliasResolver(custom map[string]string) *AliasResolver {
	return &AliasResolver{
		custom: custom,
	}
}

// Resolve returns the underlying state key for alias, or ("", false) if unknown.
func (r *AliasResolver) Resolve(alias string) (stateKey string, ok bool) {
	if r == nil {
		return StandardAliases[alias], alias != "" && StandardAliases[alias] != ""
	}

	// Check custom aliases first (recipe-local shadows standard)
	if r.custom != nil {
		if key, exists := r.custom[alias]; exists {
			return key, true
		}
	}

	// Fall back to standard aliases
	key, exists := StandardAliases[alias]
	return key, exists
}

// MustResolve returns the state key or the alias itself as a fallback.
func (r *AliasResolver) MustResolve(alias string) string {
	key, ok := r.Resolve(alias)
	if !ok {
		return alias
	}
	return key
}

// IsShadowed returns true if the given custom alias name shadows a standard alias.
// This can be used to emit warnings during recipe loading.
func (r *AliasResolver) IsShadowed(alias string) bool {
	if r == nil || r.custom == nil {
		return false
	}
	_, isCustom := r.custom[alias]
	_, isStandard := StandardAliases[alias]
	return isCustom && isStandard
}

// ListShadowed returns all custom alias names that shadow standard aliases.
func (r *AliasResolver) ListShadowed() []string {
	if r == nil || r.custom == nil {
		return nil
	}
	var shadowed []string
	for alias := range r.custom {
		if _, isStandard := StandardAliases[alias]; isStandard {
			shadowed = append(shadowed, alias)
		}
	}
	return shadowed
}
