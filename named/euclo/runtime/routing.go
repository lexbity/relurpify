package runtime

import (
	"sort"
)

// CapabilityFamilyDescriptor describes a logical capability family.
// Retained for observability metadata — the registry is no longer needed
// since profile controller + coding capability registry handle dispatch.
type CapabilityFamilyDescriptor struct {
	FamilyID          string
	PreferredParadigm string
	FallbackParadigms []string
	SupportedModes    []string
	ExpectedArtifacts []string
}

// defaultParadigmForFamily maps family IDs to their preferred paradigm.
// This replaces the registry lookup that RouteCapabilityFamilies previously used.
var defaultParadigmForFamily = map[string]string{
	"implementation": "react",
	"debugging":      "react",
	"planning":       "planner",
	"review":         "reflection",
	"verification":   "react",
}

// RouteCapabilityFamilies produces an observability artifact describing
// the capability family routing for the given mode and profile. It no longer
// requires a CapabilityFamilyRegistry — paradigm mapping is inline.
func RouteCapabilityFamilies(mode ModeResolution, profile ExecutionProfileSelection) CapabilityFamilyRouting {
	routing := CapabilityFamilyRouting{
		ModeID:    mode.ModeID,
		ProfileID: profile.ProfileID,
	}
	switch mode.ModeID {
	case "planning":
		routing.PrimaryFamilyID = "planning"
		routing.FallbackFamilyIDs = []string{"implementation"}
	case "review":
		routing.PrimaryFamilyID = "review"
		routing.FallbackFamilyIDs = []string{"planning"}
	case "debug":
		routing.PrimaryFamilyID = "debugging"
		routing.FallbackFamilyIDs = []string{"implementation", "verification"}
	default:
		routing.PrimaryFamilyID = "implementation"
		routing.FallbackFamilyIDs = []string{"verification", "planning"}
	}
	routes := make([]PhaseCapabilityRoute, 0, len(profile.PhaseRoutes))
	for phase, agent := range profile.PhaseRoutes {
		family := familyForPhase(phase, routing.PrimaryFamilyID)
		if paradigm, ok := defaultParadigmForFamily[family]; ok {
			agent = paradigm
		}
		routes = append(routes, PhaseCapabilityRoute{Phase: phase, Family: family, Agent: agent})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Phase < routes[j].Phase })
	routing.Routes = routes
	return routing
}

func familyForPhase(phase, primary string) string {
	switch phase {
	case "plan", "plan_tests":
		return "planning"
	case "review", "summarize":
		return "review"
	case "verify":
		return "verification"
	case "reproduce", "localize", "trace", "analyze":
		if primary == "debugging" {
			return "debugging"
		}
	}
	return primary
}
