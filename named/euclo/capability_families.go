package euclo

import (
	"fmt"
	"sort"
	"strings"
)

type CapabilityFamilyDescriptor struct {
	FamilyID          string
	PreferredParadigm string
	FallbackParadigms []string
	SupportedModes    []string
	ExpectedArtifacts []string
}

type CapabilityFamilyRegistry struct {
	descriptors map[string]CapabilityFamilyDescriptor
}

type PhaseCapabilityRoute struct {
	Phase  string `json:"phase"`
	Family string `json:"family"`
	Agent  string `json:"agent"`
}

type CapabilityFamilyRouting struct {
	ModeID            string                 `json:"mode_id"`
	ProfileID         string                 `json:"profile_id"`
	PrimaryFamilyID   string                 `json:"primary_family_id"`
	FallbackFamilyIDs []string               `json:"fallback_family_ids,omitempty"`
	Routes            []PhaseCapabilityRoute `json:"routes,omitempty"`
}

func NewCapabilityFamilyRegistry() *CapabilityFamilyRegistry {
	return &CapabilityFamilyRegistry{descriptors: map[string]CapabilityFamilyDescriptor{}}
}

func DefaultCapabilityFamilyRegistry() *CapabilityFamilyRegistry {
	registry := NewCapabilityFamilyRegistry()
	for _, descriptor := range []CapabilityFamilyDescriptor{
		{FamilyID: "implementation", PreferredParadigm: "react", FallbackParadigms: []string{"pipeline"}, SupportedModes: []string{"code", "tdd"}, ExpectedArtifacts: []string{"euclo.edit_intent"}},
		{FamilyID: "debugging", PreferredParadigm: "react", FallbackParadigms: []string{"reflection"}, SupportedModes: []string{"debug"}, ExpectedArtifacts: []string{"euclo.verification"}},
		{FamilyID: "planning", PreferredParadigm: "planner", FallbackParadigms: []string{"react"}, SupportedModes: []string{"planning", "code"}, ExpectedArtifacts: []string{"euclo.plan"}},
		{FamilyID: "review", PreferredParadigm: "reflection", FallbackParadigms: []string{"react"}, SupportedModes: []string{"review"}, ExpectedArtifacts: []string{"euclo.final_report"}},
		{FamilyID: "verification", PreferredParadigm: "react", FallbackParadigms: []string{"reflection"}, SupportedModes: []string{"code", "debug", "tdd"}, ExpectedArtifacts: []string{"euclo.verification"}},
	} {
		_ = registry.Register(descriptor)
	}
	return registry
}

func (r *CapabilityFamilyRegistry) Register(descriptor CapabilityFamilyDescriptor) error {
	if r == nil {
		return fmt.Errorf("capability family registry unavailable")
	}
	id := strings.TrimSpace(strings.ToLower(descriptor.FamilyID))
	if id == "" {
		return fmt.Errorf("family id required")
	}
	descriptor.FamilyID = id
	if strings.TrimSpace(descriptor.PreferredParadigm) == "" {
		return fmt.Errorf("family %s requires preferred paradigm", id)
	}
	r.descriptors[id] = descriptor
	return nil
}

func (r *CapabilityFamilyRegistry) Lookup(id string) (CapabilityFamilyDescriptor, bool) {
	if r == nil {
		return CapabilityFamilyDescriptor{}, false
	}
	descriptor, ok := r.descriptors[strings.TrimSpace(strings.ToLower(id))]
	return descriptor, ok
}

func (r *CapabilityFamilyRegistry) List() []CapabilityFamilyDescriptor {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.descriptors))
	for key := range r.descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]CapabilityFamilyDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.descriptors[key])
	}
	return out
}

func RouteCapabilityFamilies(mode ModeResolution, profile ExecutionProfileSelection, registry *CapabilityFamilyRegistry) CapabilityFamilyRouting {
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
		if descriptor, ok := registry.Lookup(family); ok {
			agent = descriptor.PreferredParadigm
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
