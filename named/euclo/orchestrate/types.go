package orchestrate

import "codeburg.org/lexbit/relurpify/named/euclo/euclotypes"

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
	// DecidedBy names the D8 lattice rule (or explicit/default) that produced
	// the selection, e.g. "lattice:family_affinity" or "explicit".
	DecidedBy string
	// Tier2 records the bounded Tier-2 disambiguation attempt when the gate was
	// consulted (D10).
	Tier2 euclotypes.Tier2Info
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
	// DecidedBy names the D8 lattice rule that produced the selection.
	DecidedBy string
	// Tier2 records the bounded Tier-2 disambiguation attempt when the gate was
	// consulted (D10).
	Tier2 euclotypes.Tier2Info
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
	// Components is the per-component score split (§3.6.1). Keys are the
	// selection_config component names; the values cap at the per-component
	// ceiling. RankScore is the sum of the components the candidate earned.
	Components map[string]int
	// Description is the one-line candidate description used by the Tier-2
	// prompt (D10) and the selection record.
	Description string
	// MatchedKeywords are the candidate-side vocabulary tokens that matched the
	// utterance (public registry data — never raw user text). They feed the
	// Tier-2 prompt and, from Phase 7, the selection record.
	MatchedKeywords []string
}

// RouteResolutionError indicates that no route could be selected.
type RouteResolutionError struct {
	PrimaryID string
	Reason    string
	// MissingRecipeID is set when the gate rejected the only candidate
	// because the thoughtrecipe is not registered: the named-missing recipe
	// is the actionable signal (re-init / doctor).
	MissingRecipeID string
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
