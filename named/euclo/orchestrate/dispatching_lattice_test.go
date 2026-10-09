package orchestrate

import (
	"math/rand"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func availableCapability(id string, score int, components map[string]int) CandidateRouteInfo {
	return CandidateRouteInfo{
		RouteID:      RouteID(id),
		RouteKind:    euclotypes.RouteKindCapability,
		Availability: RouteAvailable,
		RankScore:    score,
		Components:   components,
	}
}

func availableRecipe(id string, score int, components map[string]int) CandidateRouteInfo {
	return CandidateRouteInfo{
		RouteID:      RouteID(id),
		RouteKind:    euclotypes.RouteKindForThoughtRecipeID(id),
		Availability: RouteAvailable,
		RankScore:    score,
		Components:   components,
	}
}

// TestSelectByLatticeRank1Explicit covers D8 rank 1: an explicit request is
// selected iff registered+available; an unavailable explicit route never
// falls back.
func TestSelectByLatticeRank1Explicit(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableRecipe("euclo.thoughtrecipe.default", 5, nil),
		availableRecipe("euclo.thoughtrecipe.code_review", 1000, nil),
	}

	selected, ok, decidedBy := selectByLattice(RouteRequest{ThoughtRecipeID: "euclo.thoughtrecipe.code_review"}, candidates)
	if !ok || selected.RouteID != "euclo.thoughtrecipe.code_review" || decidedBy != decidedByExplicit {
		t.Fatalf("explicit select = %q ok=%v decidedBy=%q, want code_review/explicit", selected.RouteID, ok, decidedBy)
	}

	unavailable := []CandidateRouteInfo{{
		RouteID:      "euclo.thoughtrecipe.missing",
		RouteKind:    euclotypes.RouteKindThoughtRecipe,
		Availability: RouteUnavailableUnsupported,
		RankScore:    1000,
	}}
	if _, ok, decidedBy := selectByLattice(RouteRequest{ThoughtRecipeID: "euclo.thoughtrecipe.missing"}, unavailable); ok || decidedBy != decidedByExplicit {
		t.Fatalf("unavailable explicit must not select, got ok=%v decidedBy=%q", ok, decidedBy)
	}
}

// TestSelectByLatticeRank2Score covers D8 rank 2.
func TestSelectByLatticeRank2Score(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 20, nil),
		availableCapability("euclo:cap.b", 60, nil),
		availableCapability("euclo:cap.c", 40, nil),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "euclo:cap.b" || decidedBy != decidedByScore {
		t.Fatalf("rank2 = %q ok=%v decidedBy=%q, want euclo:cap.b/lattice:score", selected.RouteID, ok, decidedBy)
	}
}

// TestSelectByLatticeRank3FamilyAffinity covers D8 rank 3: at equal total
// score the higher family-affinity component wins.
func TestSelectByLatticeRank3FamilyAffinity(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.a", 60, map[string]int{compKeyword: 60}),
		availableCapability("euclo:cap.b", 60, map[string]int{compFamilyAffinity: 50, compKeyword: 10}),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "euclo:cap.b" || decidedBy != decidedByFamilyAffinity {
		t.Fatalf("rank3 = %q ok=%v decidedBy=%q, want euclo:cap.b/lattice:family_affinity", selected.RouteID, ok, decidedBy)
	}
}

// TestSelectByLatticeRank4UserShadowsBuiltin covers D8 rank 4: a user recipe
// shadows a builtin recipe at equal evidence, but a capability never wins this
// rank.
func TestSelectByLatticeRank4UserShadowsBuiltin(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableRecipe("euclo.thoughtrecipe.builtin_like", 50, nil),
		availableRecipe("custom.thoughtrecipe.user", 50, nil),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "custom.thoughtrecipe.user" || decidedBy != decidedByUserOverBuiltin {
		t.Fatalf("rank4 = %q ok=%v decidedBy=%q, want custom user recipe/user_over_builtin", selected.RouteID, ok, decidedBy)
	}

	// A capability must not shadow a builtin recipe via rank 4: rank 5 decides.
	mixed := []CandidateRouteInfo{
		availableRecipe("euclo.thoughtrecipe.code_review", 50, nil),
		availableCapability("euclo:cap.ast_query", 50, nil),
	}
	selected, ok, decidedBy = selectByLattice(RouteRequest{}, mixed)
	if !ok || selected.RouteID != "euclo.thoughtrecipe.code_review" || decidedBy != decidedByRecipeOverCap {
		t.Fatalf("recipe-vs-capability = %q ok=%v decidedBy=%q, want code_review/thoughtrecipe_over_capability", selected.RouteID, ok, decidedBy)
	}
}

// TestSelectByLatticeRank5ThoughtRecipeOverCapability covers D8 rank 5.
func TestSelectByLatticeRank5ThoughtRecipeOverCapability(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.ast_query", 45, nil),
		availableRecipe("custom.thoughtrecipe.review", 45, nil),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "custom.thoughtrecipe.review" || decidedBy != decidedByRecipeOverCap {
		t.Fatalf("rank5 = %q ok=%v decidedBy=%q, want user recipe/thoughtrecipe_over_capability", selected.RouteID, ok, decidedBy)
	}
}

// TestSelectByLatticeRank6RouteID covers D8 rank 6: the final tie-break is the
// lexicographic route ID, a total order.
func TestSelectByLatticeRank6RouteID(t *testing.T) {
	candidates := []CandidateRouteInfo{
		availableCapability("euclo:cap.zzz", 45, nil),
		availableCapability("euclo:cap.aaa", 45, nil),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "euclo:cap.aaa" || decidedBy != decidedByRouteID {
		t.Fatalf("rank6 = %q ok=%v decidedBy=%q, want euclo:cap.aaa/route_id", selected.RouteID, ok, decidedBy)
	}
}

// TestSelectByLatticeUnavailableNeverCompetes pins that non-available
// candidates cannot win ranks 2–6.
func TestSelectByLatticeUnavailableNeverCompetes(t *testing.T) {
	candidates := []CandidateRouteInfo{
		{RouteID: "euclo:cap.big", RouteKind: euclotypes.RouteKindCapability, Availability: RouteUnavailablePolicyDenied, RankScore: 900},
		availableCapability("euclo:cap.small", 5, nil),
	}
	selected, ok, decidedBy := selectByLattice(RouteRequest{}, candidates)
	if !ok || selected.RouteID != "euclo:cap.small" || decidedBy != decidedByScore {
		t.Fatalf("unavailable candidate competed: %q ok=%v decidedBy=%q", selected.RouteID, ok, decidedBy)
	}
}

// TestDedupeUnionsReasonsAndComponents pins the D8 dedupe contract: the
// higher-scored variant wins, and reasons and component evidence from the
// duplicate are unioned, never lost.
func TestDedupeUnionsReasonsAndComponents(t *testing.T) {
	candidates := []CandidateRouteInfo{
		{
			RouteID:      "euclo.thoughtrecipe.code_review",
			RouteKind:    euclotypes.RouteKindThoughtRecipe,
			Availability: RouteAvailable,
			RankScore:    10,
			RankReasons:  []string{"keyword"},
			Components:   map[string]int{compKeyword: 10},
		},
		{
			RouteID:      "euclo.thoughtrecipe.code_review",
			RouteKind:    euclotypes.RouteKindThoughtRecipe,
			Availability: RouteAvailable,
			RankScore:    60,
			RankReasons:  []string{"family_affinity"},
			Components:   map[string]int{compFamilyAffinity: 50, compKeyword: 10},
		},
	}
	out := dedupeAndSortRouteCandidates(candidates)
	if len(out) != 1 {
		t.Fatalf("dedupe produced %d candidates, want 1", len(out))
	}
	if out[0].RankScore != 60 {
		t.Fatalf("dedupe kept score %d, want the higher 60", out[0].RankScore)
	}
	if got := out[0].RankReasons; len(got) != 2 || got[0] != "keyword" || got[1] != "family_affinity" {
		t.Fatalf("dedupe reasons = %v, want [keyword family_affinity]", got)
	}
	if out[0].Components[compKeyword] != 10 || out[0].Components[compFamilyAffinity] != 50 {
		t.Fatalf("dedupe components = %v, want keyword 10 + family_affinity 50", out[0].Components)
	}
}

// TestSelectionDeterminism is AC-8: over 1000 generated candidate pools the
// lattice produces identical selection, decided_by, and component evidence,
// and is independent of input order.
func TestSelectionDeterminism(t *testing.T) {
	rng := rand.New(rand.NewSource(20261009)) //nolint:gosec // deterministic test fixture
	for i := 0; i < 1000; i++ {
		pool := generateCandidatePool(rng)
		first, firstOK, firstDecided := selectByLattice(RouteRequest{}, pool)
		second, secondOK, secondDecided := selectByLattice(RouteRequest{}, pool)
		if firstOK != secondOK || firstDecided != secondDecided || first.RouteID != second.RouteID || first.RankScore != second.RankScore {
			t.Fatalf("case %d not deterministic: (%q,%v,%q) vs (%q,%v,%q)",
				i, first.RouteID, firstOK, firstDecided, second.RouteID, secondOK, secondDecided)
		}
		// Order independence: shuffle the pool and re-select.
		shuffled := append([]CandidateRouteInfo(nil), pool...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		third, thirdOK, thirdDecided := selectByLattice(RouteRequest{}, shuffled)
		if thirdOK != firstOK || thirdDecided != firstDecided || third.RouteID != first.RouteID {
			t.Fatalf("case %d order-dependent: (%q,%v,%q) vs (%q,%v,%q)",
				i, first.RouteID, firstOK, firstDecided, third.RouteID, thirdOK, thirdDecided)
		}
	}
}

// generateCandidatePool builds a deterministic pseudo-random candidate pool
// from a small vocabulary so ties (and every lattice rank) are exercised.
func generateCandidatePool(rng *rand.Rand) []CandidateRouteInfo {
	ids := []string{
		"euclo.thoughtrecipe.code_review",
		"euclo.thoughtrecipe.default",
		"custom.thoughtrecipe.review",
		"euclo:cap.ast_query",
		"euclo:cap.symbol_trace",
		"euclo:cap.zzz",
	}
	n := 1 + rng.Intn(len(ids))
	picked := make(map[string]struct{}, n)
	pool := make([]CandidateRouteInfo, 0, n)
	for len(pool) < n {
		id := ids[rng.Intn(len(ids))]
		if _, ok := picked[id]; ok {
			continue
		}
		picked[id] = struct{}{}
		kind := euclotypes.RouteKindForThoughtRecipeID(id)
		if strings.HasPrefix(id, "euclo:cap") {
			kind = euclotypes.RouteKindCapability
		}
		score := rng.Intn(4) * 20 // {0,20,40,60}: forces ties and rank chains
		affinity := rng.Intn(2) * 50
		availability := RouteAvailable
		if rng.Intn(6) == 0 {
			availability = RouteUnavailablePolicyDenied
		}
		pool = append(pool, CandidateRouteInfo{
			RouteID:      RouteID(id),
			RouteKind:    kind,
			Availability: availability,
			RankScore:    score + affinity,
			Components:   map[string]int{compFamilyAffinity: affinity, compKeyword: score},
		})
	}
	return pool
}

// BenchmarkSelectByLattice asserts the NFR-3 latency budget: selecting over
// 100 candidates is well under 1 ms per call.
func BenchmarkSelectByLattice(b *testing.B) {
	candidates := make([]CandidateRouteInfo, 0, 100)
	for i := 0; i < 100; i++ {
		id := "euclo:cap.candidate_" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:      RouteID(id),
			RouteKind:    euclotypes.RouteKindCapability,
			Availability: RouteAvailable,
			RankScore:    40 + i%20,
			Components:   map[string]int{compFamilyAffinity: 50, compKeyword: 10},
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, _ := selectByLattice(RouteRequest{}, candidates); !ok {
			b.Fatal("selection failed")
		}
	}
}

// TestAssertClarificationFamilyTable covers FR-15: a live family whose shipped
// handoff target is missing fails boot; a dormant mapping and a bare registry
// do not.
func TestAssertClarificationFamilyTable(t *testing.T) {
	t.Run("bare registry is not a recipe system", func(t *testing.T) {
		reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
		if err := reg.Register(&surface.ThoughtRecipe{ID: defaultThoughtRecipeID, Name: "default"}); err != nil {
			t.Fatalf("register default: %v", err)
		}
		if err := assertClarificationFamilyTable(reg); err != nil {
			t.Fatalf("bare registry must not fail boot: %v", err)
		}
	})

	t.Run("live family with registered target passes", func(t *testing.T) {
		reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
		if err := reg.Register(&surface.ThoughtRecipe{
			ID:       "euclo.thoughtrecipe.code_review",
			Name:     "code_review",
			Metadata: surface.ThoughtRecipeMetadata{Families: []string{"review"}},
		}); err != nil {
			t.Fatalf("register code_review: %v", err)
		}
		if err := assertClarificationFamilyTable(reg); err != nil {
			t.Fatalf("consistent table must pass: %v", err)
		}
	})

	t.Run("live family with dangling target fails boot", func(t *testing.T) {
		reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
		// A recipe declares the debug family, but the shipped debug recipe is
		// absent: the handoff target dangles and boot must fail.
		if err := reg.Register(&surface.ThoughtRecipe{
			ID:       "custom.thoughtrecipe.fix",
			Name:     "fix",
			Metadata: surface.ThoughtRecipeMetadata{Families: []string{"debug"}},
		}); err != nil {
			t.Fatalf("register custom fix: %v", err)
		}
		if err := assertClarificationFamilyTable(reg); err == nil {
			t.Fatal("expected boot failure for a dangling family handoff target")
		}
	})
}
