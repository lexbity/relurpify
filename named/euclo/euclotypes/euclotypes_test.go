package euclotypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteResolutionNormalizeTrimsAndCleansReasons(t *testing.T) {
	resolution := &RouteResolution{
		RouteKind:        "  capability  ",
		ThoughtRecipeID:  " tr-1 ",
		CapabilityID:     " cap.x ",
		ResolutionSource: " keyword ",
		DecidedBy:        " exact-match ",
		Tier2: Tier2Info{
			Outcome:     " applied ",
			Model:       " llama3 ",
			CandidateID: " cap.y ",
		},
		ReasonCodes: []string{" strong-match ", "", "  ", "explicit"},
	}

	resolution.Normalize()

	assert.Equal(t, "capability", resolution.RouteKind)
	assert.Equal(t, "tr-1", resolution.ThoughtRecipeID)
	assert.Equal(t, "cap.x", resolution.CapabilityID)
	assert.Equal(t, "keyword", resolution.ResolutionSource)
	assert.Equal(t, "exact-match", resolution.DecidedBy)
	assert.Equal(t, "applied", resolution.Tier2.Outcome)
	assert.Equal(t, "llama3", resolution.Tier2.Model)
	assert.Equal(t, "cap.y", resolution.Tier2.CandidateID)
	assert.Equal(t, []string{"strong-match", "explicit"}, resolution.ReasonCodes,
		"blank reasons are dropped, surviving order is preserved")
}

func TestRouteResolutionNormalizeNilReceiver(t *testing.T) {
	var resolution *RouteResolution
	resolution.Normalize() // must not panic
}

func TestRouteResolutionNormalizeReasonsCollapseToNil(t *testing.T) {
	emptySlice := &RouteResolution{ReasonCodes: []string{}}
	emptySlice.Normalize()
	assert.Nil(t, emptySlice.ReasonCodes, "empty slice normalizes to nil")

	allBlank := &RouteResolution{ReasonCodes: []string{" ", "\t"}}
	allBlank.Normalize()
	assert.Nil(t, allBlank.ReasonCodes, "reasons that are all blank normalize to nil")

	nilReasons := &RouteResolution{}
	nilReasons.Normalize()
	assert.Nil(t, nilReasons.ReasonCodes)
}

func TestRouteResolutionRouteID(t *testing.T) {
	assert.Equal(t, "", (*RouteResolution)(nil).RouteID(), "nil resolution has no route ID")
	assert.Equal(t, "", (&RouteResolution{}).RouteID())

	assert.Equal(t, "tr-1", (&RouteResolution{ThoughtRecipeID: " tr-1 ", CapabilityID: "cap.x"}).RouteID(),
		"thoughtrecipe ID wins over capability ID")
	assert.Equal(t, "cap.x", (&RouteResolution{CapabilityID: " cap.x "}).RouteID())
}

func TestRouteKindPredicates(t *testing.T) {
	assert.True(t, IsIntentRouteKind("intent"))
	assert.True(t, IsIntentRouteKind(" INTENT "))
	assert.False(t, IsIntentRouteKind("capability"))

	assert.True(t, IsThoughtRecipeRouteKind("thoughtrecipe"))
	assert.True(t, IsThoughtRecipeRouteKind(" ThoughtRecipe "))
	assert.False(t, IsThoughtRecipeRouteKind("intent"))

	assert.True(t, IsCapabilityRouteKind("capability"))
	assert.True(t, IsCapabilityRouteKind("Capability\t"))
	assert.False(t, IsCapabilityRouteKind(""))
}

func TestRouteKindForThoughtRecipeID(t *testing.T) {
	assert.Equal(t, RouteKindIntent, RouteKindForThoughtRecipeID("euclo.thoughtrecipe.intent.summary"),
		"the Euclo intent namespace maps to the intent route kind")
	assert.Equal(t, RouteKindIntent, RouteKindForThoughtRecipeID(" EUCLO.THOUGHTRECIPE.INTENT.X "))
	assert.Equal(t, RouteKindThoughtRecipe, RouteKindForThoughtRecipeID("euclo.thoughtrecipe.plan"))
	assert.Equal(t, RouteKindThoughtRecipe, RouteKindForThoughtRecipeID("euclo.thoughtrecipe.intents"))
	assert.Equal(t, RouteKindThoughtRecipe, RouteKindForThoughtRecipeID(""))
}

func TestRouteKindConstants(t *testing.T) {
	assert.Equal(t, "thoughtrecipe", RouteKindThoughtRecipe)
	assert.Equal(t, "capability", RouteKindCapability)
	assert.Equal(t, "intent", RouteKindIntent)
}

func TestTier2InfoJSONShape(t *testing.T) {
	info := Tier2Info{
		Used:        true,
		Outcome:     "applied",
		Model:       "llama3",
		CandidateID: "cap.y",
		Confidence:  0.87,
		LatencyMs:   120,
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, true, raw["used"])
	assert.Equal(t, "applied", raw["outcome"])
	assert.Equal(t, "llama3", raw["model"])
	assert.Equal(t, "cap.y", raw["candidate_id"])
	assert.Equal(t, 0.87, raw["confidence"])
	assert.Equal(t, float64(120), raw["latency_ms"])

	// Omitempty fields disappear when unset.
	minimal, err := json.Marshal(Tier2Info{})
	require.NoError(t, err)
	assert.Equal(t, `{"used":false}`, string(minimal))
}

func TestRouteContinuationCarriesFields(t *testing.T) {
	continuation := RouteContinuation{
		SharedContext:         true,
		SourceRouteKind:       RouteKindCapability,
		SourceRouteID:         "cap.x",
		TargetRouteKind:       RouteKindIntent,
		TargetRouteID:         "euclo.thoughtrecipe.intent.summarize",
		ActiveThoughtRecipeID: "tr-9",
	}
	assert.True(t, continuation.SharedContext)
	assert.Equal(t, RouteKindCapability, continuation.SourceRouteKind)
	assert.Equal(t, "tr-9", continuation.ActiveThoughtRecipeID)
}

func TestFailureKindClosedTaxonomy(t *testing.T) {
	known := []FailureKind{
		FailureModelUnavailable,
		FailureModelInvalidOutput,
		FailureCapabilityUnavailable,
		FailureCapabilityDenied,
		FailureBudgetExhausted,
		FailureGroundingFailed,
		FailureCancelled,
		FailureUnknown,
	}
	values := map[FailureKind]bool{}
	for _, kind := range known {
		assert.Truef(t, kind.Known(), "expected %q to be a known failure kind", kind)
		values[kind] = true
	}
	assert.Len(t, values, len(known), "taxonomy values are distinct")

	for _, kind := range []FailureKind{"", "timeout", "ModelUnavailable"} {
		assert.Falsef(t, kind.Known(), "expected %q to be unknown", kind)
	}
	assert.Equal(t, "model_unavailable", string(FailureModelUnavailable))
	assert.Equal(t, "unknown", string(FailureUnknown))
}

func TestStepFailureCarriesKindMessageAndCause(t *testing.T) {
	cause := assert.AnError
	failure := StepFailure{Kind: FailureCapabilityDenied, Message: "governance denied tool.invoke", Cause: cause}

	assert.Equal(t, FailureCapabilityDenied, failure.Kind)
	assert.Equal(t, "governance denied tool.invoke", failure.Message)
	assert.Same(t, cause, failure.Cause)

	data, err := json.Marshal(failure)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "Cause", "the originating error is never serialized")
	assert.Contains(t, string(data), `"kind":"capability_denied"`)
	assert.Contains(t, string(data), `"message":"governance denied tool.invoke"`)
}

// FuzzRouteKindForThoughtRecipeID checks the classifier is total: it only ever
// returns one of the two non-capability route kinds regardless of input, and
// the intent namespace is matched case-insensitively on the trimmed value.
func FuzzRouteKindForThoughtRecipeID(f *testing.F) {
	f.Add("euclo.thoughtrecipe.intent.summary")
	f.Add("euclo.thoughtrecipe.plan")
	f.Add("")
	f.Add(" EUCLO.THOUGHTRECIPE.INTENT.X ")
	f.Add("euclo.thoughtrecipe.intents")
	f.Add("\x00weird")

	f.Fuzz(func(t *testing.T, id string) {
		kind := RouteKindForThoughtRecipeID(id)
		if kind != RouteKindIntent && kind != RouteKindThoughtRecipe {
			t.Fatalf("RouteKindForThoughtRecipeID(%q) = %q, want intent or thoughtrecipe", id, kind)
		}
		want := RouteKindIntent
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "euclo.thoughtrecipe.intent.") {
			want = RouteKindThoughtRecipe
		}
		if kind != want {
			t.Fatalf("RouteKindForThoughtRecipeID(%q) = %q, want %q", id, kind, want)
		}

		// RouteID and Normalize stay total over fuzzed inputs.
		resolution := &RouteResolution{
			ThoughtRecipeID: id,
			CapabilityID:    id,
			ReasonCodes:     []string{id, "  "},
		}
		resolution.Normalize()
		assert.Equal(t, strings.TrimSpace(id), resolution.RouteID())
	})
}
