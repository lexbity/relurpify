// Package euclotypes holds the core route types shared between the orchestrate
// and state packages. Keeping them here prevents import cycles: both packages
// can import euclotypes without either importing the other.
package euclotypes

import "strings"

// Canonical route kinds used across dispatcher, fork, and execution paths.
const (
	RouteKindThoughtRecipe = "thoughtrecipe"
	RouteKindCapability    = "capability"
	RouteKindIntent        = "intent"
)

// RouteSelection holds the resolved execution route.
type RouteSelection struct {
	RouteKind       string // thoughtrecipe, capability, or intent
	ThoughtRecipeID string
	CapabilityID    string
}

// RouteResolution records how a route was selected.
type RouteResolution struct {
	RouteKind                 string
	ThoughtRecipeID           string
	CapabilityID              string
	ResolutionSource          string
	FallbackTaken             bool
	ClarificationStateVersion uint64
	ReasonCodes               []string
	// DecidedBy names the D8 lattice rule (or explicit/default) that produced
	// the selection; it is the durable seed of the Selection Decision Record.
	DecidedBy string
	// Tier2 records the bounded Tier-2 disambiguation attempt, if the gate was
	// consulted (D10).
	Tier2 Tier2Info
}

// Tier2Info records the bounded Tier-2 disambiguation attempt for one
// selection. Used=false means the gate was not consulted (strong/explicit
// match). Outcome is one of applied|rejected|unparseable|low_confidence|
// unavailable.
type Tier2Info struct {
	Used        bool    `json:"used"`
	Outcome     string  `json:"outcome,omitempty"`
	Model       string  `json:"model,omitempty"`
	CandidateID string  `json:"candidate_id,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	LatencyMs   int64   `json:"latency_ms,omitempty"`
}

// Normalize trims route-resolution fields and preserves stable reason ordering.
func (r *RouteResolution) Normalize() {
	if r == nil {
		return
	}
	r.RouteKind = strings.TrimSpace(r.RouteKind)
	r.ThoughtRecipeID = strings.TrimSpace(r.ThoughtRecipeID)
	r.CapabilityID = strings.TrimSpace(r.CapabilityID)
	r.ResolutionSource = strings.TrimSpace(r.ResolutionSource)
	r.DecidedBy = strings.TrimSpace(r.DecidedBy)
	r.Tier2.Outcome = strings.TrimSpace(r.Tier2.Outcome)
	r.Tier2.Model = strings.TrimSpace(r.Tier2.Model)
	r.Tier2.CandidateID = strings.TrimSpace(r.Tier2.CandidateID)
	if len(r.ReasonCodes) == 0 {
		r.ReasonCodes = nil
		return
	}
	out := make([]string, 0, len(r.ReasonCodes))
	for _, reason := range r.ReasonCodes {
		if trimmed := strings.TrimSpace(reason); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		r.ReasonCodes = nil
		return
	}
	r.ReasonCodes = out
}

// RouteID returns the selected route identifier.
func (r *RouteResolution) RouteID() string {
	if r == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(r.ThoughtRecipeID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(r.CapabilityID)
}

// RouteContinuation records how the selected route continues the shared runtime context.
type RouteContinuation struct {
	SharedContext         bool
	SourceRouteKind       string
	SourceRouteID         string
	TargetRouteKind       string
	TargetRouteID         string
	ActiveThoughtRecipeID string
}

// IsIntentRouteKind reports whether a route kind represents an intent thoughtrecipe.
func IsIntentRouteKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), RouteKindIntent)
}

// IsThoughtRecipeRouteKind reports whether a route kind represents thoughtrecipe execution.
func IsThoughtRecipeRouteKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), RouteKindThoughtRecipe)
}

// IsCapabilityRouteKind reports whether a route kind represents capability execution.
func IsCapabilityRouteKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), RouteKindCapability)
}

// RouteKindForThoughtRecipeID derives the canonical route kind for a thoughtrecipe ID.
// Intent thoughtrecipes are identified by the Euclo intent thoughtrecipe namespace.
func RouteKindForThoughtRecipeID(thoughtrecipeID string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(thoughtrecipeID)), "euclo.thoughtrecipe.intent.") {
		return RouteKindIntent
	}
	return RouteKindThoughtRecipe
}
