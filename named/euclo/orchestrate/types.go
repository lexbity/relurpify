package orchestrate

// RouteSelection holds the resolved execution route.
// To be fully implemented in Phase 12.
type RouteSelection struct {
	RouteKind    string // "recipe" or "capability"
	RecipeID     string
	CapabilityID string
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
	FamilyID     string
	RecipeID     string
	CapabilityID string
	Instruction  string
	Inputs       map[string]any
	FallbackID   string
	DryRun       bool
	SkillFilter  string
	TelemetryOff bool
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
