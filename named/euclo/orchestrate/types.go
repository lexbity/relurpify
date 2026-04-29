package orchestrate

// RouteSelection holds the resolved execution route.
// To be fully implemented in Phase 12.
type RouteSelection struct {
	RouteKind string // "recipe" or "capability"
	RecipeID  string
	CapabilityID string
}
