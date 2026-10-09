package orchestrate

import "strings"

// Selection scoring and gating constants (Wave 2 §3.6.1). Defaults live in
// this one file as code constants; new YAML/config surfaces are rejected.
//
// The component names are stable keys recorded in a candidate's Components
// map and, from Phase 7, in the Selection Decision Record's per-candidate
// component split.
const (
	// component names (stable record keys)
	compExplicit       = "explicit"
	compHandoff        = "handoff"
	compKeyword        = "keyword"
	compFamilyAffinity = "family_affinity"
	compIntentKeyword  = "intent_keyword"
	compDescription    = "description"
	compPriority       = "priority" // capability euclo.priority annotation; shipped data, not a config surface

	// points per component
	scoreExplicit         = 1000
	scoreHandoff          = 100
	scorePerKeyword       = 10
	scoreFamilyAffinity   = 50
	scorePerIntentKeyword = 5
	scorePerDescription   = 1

	// caps per component
	capHandoff        = 100
	capKeyword        = 30
	capFamilyAffinity = 50
	capIntentKeyword  = 15
	capDescription    = 10
)

// The Tier-2 disambiguation gate (D10). The deterministic winner is consulted
// only inside the weak/tie band below; the model sees only the top-K candidate
// refs, and its answer is adopted only above the confidence floor.
const (
	// strongMatchFloor is the deterministic score at or above which the
	// Tier-2 model is never consulted.
	strongMatchFloor = 60
	// tieBand is the top1−top2 gap at or below which the Tier-2 model may be
	// asked to disambiguate.
	tieBand = 10
	// tier2TopK bounds the candidate refs the Tier-2 prompt may carry.
	tier2TopK = 5
	// tier2ConfidenceFloor is the model confidence below which its answer is
	// rejected and the deterministic winner stands.
	tier2ConfidenceFloor = 0.5
)

// decidedBy reason vocabulary recorded on RouteResult, DryRunReport, the
// euclo.route.decided_by envelope key, and (Phase 7) the selection record.
const (
	decidedByExplicit        = "explicit"
	decidedByDefaultRecipe   = "default_recipe"
	decidedByScore           = "lattice:score"
	decidedByFamilyAffinity  = "lattice:family_affinity"
	decidedByUserOverBuiltin = "lattice:user_over_builtin"
	decidedByRecipeOverCap   = "lattice:thoughtrecipe_over_capability"
	decidedByRouteID         = "lattice:route_id"
	decidedByTier2           = "tier2"
)

// builtinRecipePrefix is the ID prefix that marks a builtin thoughtrecipe
// (D8 rank 4: at equal evidence, user recipes shadow builtins). The built-in
// default and clarification recipes both carry it.
const builtinRecipePrefix = "euclo.thoughtrecipe."

// isBuiltinRoute reports whether a route ID belongs to the builtin
// thoughtrecipe namespace.
func isBuiltinRoute(id RouteID) bool {
	return strings.HasPrefix(string(id), builtinRecipePrefix)
}

// boundedComponent caps a per-component score at its ceiling.
func boundedComponent(matches, per, ceiling int) int {
	if matches <= 0 {
		return 0
	}
	score := matches * per
	if score > ceiling {
		return ceiling
	}
	return score
}
