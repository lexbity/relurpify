package policyresolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// fakeDescriptor is a fully configured ports.DescriptorView used to exercise
// every selector-matching predicate over real values.
type fakeDescriptor struct {
	id                          string
	name                        string
	kind                        string
	runtimeFamily               string
	description                 string
	version                     string
	category                    string
	tags                        []string
	trustClass                  string
	riskClasses                 []risk.RiskClass
	effectClasses               []classification.EffectClass
	sourceProviderID            string
	sourceScope                 string
	sourceSessionID             string
	coordinationRole            string
	coordinationTarget          bool
	coordinationTaskTypes       []string
	coordinationExecutionModes  []string
	coordinationLongRunning     int32
	coordinationDirectInsertion int32
	coordinationMaxDepth        int
	coordinationMaxRuntime      int
}

func (d fakeDescriptor) CapabilityID() string                        { return d.id }
func (d fakeDescriptor) CapabilityName() string                      { return d.name }
func (d fakeDescriptor) CapabilityKind() string                      { return d.kind }
func (d fakeDescriptor) RuntimeFamily() string                       { return d.runtimeFamily }
func (d fakeDescriptor) Description() string                         { return d.description }
func (d fakeDescriptor) Version() string                             { return d.version }
func (d fakeDescriptor) Category() string                            { return d.category }
func (d fakeDescriptor) Tags() []string                              { return d.tags }
func (d fakeDescriptor) TrustClass() string                          { return d.trustClass }
func (d fakeDescriptor) RiskClasses() []risk.RiskClass               { return d.riskClasses }
func (d fakeDescriptor) EffectClasses() []classification.EffectClass { return d.effectClasses }
func (d fakeDescriptor) SourceProviderID() string                    { return d.sourceProviderID }
func (d fakeDescriptor) SourceScope() string                         { return d.sourceScope }
func (d fakeDescriptor) SourceSessionID() string                     { return d.sourceSessionID }
func (d fakeDescriptor) CoordinationRole() string                    { return d.coordinationRole }
func (d fakeDescriptor) CoordinationTarget() bool                    { return d.coordinationTarget }
func (d fakeDescriptor) CoordinationTaskTypes() []string             { return d.coordinationTaskTypes }
func (d fakeDescriptor) CoordinationExecutionModes() []string        { return d.coordinationExecutionModes }
func (d fakeDescriptor) CoordinationLongRunning() int32              { return d.coordinationLongRunning }
func (d fakeDescriptor) CoordinationDirectInsertionAllowed() int32 {
	return d.coordinationDirectInsertion
}
func (d fakeDescriptor) CoordinationMaxDepth() int          { return d.coordinationMaxDepth }
func (d fakeDescriptor) CoordinationMaxRuntimeSeconds() int { return d.coordinationMaxRuntime }

var _ ports.DescriptorView = fakeDescriptor{}

// fakeRegistry implements RegistryView over a set of descriptors. Descriptors
// whose id starts with "hidden" report a non-callable effective exposure.
type fakeRegistry struct {
	descriptors map[string]fakeDescriptor
}

func newFakeRegistry(descriptors ...fakeDescriptor) *fakeRegistry {
	r := &fakeRegistry{descriptors: make(map[string]fakeDescriptor, len(descriptors))}
	for _, d := range descriptors {
		r.descriptors[d.id] = d
	}
	return r
}

func (r *fakeRegistry) GetCapability(idOrName string) (any, bool) {
	for _, d := range r.descriptors {
		if d.id == idOrName || d.name == idOrName {
			return d, true
		}
	}
	return nil, false
}

func (r *fakeRegistry) CallableCapabilities() []any {
	var out []any
	for _, d := range r.descriptors {
		out = append(out, d)
	}
	return out
}

func (r *fakeRegistry) EffectiveExposure(desc any) string {
	d, ok := desc.(fakeDescriptor)
	if !ok {
		return ""
	}
	if len(d.id) >= 6 && d.id[:6] == "hidden" {
		return "hidden"
	}
	return "callable"
}

var _ RegistryView = (*fakeRegistry)(nil)

func sampleDescriptor(id, name string) fakeDescriptor {
	return fakeDescriptor{
		id:            id,
		name:          name,
		kind:          "tool",
		runtimeFamily: "relurpic",
		description:   "desc for " + id,
		version:       "1.0.0",
		category:      "testing",
		tags:          []string{"edit", "fast"},
		trustClass:    "builtin-trusted",
		riskClasses:   []risk.RiskClass{risk.RiskClassExecute},
		effectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
		sourceScope:   "builtin",
	}
}

func TestResolveAgentPolicyEmptyConfigYieldsNilPhaseCapabilities(t *testing.T) {
	resolved := ResolveAgentPolicy(newFakeRegistry(), ResolvedAgentOrchestrationConfig{})
	assert.Nil(t, resolved.PhaseCapabilities)
	assert.Empty(t, resolved.VerificationSuccessCapabilities)
	assert.Empty(t, resolved.RecoveryProbeCapabilities)
	assert.False(t, resolved.Planning.RequireVerificationStep)
	assert.Nil(t, resolved.Review.SeverityWeights)
}

func TestResolveAgentPolicyExplicitNamesFilteredByExposure(t *testing.T) {
	registry := newFakeRegistry(
		sampleDescriptor("cap.alpha", "Alpha"),
		sampleDescriptor("hidden.cap", "HiddenCap"),
	)
	config := ResolvedAgentOrchestrationConfig{
		PhaseCapabilities: map[string][]string{
			"explore": {" cap.alpha ", "hidden.cap", "  ", "unknown.cap"},
		},
	}

	resolved := ResolveAgentPolicy(registry, config)

	require.Len(t, resolved.PhaseCapabilities, 1)
	assert.Equal(t, []string{"Alpha"}, resolved.PhaseCapabilities["explore"],
		"whitespace is trimmed, non-callable and unknown capabilities are dropped")
}

func TestResolveAgentPolicyMergesPhaseCapabilitiesAndSelectors(t *testing.T) {
	registry := newFakeRegistry(
		sampleDescriptor("cap.alpha", "Alpha"),
		sampleDescriptor("cap.beta", "Beta"),
	)
	config := ResolvedAgentOrchestrationConfig{
		PhaseCapabilities: map[string][]string{
			"edit": {"cap.alpha"},
		},
		PhaseCapabilitySelectors: map[string][]ResolvedCapabilitySelector{
			// Selector names resolve against the descriptor display name, not its ID.
			"edit": {{Name: " Alpha "}, {Name: "Beta"}},
		},
	}

	resolved := ResolveAgentPolicy(registry, config)

	assert.Equal(t, []string{"Alpha", "Beta"}, resolved.PhaseCapabilities["edit"],
		"explicit names and selector matches merge without duplicates")
}

func TestResolveAgentPolicyVerificationAndRecoverySelectors(t *testing.T) {
	alpha := sampleDescriptor("cap.alpha", "Alpha")
	registry := newFakeRegistry(alpha, sampleDescriptor("cap.beta", "Beta"))

	config := ResolvedAgentOrchestrationConfig{
		Verification: ResolvedAgentVerificationPolicy{
			SuccessTools:               []string{"Alpha"},
			SuccessCapabilitySelectors: []ResolvedCapabilitySelector{{ID: " cap.alpha "}},
			StopOnSuccess:              true,
		},
		Recovery: ResolvedAgentRecoveryPolicy{
			FailureProbeCapabilitySelectors: []ResolvedCapabilitySelector{
				{Kind: "tool", RuntimeFamilies: []string{"relurpic"}},
			},
		},
	}

	resolved := ResolveAgentPolicy(registry, config)

	assert.Equal(t, []string{"Alpha"}, resolved.VerificationSuccessCapabilities,
		"name lookup is case-insensitive and ID selector resolves via registry")
	assert.Equal(t, []string{"Alpha", "Beta"}, resolved.RecoveryProbeCapabilities,
		"attribute-only selector scans all matching callable candidates")
}

func TestResolveAgentPolicyPlanningAndReviewPassthrough(t *testing.T) {
	registry := newFakeRegistry(sampleDescriptor("cap.alpha", "Alpha"))
	criteria := []string{"correctness"}
	focusTags := []string{"security"}
	weights := map[string]float64{"high": 2.0}

	config := ResolvedAgentOrchestrationConfig{
		Planning: ResolvedAgentPlanningPolicy{
			RequiredBeforeEdit:          []ResolvedCapabilitySelector{{Name: "Alpha"}},
			PreferredEditCapabilities:   []ResolvedCapabilitySelector{{ID: "cap.alpha"}},
			PreferredVerifyCapabilities: []ResolvedCapabilitySelector{{ID: "cap.missing"}},
			StepTemplates:               []policy.ResolvedSkillStepTemplate{{Kind: "verify", Description: "run tests"}},
			RequireVerificationStep:     true,
		},
		Review: ResolvedAgentReviewPolicy{
			Criteria:        criteria,
			FocusTags:       focusTags,
			SeverityWeights: weights,
			ApprovalRules:   policy.ResolvedAgentReviewApprovalRules{RequireVerificationEvidence: true},
		},
	}

	resolved := ResolveAgentPolicy(registry, config)

	assert.Equal(t, []string{"Alpha"}, resolved.Planning.RequiredBeforeEdit)
	assert.Equal(t, []string{"Alpha"}, resolved.Planning.PreferredEditCapabilities)
	assert.Empty(t, resolved.Planning.PreferredVerifyCapabilities, "unknown selector resolves to nothing")
	assert.Equal(t, config.Planning.StepTemplates, resolved.Planning.StepTemplates)
	assert.True(t, resolved.Planning.RequireVerificationStep)
	assert.Equal(t, []string{"correctness"}, resolved.Review.Criteria)
	assert.Equal(t, []string{"security"}, resolved.Review.FocusTags)
	assert.Equal(t, map[string]float64{"high": 2.0}, resolved.Review.SeverityWeights)
	assert.True(t, resolved.Review.ApprovalRules.RequireVerificationEvidence)

	// Resolved copies must be independent of the caller's slices and maps.
	criteria[0] = "mutated"
	focusTags[0] = "mutated"
	weights["high"] = -1
	assert.Equal(t, []string{"correctness"}, resolved.Review.Criteria)
	assert.Equal(t, []string{"security"}, resolved.Review.FocusTags)
	assert.Equal(t, map[string]float64{"high": 2.0}, resolved.Review.SeverityWeights)
}

func TestResolveEffectiveAgentPolicyBundlesSpecAndPolicy(t *testing.T) {
	registry := newFakeRegistry(sampleDescriptor("cap.alpha", "Alpha"))
	config := ResolvedAgentOrchestrationConfig{
		PhaseCapabilities: map[string][]string{"verify": {"cap.alpha"}},
	}

	effective := ResolveEffectiveAgentPolicy(config, registry)

	assert.Equal(t, config, effective.Spec)
	assert.Equal(t, []string{"Alpha"}, effective.Policy.PhaseCapabilities["verify"])
}

func TestResolveCapabilityNamesWithoutRegistryPassesNamesThrough(t *testing.T) {
	config := ResolvedAgentOrchestrationConfig{
		PhaseCapabilities: map[string][]string{"explore": {"  cap.raw  ", "", "cap.raw"}},
		Planning: ResolvedAgentPlanningPolicy{
			PreferredEditCapabilities: []ResolvedCapabilitySelector{{Name: " raw.plan "}},
		},
	}

	resolved := ResolveAgentPolicy(nil, config)

	assert.Equal(t, []string{"cap.raw"}, resolved.PhaseCapabilities["explore"],
		"without a registry, explicit names pass through trimmed and deduplicated")
	assert.Equal(t, []string{"raw.plan"}, resolved.Planning.PreferredEditCapabilities,
		"without a registry, named selectors pass their name through trimmed")
}

func TestResolveCapabilityNamesUnnamedSelectorScansSortedCandidates(t *testing.T) {
	// Deliberately registered out of resolved-name order to prove candidate sorting.
	registry := newFakeRegistry(
		sampleDescriptor("cap.zulu", "Zulu"),
		sampleDescriptor("cap.alpha", "Alpha"),
		sampleDescriptor("cap.beta", "Beta"),
	)

	config := ResolvedAgentOrchestrationConfig{
		Planning: ResolvedAgentPlanningPolicy{
			PreferredVerifyCapabilities: []ResolvedCapabilitySelector{
				{Tags: []string{"edit"}},
			},
		},
	}

	resolved := ResolveAgentPolicy(registry, config)

	assert.Equal(t, []string{"Alpha", "Beta", "Zulu"}, resolved.Planning.PreferredVerifyCapabilities,
		"attribute-only selectors match candidates in stable resolved-name order")
}

func TestSelectorCapabilityNamePrefersNameOverID(t *testing.T) {
	assert.Equal(t, "ByName", selectorCapabilityName(ResolvedCapabilitySelector{Name: " ByName ", ID: "by-id"}))
	assert.Equal(t, "by-id", selectorCapabilityName(ResolvedCapabilitySelector{ID: " by-id "}))
	assert.Equal(t, "", selectorCapabilityName(ResolvedCapabilitySelector{}))
}

func TestResolvedCapabilityNameFallsBackToID(t *testing.T) {
	assert.Equal(t, "Display", resolvedCapabilityName(fakeDescriptor{id: "cap.id", name: " Display "}))
	assert.Equal(t, "cap.id", resolvedCapabilityName(fakeDescriptor{id: "cap.id", name: "   "}))
	assert.Equal(t, "", resolvedCapabilityName("not a descriptor"))
	assert.Equal(t, "", resolvedCapabilityName(nil))
}

func TestSelectorMatchesDescriptorAllCriteria(t *testing.T) {
	desc := fakeDescriptor{
		id:                          "cap.alpha",
		name:                        "Alpha",
		kind:                        "tool",
		runtimeFamily:               "relurpic",
		tags:                        []string{"edit", "fast"},
		trustClass:                  "builtin-trusted",
		riskClasses:                 []risk.RiskClass{risk.RiskClassExecute, risk.RiskClassNetwork},
		effectClasses:               []classification.EffectClass{classification.EffectClassProcessSpawn},
		sourceScope:                 "builtin",
		sourceProviderID:            "provider-1",
		sourceSessionID:             "sess-1",
		coordinationRole:            "executor",
		coordinationTaskTypes:       []string{"refactor", "review"},
		coordinationExecutionModes:  []string{"sync", "session-backed"},
		coordinationLongRunning:     1,
		coordinationDirectInsertion: 2,
	}

	positive := []ResolvedCapabilitySelector{
		{ID: " CAP.ALPHA "},
		{Name: "alpha"},
		{Kind: "tool"},
		{RuntimeFamilies: []string{"provider", "relurpic"}},
		{Tags: []string{"edit", "fast"}},
		{SourceScopes: []string{"workspace", "builtin"}},
		{TrustClasses: []string{"workspace-trusted", "builtin-trusted"}},
		{RiskClasses: []string{"read-only", "execute"}},
		{EffectClasses: []string{"network-egress", "process-spawn"}},
		{CoordinationRoles: []string{"planner", "executor"}},
		{CoordinationTaskTypes: []string{"refactor"}},
		{CoordinationExecutionModes: []string{"sync"}},
		{CoordinationLongRunning: 1},
		{CoordinationDirectInsertion: 2},
		{}, // empty selector matches everything
	}
	for _, selector := range positive {
		assert.Truef(t, selectorMatchesDescriptor(selector, desc),
			"selector %+v should match descriptor", selector)
	}

	negative := []ResolvedCapabilitySelector{
		{ID: "cap.other"},
		{Name: "Beta"},
		{Kind: "prompt"},
		{RuntimeFamilies: []string{"provider"}},
		{Tags: []string{"edit", "slow"}},
		{ExcludeTags: []string{"fast"}},
		{SourceScopes: []string{"workspace"}},
		{TrustClasses: []string{"remote-declared"}},
		{RiskClasses: []string{"destructive"}},
		{EffectClasses: []string{"credential-use"}},
		{CoordinationRoles: []string{"planner"}},
		{CoordinationTaskTypes: []string{"migration"}},
		{CoordinationExecutionModes: []string{"background-agent"}},
		{CoordinationLongRunning: 2},
		{CoordinationDirectInsertion: 1},
	}
	for _, selector := range negative {
		assert.Falsef(t, selectorMatchesDescriptor(selector, desc),
			"selector %+v should not match descriptor", selector)
	}

	assert.False(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{}, "not a descriptor"),
		"non-descriptor candidates never match")
}

func TestSelectorLongRunningAndDirectInsertionZeroMeansUnset(t *testing.T) {
	desc := fakeDescriptor{id: "cap.alpha", name: "Alpha", coordinationLongRunning: 2, coordinationDirectInsertion: 1}
	assert.True(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{}, desc))
	assert.True(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{CoordinationLongRunning: 2}, desc))
	assert.True(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{CoordinationDirectInsertion: 1}, desc))
	assert.False(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{CoordinationLongRunning: 1}, desc))
	assert.False(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{CoordinationDirectInsertion: 2}, desc))
}

func TestSelectorIDAloneMatchesEvenWhenNameEmpty(t *testing.T) {
	desc := fakeDescriptor{id: "cap.alpha", name: ""}
	assert.True(t, selectorMatchesDescriptor(ResolvedCapabilitySelector{ID: "cap.alpha"}, desc),
		"descriptors without a display name are matched by ID")
}

func TestMergeResolvedNamesTrimsDeduplicatesAndSkipsEmpty(t *testing.T) {
	merged := mergeResolvedNames([]string{" a ", "b"}, []string{"a", " ", "c", ""})
	assert.Equal(t, []string{"a", "b", "c"}, merged)
	assert.Empty(t, mergeResolvedNames(nil, nil))
}

func TestCloneSeverityWeights(t *testing.T) {
	assert.Nil(t, cloneSeverityWeights(nil))
	source := map[string]float64{"high": 1.5}
	cloned := cloneSeverityWeights(source)
	require.Equal(t, source, cloned)
	cloned["high"] = 99
	assert.Equal(t, 1.5, source["high"], "clone must be independent of the source map")
}

func TestRegistryCapabilitiesSortedNilRegistry(t *testing.T) {
	assert.Nil(t, registryCapabilitiesSorted(nil))
}
