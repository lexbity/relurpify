package orchestrate

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

// RouteID is the canonical route identifier type used by route reporting.
type RouteID string

// RouteAvailability mirrors the route catalog availability states.
type RouteAvailability string

const (
	RouteAvailable                    RouteAvailability = "available"
	RouteUnavailableDependencyMissing RouteAvailability = "unavailable:dependency_missing"
	RouteUnavailableToolNotEnabled    RouteAvailability = "unavailable:tool_not_enabled"
	RouteUnavailablePolicyDenied      RouteAvailability = "unavailable:policy_denied"
	RouteUnavailableUnsupported       RouteAvailability = "unavailable:unsupported"
)

// RouteRequest is the external input to Euclo route dispatch.
type RouteRequest struct {
	FamilyID        string
	ThoughtRecipeID string
	CapabilityID    string
	Instruction     string
	Inputs          map[string]any
	FallbackID      string
	DryRun          bool
	SkillFilter     string
	TelemetryOff    bool
}

// RouteResult is the runtime outcome of a route dispatch.
type RouteResult struct {
	RouteKind           string
	RouteID             string
	SkillFilterName     string
	CandidateCount      int
	FallbackTaken       bool
	FallbackID          string
	ApprovalRequired    bool
	ArtifactKinds       []string
	Outcome             string
	TelemetrySuppressed bool
}

// DryRunReport captures the selected route plus the candidate set considered.
type DryRunReport struct {
	Request               RouteRequest
	SelectedRoute         RouteID
	SelectedKind          string
	SkillFilterName       string
	Candidates            []CandidateRouteInfo
	PolicyBlockers        []string
	HITLRequired          bool
	ExpectedArtifactKinds []string
	FallbackPath          *RouteID
	ExecutionClass        string
	PreflightErrors       []string
}

// CandidateRouteInfo describes one candidate route in the ranking set.
type CandidateRouteInfo struct {
	RouteID        RouteID
	RouteKind      string
	Availability   RouteAvailability
	RankScore      int
	RankReasons    []string
	Suppressed     bool
	SuppressReason string
}

// RouteResolutionError indicates that no route could be selected.
type RouteResolutionError struct {
	PrimaryID string
	Reason    string
}

func (e *RouteResolutionError) Error() string {
	if e == nil {
		return "route resolution failed"
	}
	if e.PrimaryID == "" {
		return e.Reason
	}
	if e.Reason == "" {
		return "route resolution failed"
	}
	return e.PrimaryID + ": " + e.Reason
}
