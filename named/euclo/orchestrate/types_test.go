package orchestrate

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestRouteResolutionRouteIDPrefersThoughtRecipe(t *testing.T) {
	resolution := &euclotypes.RouteResolution{
		RouteKind:       euclotypes.RouteKindIntent,
		ThoughtRecipeID: "euclo.thoughtrecipe.intent.clarify",
		CapabilityID:    "euclo:cap.ast_query",
	}

	if got := resolution.RouteID(); got != "euclo.thoughtrecipe.intent.clarify" {
		t.Fatalf("RouteID = %q, want thoughtrecipe id", got)
	}
}

func TestRouteResolutionNormalizeTrimsAndDropsEmptyReasons(t *testing.T) {
	resolution := &euclotypes.RouteResolution{
		RouteKind:        " capability ",
		ThoughtRecipeID:  " ",
		CapabilityID:     " euclo:cap.ast_query ",
		ResolutionSource: " deterministic ",
		ReasonCodes:      []string{" explicit ", " ", "\t"},
	}

	resolution.Normalize()

	if resolution.RouteKind != "capability" {
		t.Fatalf("RouteKind = %q, want capability", resolution.RouteKind)
	}
	if resolution.CapabilityID != "euclo:cap.ast_query" {
		t.Fatalf("CapabilityID = %q, want euclo:cap.ast_query", resolution.CapabilityID)
	}
	if resolution.ResolutionSource != "deterministic" {
		t.Fatalf("ResolutionSource = %q, want deterministic", resolution.ResolutionSource)
	}
	if len(resolution.ReasonCodes) != 1 || resolution.ReasonCodes[0] != "explicit" {
		t.Fatalf("unexpected reason codes: %#v", resolution.ReasonCodes)
	}
}

func TestRouteSelectionAndResolutionAreDistinctRecords(t *testing.T) {
	selection := &euclotypes.RouteSelection{
		RouteKind:       euclotypes.RouteKindCapability,
		CapabilityID:    "euclo:cap.ast_query",
		ThoughtRecipeID: "",
	}
	resolution := &euclotypes.RouteResolution{
		RouteKind:                 euclotypes.RouteKindCapability,
		CapabilityID:              "euclo:cap.ast_query",
		ResolutionSource:          "deterministic",
		ClarificationStateVersion: 3,
	}

	if selection.RouteKind != resolution.RouteKind {
		t.Fatalf("expected route kinds to match for the same route")
	}
	if selection.CapabilityID != resolution.CapabilityID {
		t.Fatalf("expected capability ids to match")
	}
	if resolution.ClarificationStateVersion != 3 {
		t.Fatalf("ClarificationStateVersion = %d, want 3", resolution.ClarificationStateVersion)
	}
}
