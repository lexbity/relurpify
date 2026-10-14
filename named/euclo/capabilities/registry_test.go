package capabilities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func TestGetBuiltinFamiliesShape(t *testing.T) {
	families := GetBuiltinFamilies()
	require.Len(t, families, 6)

	wantIDs := []string{
		FamilyCodeUnderstanding,
		FamilyRefactorPatch,
		FamilyVerification,
		FamilyRegressionLocalization,
		FamilyMigrationCompat,
		FamilyReviewSynthesis,
	}
	byID := map[string]CapabilityFamily{}
	for _, family := range families {
		byID[family.ID] = family
		assert.NotEmptyf(t, family.Name, "family %s needs a display name", family.ID)
		assert.NotEmptyf(t, family.Description, "family %s needs a description", family.ID)
		assert.NotEmptyf(t, family.CapabilityIDs, "family %s needs capabilities", family.ID)
	}
	for _, id := range wantIDs {
		family, ok := byID[id]
		require.Truef(t, ok, "expected builtin family %s", id)
		assert.NotEmptyf(t, family.FallbackCapability, "family %s needs a fallback capability", family.ID)
		assert.Containsf(t, family.CapabilityIDs, family.FallbackCapability,
			"family %s fallback must be one of its own capabilities", id)
	}
	// Every capability ID is unique across families.
	seen := map[string]bool{}
	for _, family := range families {
		for _, id := range family.CapabilityIDs {
			assert.Falsef(t, seen[id], "capability %s registered by two families", id)
			seen[id] = true
		}
	}
}

func TestNewRegistryLoadsBuiltins(t *testing.T) {
	registry := NewRegistry()

	names := registry.CallableCapabilityNames()
	require.NotEmpty(t, names)
	assert.Contains(t, names, "euclo:cap.ast_query")
	assert.Contains(t, names, "euclo:cap.bisect")
	assert.Contains(t, names, "euclo:cap.test_run")
	assert.IsIncreasing(t, names, "capability names are returned sorted")

	desc, found := registry.GetCapability("euclo:cap.ast_query")
	require.True(t, found)
	assert.Equal(t, agentspec.CapabilityKindTool, desc.Kind)
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
	assert.Equal(t, "Ast query", desc.Name, "display name derives from the capability ID")
	assert.Equal(t, FamilyCodeUnderstanding, desc.Category)
	// Tags pass through descriptor normalization, which kebab-cases them.
	assert.Equal(t, []string{"code-understanding"}, desc.Tags)
	assert.True(t, desc.Availability.Available)

	// All builtin descriptors are listed in stable ID order.
	listed := registry.ListCapabilities()
	require.Len(t, listed, len(names))
	assert.Equal(t, names[0], listed[0].ID)
}

func TestRegistryRegisterFamilyValidation(t *testing.T) {
	registry := NewRegistry()

	err := registry.RegisterFamily(CapabilityFamily{ID: "  "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "family id required")

	require.NoError(t, registry.RegisterFamily(CapabilityFamily{ID: "custom"}))
	err = registry.RegisterFamily(CapabilityFamily{ID: "custom"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `family "custom" already registered`)
}

func TestRegistryRegisterCapabilityValidationAndDefaults(t *testing.T) {
	registry := NewRegistry()

	require.Error(t, (*EucloCapabilityRegistry)(nil).RegisterCapability(descriptor.CapabilityDescriptor{ID: "x"}, "f"))
	require.Error(t, (*EucloCapabilityRegistry)(nil).RegisterFamily(CapabilityFamily{ID: "f"}))

	err := registry.RegisterCapability(descriptor.CapabilityDescriptor{ID: "  "}, "family")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability id required")

	err = registry.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.x"}, "  ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "family id required")

	minimal := descriptor.CapabilityDescriptor{ID: "euclo:cap.bare"}
	require.NoError(t, registry.RegisterCapability(minimal, "custom-family"))

	stored, found := registry.GetCapability("euclo:cap.bare")
	require.True(t, found)
	// NOTE: descriptor normalization runs first and derives "local-tool" from
	// the defaulted tool kind, so this package's own relurpic fallback
	// (registry.go) is unreachable for bare descriptors.
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyLocalTool, stored.RuntimeFamily)
	assert.Equal(t, agentspec.CapabilityKindTool, stored.Kind, "tool is the default kind")
	assert.Equal(t, "euclo:cap.bare", stored.Name, "ID doubles as the display name when unset")

	err = registry.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.bare"}, "custom-family")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `capability "euclo:cap.bare" already registered`)

	// A second distinct capability joins the same family list.
	require.NoError(t, registry.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.bare2"}, "multi"))
	require.NoError(t, registry.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.bare3"}, "multi"))
	assert.Equal(t, []string{"euclo:cap.bare2", "euclo:cap.bare3"},
		memberIDs(registry, "multi"), "family membership tracks every bound capability")
}

// memberIDs projects the per-family candidates for an instruction matching
// every member keyword.
func memberIDs(registry *EucloCapabilityRegistry, family string) []string {
	var out []string
	for _, candidate := range registry.MatchByKeywords("bare2 bare3", family, nil) {
		out = append(out, candidate.CapabilityID)
	}
	return out
}

func TestRegistryGetCapabilityMissesAndNilSafety(t *testing.T) {
	desc, found := (*EucloCapabilityRegistry)(nil).GetCapability("euclo:cap.ast_query")
	assert.False(t, found)
	assert.Empty(t, desc.ID)

	registry := NewRegistry()
	_, found = registry.GetCapability("euclo:cap.does_not_exist")
	assert.False(t, found)
}

func TestRegistryCallableCapabilityNamesNilSafety(t *testing.T) {
	assert.Nil(t, (*EucloCapabilityRegistry)(nil).CallableCapabilityNames())
	assert.Nil(t, (*EucloCapabilityRegistry)(nil).ListCapabilities())
	assert.Nil(t, (*EucloCapabilityRegistry)(nil).MatchByKeywords("fix", "debug", nil))
	fallback, found := (*EucloCapabilityRegistry)(nil).FallbackForFamily(FamilyVerification)
	assert.False(t, found)
	assert.Equal(t, descriptor.CapabilityDescriptor{}, fallback)
}

func TestRegistryFallbackForFamilyPrefersDeclaredFallback(t *testing.T) {
	registry := NewRegistry()

	desc, found := registry.FallbackForFamily(FamilyVerification)
	require.True(t, found)
	assert.Equal(t, "euclo:cap.test_run", desc.ID, "declared family fallback wins")

	desc, found = registry.FallbackForFamily(FamilyRegressionLocalization)
	require.True(t, found)
	assert.Equal(t, "euclo:cap.bisect", desc.ID)

	t.Run("falls back to first registered member when declared fallback missing", func(t *testing.T) {
		custom := NewRegistry()
		require.NoError(t, custom.RegisterFamily(CapabilityFamily{ID: "custom", FallbackCapability: "euclo:cap.absent"}))
		require.NoError(t, custom.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.member-b"}, "custom"))
		require.NoError(t, custom.RegisterCapability(descriptor.CapabilityDescriptor{ID: "euclo:cap.member-a"}, "custom"))

		desc, found := custom.FallbackForFamily("custom")
		require.True(t, found)
		assert.Equal(t, "euclo:cap.member-b", desc.ID, "first registered member is the structural fallback")
	})

	t.Run("unknown family has no fallback", func(t *testing.T) {
		_, found := registry.FallbackForFamily("nonexistent-family")
		assert.False(t, found)
	})
}

func TestRegistryMatchByKeywordsScoresAndSorts(t *testing.T) {
	registry := NewRegistry()

	candidates := registry.MatchByKeywords("please run the coverage check for the test run", FamilyVerification, nil)
	require.Len(t, candidates, 2)
	// Both capabilities score 2.0 (test+run, coverage+check); the tie is
	// broken by capability ID.
	assert.Equal(t, "euclo:cap.coverage_check", candidates[0].CapabilityID)
	assert.InDelta(t, 2.0, candidates[0].Score, 1e-9)
	assert.Equal(t, "euclo:cap.test_run", candidates[1].CapabilityID)
	assert.InDelta(t, 2.0, candidates[1].Score, 1e-9)

	t.Run("negative constraints halve scores", func(t *testing.T) {
		instruction := "run coverage check but skip test"
		plain := registry.MatchByKeywords(instruction, FamilyVerification, nil)
		require.Len(t, plain, 2)

		penalized := registry.MatchByKeywords(instruction, FamilyVerification, []string{"skip"})
		require.Len(t, penalized, 2)
		for i := range plain {
			assert.Equal(t, plain[i].CapabilityID, penalized[i].CapabilityID)
			assert.InDeltaf(t, plain[i].Score/2, penalized[i].Score, 1e-9,
				"one matching constraint must halve the score of %s", plain[i].CapabilityID)
		}
	})

	t.Run("unknown family returns no candidates", func(t *testing.T) {
		assert.Empty(t, registry.MatchByKeywords("anything", "no-such-family", nil))
	})

	t.Run("zero-score candidates are dropped", func(t *testing.T) {
		assert.Empty(t, registry.MatchByKeywords("unrelated words entirely", FamilyVerification, nil))
	})

	t.Run("higher score sorts before lower score", func(t *testing.T) {
		// "run test coverage" scores test_run=2 vs coverage_check=1, so the
		// score comparator (not the ID tie-break) decides the order.
		scored := registry.MatchByKeywords("run test coverage", FamilyVerification, nil)
		require.Len(t, scored, 2)
		assert.Equal(t, "euclo:cap.test_run", scored[0].CapabilityID)
		assert.Equal(t, "euclo:cap.coverage_check", scored[1].CapabilityID)
	})
}

func TestFallbackForFamilyUsesHardcodedFamilyDefaults(t *testing.T) {
	// A family registered without a declared fallback relies on the hardcoded
	// per-family defaults. Builtins always declare one, so this strips the
	// declared fallbacks in place (in-package test) to reach that path.
	defaults := map[string]string{
		FamilyCodeUnderstanding:      "euclo:cap.ast_query",
		FamilyRefactorPatch:          "euclo:cap.targeted_refactor",
		FamilyVerification:           "euclo:cap.test_run",
		FamilyRegressionLocalization: "euclo:cap.bisect",
		FamilyMigrationCompat:        "euclo:cap.api_compat",
		FamilyReviewSynthesis:        "euclo:cap.code_review",
	}

	registry := NewRegistry()
	registry.mu.Lock()
	for family := range defaults {
		base := registry.families[family]
		base.FallbackCapability = ""
		registry.families[family] = base
	}
	registry.mu.Unlock()

	for family, want := range defaults {
		desc, found := registry.FallbackForFamily(family)
		require.Truef(t, found, "fallback for %s", family)
		assert.Equalf(t, want, desc.ID, "hardcoded fallback for %s", family)
	}
}

func TestMatchByKeywordsPackageFunctionDelegates(t *testing.T) {
	assert.Nil(t, MatchByKeywords("fix it", FamilyVerification, nil, nil),
		"nil registry yields no candidates")

	registry := NewRegistry()
	candidates := MatchByKeywords("bisect the regression", FamilyRegressionLocalization, registry, nil)
	require.Len(t, candidates, 1)
	assert.Equal(t, "euclo:cap.bisect", candidates[0].CapabilityID)
	assert.Equal(t, FamilyRegressionLocalization, candidates[0].FamilyID)
}

func TestFallbackForFamilyPackageFunction(t *testing.T) {
	registry := NewRegistry()

	id, found := FallbackForFamily(FamilyCodeUnderstanding, registry)
	require.True(t, found)
	assert.Equal(t, "euclo:cap.ast_query", id)

	// Family names outside the capability registry fall back to the static map.
	for family, want := range map[string]string{
		"debug":  "euclo:cap.bisect",
		"repair": "euclo:cap.targeted_refactor",
		"review": "euclo:cap.code_review",
	} {
		gotID, ok := FallbackForFamily(family, nil)
		require.Truef(t, ok, "static fallback for %s", family)
		assert.Equalf(t, want, gotID, "static fallback for %s", family)
	}

	gotID, ok := FallbackForFamily("no-such-family", nil)
	assert.False(t, ok)
	assert.Empty(t, gotID)
}

func TestScoreCapabilityByKeywordsAndExtraction(t *testing.T) {
	assert.Empty(t, extractKeywordsFromCapabilityID("no-colon-here"))
	assert.Equal(t, []string{"bisect"}, extractKeywordsFromCapabilityID("euclo:cap.bisect"))
	assert.Equal(t, []string{"test", "run"}, extractKeywordsFromCapabilityID("euclo:cap.test_run"))

	score := scoreCapabilityByKeywords("euclo:cap.test_run", "please run the test", nil)
	assert.InDelta(t, 2.0, score, 1e-9, "test and run both appear in the instruction")

	assert.InDelta(t, 0.0, scoreCapabilityByKeywords("euclo:cap.test_run", "nothing relevant", nil), 1e-9)

	halved := scoreCapabilityByKeywords("euclo:cap.test_run", "run the test but skip run", []string{"skip"})
	assert.InDelta(t, 1.0, halved, 1e-9, "one matching constraint halves the score")
}

func TestCapabilityTypesCarryFields(t *testing.T) {
	family := CapabilityFamily{ID: "f", Name: "n", Description: "d", CapabilityIDs: []string{"a"}, FallbackCapability: "a"}
	assert.Equal(t, "d", family.Description)

	selection := CapabilitySelection{
		CapabilityID: "euclo:cap.bisect",
		FamilyID:     FamilyRegressionLocalization,
		MatchReason:  "keyword overlap",
		Confidence:   0.9,
	}
	assert.Equal(t, "keyword overlap", selection.MatchReason)
	assert.InDelta(t, 0.9, selection.Confidence, 1e-9)

	candidate := CapabilityCandidate{CapabilityID: "euclo:cap.bisect", FamilyID: FamilyRegressionLocalization, Score: 2}
	assert.Equal(t, 2.0, candidate.Score)
}

// failingRegistry is a capabilityInvoker stub with scriptable behavior.
type scriptedRegistry struct {
	calls    []string
	results  map[string]*ports.ToolResult
	errs     map[string]error
	lastArgs map[string]any
}

func (r *scriptedRegistry) InvokeCapability(_ context.Context, _ ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	r.calls = append(r.calls, idOrName)
	r.lastArgs = args
	if err, ok := r.errs[idOrName]; ok {
		return nil, err
	}
	if result, ok := r.results[idOrName]; ok {
		return result, nil
	}
	return &ports.ToolResult{Success: true, Data: map[string]any{"capability": idOrName}}, nil
}

func TestInvokeCapabilityRegistryErrorYieldsErrorResult(t *testing.T) {
	registry := &scriptedRegistry{errs: map[string]error{
		"euclo:cap.boom": errors.New("handler missing"),
	}}

	result, err := InvokeCapability(context.Background(), "euclo:cap.boom", &execution.Task{}, nil, registry)

	require.Error(t, err)
	assert.Equal(t, "handler missing", err.Error())
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "euclo:cap.boom", result.NodeID)
	message, ok := execution.ResultField(result.Data, "error")
	require.True(t, ok)
	assert.Equal(t, "handler missing", message)
}

func TestInvokeCapabilityNilToolResultYieldsErrorResult(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.void": nil,
	}}

	result, err := InvokeCapability(context.Background(), "euclo:cap.void", nil, nil, registry)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry returned nil result for capability euclo:cap.void")
	assert.False(t, result.Success)
	message, ok := execution.ResultField(result.Data, "error")
	require.True(t, ok)
	assert.Contains(t, message, "nil result")
}

func TestInvokeCapabilityToolResultErrorSurfacesAsError(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.partial": {Success: false, Data: map[string]any{"out": "half"}, Error: "partial failure"},
	}}

	result, err := InvokeCapability(context.Background(), "euclo:cap.partial", nil, nil, registry)

	require.Error(t, err)
	assert.Equal(t, "partial failure", err.Error())
	require.NotNil(t, result)
	assert.False(t, result.Success)
	field, ok := execution.ResultField(result.Data, "out")
	require.True(t, ok)
	assert.Equal(t, "half", field)
}

func TestInvokeCapabilitySuccessCarriesToolData(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.ok": {Success: true, Data: map[string]any{"answer": 42}},
	}}

	result, err := InvokeCapability(context.Background(), "euclo:cap.ok", &execution.Task{}, nil, registry)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "euclo:cap.ok", result.NodeID)
	field, ok := execution.ResultField(result.Data, "answer")
	require.True(t, ok)
	assert.Equal(t, 42, field)
}

func TestInvokeCapabilitySequenceEmptyListFails(t *testing.T) {
	result, err := InvokeCapabilitySequence(context.Background(), nil, "AND", nil, nil, &scriptedRegistry{})
	require.Error(t, err)
	assert.Equal(t, "no capabilities to invoke", err.Error())
	assert.False(t, result.Success)
	message, _ := execution.ResultField(result.Data, "error")
	assert.Equal(t, "no capabilities to invoke", message)
}

func TestInvokeCapabilitySequenceANDShortCircuitsOnFailure(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.fail": {Success: false, Error: "second failed"},
	}}

	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.ok", "euclo:cap.fail", "euclo:cap.never"}, "AND", nil, nil, registry)

	require.Error(t, err)
	assert.Equal(t, "second failed", err.Error())
	assert.False(t, result.Success)
	assert.Equal(t, []string{"euclo:cap.ok", "euclo:cap.fail"}, registry.calls,
		"AND stops at the first failing capability")
}

func TestInvokeCapabilitySequenceANDAllSucceed(t *testing.T) {
	registry := &scriptedRegistry{}

	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.a", "euclo:cap.b"}, "AND", nil, nil, registry)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, registry.calls, 2)
	operator, ok := execution.ResultField(result.Data, "sequence_operator")
	require.True(t, ok)
	assert.Equal(t, "AND", operator)
	caps, ok := execution.ResultField(result.Data, "capabilities")
	require.True(t, ok)
	assert.Equal(t, []string{"euclo:cap.a", "euclo:cap.b"}, caps)
}

func TestInvokeCapabilitySequenceORStopsAtFirstSuccess(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.bad": {Success: false, Error: "bad"},
	}}

	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.bad", "euclo:cap.good"}, "OR", nil, nil, registry)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, []string{"euclo:cap.bad", "euclo:cap.good"}, registry.calls)
	assert.Equal(t, "euclo:cap.good", result.NodeID, "the first succeeding capability is the result")
}

func TestInvokeCapabilitySequenceORAllFailReportsLastError(t *testing.T) {
	registry := &scriptedRegistry{errs: map[string]error{
		"euclo:cap.a": errors.New("first failure"),
		"euclo:cap.b": errors.New("second failure"),
	}}

	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.a", "euclo:cap.b"}, "OR", nil, nil, registry)

	require.Error(t, err)
	assert.Equal(t, "second failure", err.Error(), "OR reports the last capability error")
	assert.False(t, result.Success)
	message, ok := execution.ResultField(result.Data, "error")
	require.True(t, ok)
	assert.Equal(t, "all capabilities in OR sequence failed", message)
}

func TestInvokeCapabilitySequenceORFailureWithoutError(t *testing.T) {
	registry := &scriptedRegistry{results: map[string]*ports.ToolResult{
		"euclo:cap.a": {Success: false},
		"euclo:cap.b": {Success: false},
	}}

	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.a", "euclo:cap.b"}, "OR", nil, nil, registry)

	// Errorless failed ToolResults leave lastError nil: the failure is only
	// visible through the result payload, not the error return.
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestInvokeCapabilitySequenceUnknownOperatorFails(t *testing.T) {
	registry := &scriptedRegistry{}
	result, err := InvokeCapabilitySequence(context.Background(),
		[]string{"euclo:cap.a"}, "XOR", nil, nil, registry)

	require.Error(t, err)
	assert.Equal(t, "unknown operator: XOR", err.Error())
	assert.False(t, result.Success)
	message, _ := execution.ResultField(result.Data, "error")
	assert.Equal(t, "unknown operator: XOR", message)
	assert.Empty(t, registry.calls, "no capability runs for an unknown operator")
}

func TestInvokeCapabilitySequenceCaseSensitiveOperators(t *testing.T) {
	registry := &scriptedRegistry{}
	result, err := InvokeCapabilitySequence(context.Background(), []string{"euclo:cap.a"}, "and", nil, nil, registry)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown operator: and")
	assert.False(t, result.Success)
}

// FuzzScoreCapabilityByKeywords checks scoring is total and non-negative for
// arbitrary instructions, capability IDs, and derived constraints.
func FuzzScoreCapabilityByKeywords(f *testing.F) {
	f.Add("run the test", "euclo:cap.test_run", true)
	f.Add("", "euclo:cap.bisect", false)
	f.Add("BISECT bisect _", "euclo:cap.bisect", true)
	f.Add("\x00\xff", ":", true)

	f.Fuzz(func(t *testing.T, instruction, capID string, penalize bool) {
		var constraints []string
		if penalize {
			constraints = strings.Fields(instruction)
		}
		score := scoreCapabilityByKeywords(capID, instruction, constraints)
		if score < 0 {
			t.Fatalf("score for capID %q must never be negative, got %f", capID, score)
		}
	})
}
