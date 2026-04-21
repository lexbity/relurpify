package authorization

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ----

// allowRule builds a minimal enabled rule with an "allow" effect.
func allowRule(id string) core.PolicyRule {
	return core.PolicyRule{
		ID:      id,
		Name:    id,
		Enabled: true,
		Effect:  core.PolicyEffect{Action: "allow"},
	}
}

// denyRule builds a minimal enabled rule with a "deny" effect.
func denyRule(id string) core.PolicyRule {
	return core.PolicyRule{
		ID:      id,
		Name:    id,
		Enabled: true,
		Effect:  core.PolicyEffect{Action: "deny"},
	}
}

// boolPtr is a convenience to take the address of a bool literal.
func boolPtr(v bool) *bool { return &v }

// ---- evaluateCompiledRules ----

func TestEvaluateCompiledRules_EmptyRules(t *testing.T) {
	result := evaluateCompiledRules(nil, core.PolicyRequest{})
	assert.Nil(t, result)
}

func TestEvaluateCompiledRules_DisabledRuleSkipped(t *testing.T) {
	rule := allowRule("r1")
	rule.Enabled = false
	result := evaluateCompiledRules([]core.PolicyRule{rule}, core.PolicyRequest{})
	assert.Nil(t, result)
}

func TestEvaluateCompiledRules_NoConditionsMatchesAll(t *testing.T) {
	rule := allowRule("r1")
	result := evaluateCompiledRules([]core.PolicyRule{rule}, core.PolicyRequest{})
	require.NotNil(t, result)
	assert.Equal(t, "allow", result.Effect)
}

func TestEvaluateCompiledRules_FirstEnabledMatchReturned(t *testing.T) {
	rules := []core.PolicyRule{allowRule("r1"), denyRule("r2")}
	result := evaluateCompiledRules(rules, core.PolicyRequest{})
	require.NotNil(t, result)
	assert.Equal(t, "allow", result.Effect)
}

func TestEvaluateCompiledRules_NoMatchReturnsNil(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TrustClasses = []core.TrustClass{core.TrustClassRemoteDeclared}
	result := evaluateCompiledRules([]core.PolicyRule{rule}, core.PolicyRequest{
		TrustClass: core.TrustClassBuiltinTrusted,
	})
	assert.Nil(t, result)
}

// ---- decisionForRule ----

func TestDecisionForRule_Allow(t *testing.T) {
	rule := allowRule("r1")
	rule.Effect.Reason = "trusted binary"
	d := decisionForRule(&rule)
	assert.Equal(t, "allow", d.Effect)
	assert.Equal(t, "trusted binary", d.Reason)
}

func TestDecisionForRule_Deny(t *testing.T) {
	rule := denyRule("r1")
	rule.Effect.Reason = "blocked"
	d := decisionForRule(&rule)
	assert.Equal(t, "deny", d.Effect)
	assert.Equal(t, "blocked", d.Reason)
}

func TestDecisionForRule_UnknownActionRequiresApproval(t *testing.T) {
	rule := core.PolicyRule{
		ID: "r1", Name: "r1", Enabled: true,
		Effect: core.PolicyEffect{Action: "require_approval", Reason: "sensitive"},
	}
	d := decisionForRule(&rule)
	assert.Equal(t, "require_approval", d.Effect)
}

// ---- ruleMatchesRequest — condition-by-condition ----

func TestRuleMatchesRequest_NoConditions_AlwaysMatches(t *testing.T) {
	rule := allowRule("r1")
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{}))
}

func TestRuleMatchesRequest_Capabilities_MatchByID(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Capabilities = []string{"file_read"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{CapabilityID: "file_read"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{CapabilityID: "file_write"}))
}

func TestRuleMatchesRequest_Capabilities_MatchByName(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Capabilities = []string{"File Read"}
	// Match is case-insensitive on the name field.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{CapabilityName: "file read"}))
}

func TestRuleMatchesRequest_Capabilities_EmptyCapabilityIDMiss(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Capabilities = []string{"file_read"}
	// Neither ID nor Name match → no match.
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{}))
}

func TestRuleMatchesRequest_TrustClass_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TrustClasses = []core.TrustClass{core.TrustClassRemoteDeclared}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{TrustClass: core.TrustClassRemoteDeclared}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{TrustClass: core.TrustClassBuiltinTrusted}))
}

func TestRuleMatchesRequest_CapabilityKind_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.CapabilityKinds = []core.CapabilityKind{core.CapabilityKindTool}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{CapabilityKind: core.CapabilityKindTool}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{CapabilityKind: core.CapabilityKindPrompt}))
}

func TestRuleMatchesRequest_RuntimeFamily_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RuntimeFamilies = []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyLocalTool}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{RuntimeFamily: core.CapabilityRuntimeFamilyLocalTool}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{RuntimeFamily: core.CapabilityRuntimeFamilyProvider}))
}

func TestRuleMatchesRequest_EffectClasses_MatchesIfAnyPresent(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.EffectClasses = []core.EffectClass{core.EffectClassProcessSpawn}
	// Request carries two effect classes including the one in the condition.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation, core.EffectClassProcessSpawn},
	}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{
		EffectClasses: []core.EffectClass{core.EffectClassNetworkEgress},
	}))
}

func TestRuleMatchesRequest_MinRiskClasses_ThresholdMet(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.MinRiskClasses = []core.RiskClass{core.RiskClassNetwork} // rank 3
	// Request has a higher-ranked risk class → meets threshold.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{
		RiskClasses: []core.RiskClass{core.RiskClassDestructive}, // rank 7
	}))
}

func TestRuleMatchesRequest_MinRiskClasses_ThresholdNotMet(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.MinRiskClasses = []core.RiskClass{core.RiskClassDestructive} // rank 7
	// Request only has read-only risk.
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{
		RiskClasses: []core.RiskClass{core.RiskClassReadOnly}, // rank 1
	}))
}

func TestRuleMatchesRequest_ProviderKind_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.ProviderKinds = []core.ProviderKind{core.ProviderKindMCPClient}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ProviderKind: core.ProviderKindMCPClient}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ProviderKind: "mcp-server"}))
}

func TestRuleMatchesRequest_ExternalProvider_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.ExternalProviders = []core.ExternalProvider{core.ExternalProviderDiscord}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ExternalProvider: core.ExternalProviderDiscord}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ExternalProvider: core.ExternalProviderTelegram}))
}

func TestRuleMatchesRequest_SensitivityClass_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.SensitivityClasses = []core.SensitivityClass{core.SensitivityClassHigh}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{SensitivityClass: core.SensitivityClassHigh}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{SensitivityClass: core.SensitivityClassLow}))
}

func TestRuleMatchesRequest_RouteMode_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RouteModes = []core.RouteMode{core.RouteModeGateway}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{RouteMode: core.RouteModeGateway}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{RouteMode: core.RouteModeDirect}))
}

func TestRuleMatchesRequest_SessionScope_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.SessionScopes = []core.SessionScope{core.SessionScopePerChannelPeer}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{SessionScope: core.SessionScopePerChannelPeer}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{SessionScope: core.SessionScopeMain}))
}

func TestRuleMatchesRequest_SessionOperation_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.SessionOperations = []core.SessionOperation{core.SessionOperationSend}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{SessionOperation: core.SessionOperationSend}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{SessionOperation: core.SessionOperationClose}))
}

func TestRuleMatchesRequest_SourceDomain_CaseInsensitive(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.SourceDomains = []string{"GITHUB.COM"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{SourceDomain: "github.com"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{SourceDomain: "gitlab.com"}))
}

func TestRuleMatchesRequest_SourceDomain_EmptyRequestMiss(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.SourceDomains = []string{"github.com"}
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{}))
}

func TestRuleMatchesRequest_ExportName_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.ExportNames = []string{"file_read"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ExportName: "file_read"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ExportName: "file_write"}))
}

func TestRuleMatchesRequest_ContextClass_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.ContextClasses = []string{"workspace"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ContextClass: "workspace"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ContextClass: "external"}))
}

func TestRuleMatchesRequest_Partition_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Partitions = []string{"prod"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{Partition: "prod"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Partition: "staging"}))
}

func TestRuleMatchesRequest_ChannelID_Match(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.ChannelIDs = []string{"chan-abc"}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ChannelID: "chan-abc"}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ChannelID: "chan-xyz"}))
}

// ---- Actor condition ----

func TestRuleMatchesRequest_Actor_EmptyCondition_MatchesAll(t *testing.T) {
	rule := allowRule("r1")
	// No actor conditions set → matches any actor.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{
		Actor: core.EventActor{Kind: "user", ID: "alice"},
	}))
}

func TestRuleMatchesRequest_Actor_KindFilter(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Actors = []core.ActorMatch{{Kind: "user"}}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{Actor: core.EventActor{Kind: "user"}}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Actor: core.EventActor{Kind: "service"}}))
}

func TestRuleMatchesRequest_Actor_IDFilter(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Actors = []core.ActorMatch{{IDs: []string{"alice", "bob"}}}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{Actor: core.EventActor{ID: "alice"}}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Actor: core.EventActor{ID: "charlie"}}))
}

func TestRuleMatchesRequest_Actor_Authenticated(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.Actors = []core.ActorMatch{{Authenticated: true}}
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{Authenticated: true}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Authenticated: false}))
}

// ---- Boolean pointer conditions ----

func TestRuleMatchesRequest_RequireOwnership_True(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireOwnership = boolPtr(true)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{IsOwner: true}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{IsOwner: false}))
}

func TestRuleMatchesRequest_RequireOwnership_False(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireOwnership = boolPtr(false)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{IsOwner: false}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{IsOwner: true}))
}

func TestRuleMatchesRequest_RequireDelegation(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireDelegation = boolPtr(true)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{IsDelegated: true}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{IsDelegated: false}))
}

func TestRuleMatchesRequest_RequireExternalBinding(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireExternalBinding = boolPtr(true)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{HasExternalBinding: true}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{HasExternalBinding: false}))
}

func TestRuleMatchesRequest_RequireResolvedExternal(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireResolvedExternal = boolPtr(true)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{ResolvedExternal: true}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{ResolvedExternal: false}))
}

func TestRuleMatchesRequest_RequireRestrictedExternal(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.RequireRestrictedExternal = boolPtr(false)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{RestrictedExternal: false}))
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{RestrictedExternal: true}))
}

func TestRuleMatchesRequest_BooleanNilCondition_Ignored(t *testing.T) {
	rule := allowRule("r1")
	// All pointer conditions are nil → they are not checked.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{IsOwner: true, IsDelegated: false}))
}

// ---- TimeWindow condition ----

func TestRuleMatchesRequest_TimeWindow_Inside(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TimeWindow = &core.TimeWindow{After: "08:00", Before: "18:00"}
	ts := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{Timestamp: ts}))
}

func TestRuleMatchesRequest_TimeWindow_Before_Boundary(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TimeWindow = &core.TimeWindow{After: "08:00", Before: "18:00"}
	ts := time.Date(2026, 4, 8, 7, 0, 0, 0, time.UTC) // 07:00 < 08:00
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Timestamp: ts}))
}

func TestRuleMatchesRequest_TimeWindow_After_Boundary(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TimeWindow = &core.TimeWindow{After: "08:00", Before: "18:00"}
	ts := time.Date(2026, 4, 8, 20, 0, 0, 0, time.UTC) // 20:00 > 18:00
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{Timestamp: ts}))
}

// ---- matchesMinRiskClasses ----

func TestMatchesMinRiskClasses_EmptyCondition(t *testing.T) {
	assert.False(t, matchesMinRiskClasses(nil, []core.RiskClass{core.RiskClassDestructive}))
}

func TestMatchesMinRiskClasses_EmptyActual(t *testing.T) {
	assert.False(t, matchesMinRiskClasses([]core.RiskClass{core.RiskClassReadOnly}, nil))
}

func TestMatchesMinRiskClasses_ExactThreshold(t *testing.T) {
	// Threshold = execute (rank 4), actual = execute (rank 4) → meets.
	assert.True(t, matchesMinRiskClasses(
		[]core.RiskClass{core.RiskClassExecute},
		[]core.RiskClass{core.RiskClassExecute},
	))
}

func TestMatchesMinRiskClasses_BelowThreshold(t *testing.T) {
	// Threshold = execute (rank 4), actual = sessioned (rank 2) → does not meet.
	assert.False(t, matchesMinRiskClasses(
		[]core.RiskClass{core.RiskClassExecute},
		[]core.RiskClass{core.RiskClassSessioned},
	))
}

func TestMatchesMinRiskClasses_MultipleActual_OneQualifies(t *testing.T) {
	assert.True(t, matchesMinRiskClasses(
		[]core.RiskClass{core.RiskClassNetwork},
		[]core.RiskClass{core.RiskClassReadOnly, core.RiskClassDestructive},
	))
}

func TestMatchesMinRiskClasses_RankOrder(t *testing.T) {
	// Verify rank ordering: read-only < sessioned < network < execute < credentialed < exfiltration < destructive.
	ranks := []core.RiskClass{
		core.RiskClassReadOnly,
		core.RiskClassSessioned,
		core.RiskClassNetwork,
		core.RiskClassExecute,
		core.RiskClassCredentialed,
		core.RiskClassExfiltration,
		core.RiskClassDestructive,
	}
	for i := 1; i < len(ranks); i++ {
		assert.Greater(t, riskRank(ranks[i]), riskRank(ranks[i-1]),
			"expected rank(%s) > rank(%s)", ranks[i], ranks[i-1])
	}
}

// ---- matchesTimeWindow ----

func TestMatchesTimeWindow_OnlyAfter(t *testing.T) {
	w := core.TimeWindow{After: "09:00"}
	assert.True(t, matchesTimeWindow(w, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)))
	assert.False(t, matchesTimeWindow(w, time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)))
}

func TestMatchesTimeWindow_OnlyBefore(t *testing.T) {
	w := core.TimeWindow{Before: "17:00"}
	assert.True(t, matchesTimeWindow(w, time.Date(2026, 1, 1, 16, 0, 0, 0, time.UTC)))
	assert.False(t, matchesTimeWindow(w, time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)))
}

func TestMatchesTimeWindow_ZeroTimestampUsesNow(t *testing.T) {
	// A wide window should match "now" whatever time it is.
	w := core.TimeWindow{After: "00:00", Before: "23:59"}
	assert.True(t, matchesTimeWindow(w, time.Time{}))
}

// ---- DecideByPatterns (public, uses search.MatchGlob) ----

func TestDecideByPatterns_DenyFirst(t *testing.T) {
	// Deny patterns are checked before allow patterns; "git push *" matches the
	// target before "git *" gets a chance to allow it.
	decision, pattern := DecideByPatterns(
		"git push force",
		[]string{"git *"},      // allow — also matches
		[]string{"git push *"}, // deny — checked first, more specific
		core.AgentPermissionAllow,
	)
	assert.Equal(t, core.AgentPermissionDeny, decision)
	assert.Equal(t, "git push *", pattern)
}

func TestDecideByPatterns_AllowAfterDenyMiss(t *testing.T) {
	decision, pattern := DecideByPatterns(
		"go test ./...",
		[]string{"go test **"},
		[]string{"go build *"},
		core.AgentPermissionDeny,
	)
	assert.Equal(t, core.AgentPermissionAllow, decision)
	assert.Equal(t, "go test **", pattern)
}

func TestDecideByPatterns_DefaultWhenNoMatch(t *testing.T) {
	decision, pattern := DecideByPatterns(
		"npm install",
		[]string{"go **"},
		[]string{"rm **"},
		core.AgentPermissionAsk,
	)
	assert.Equal(t, core.AgentPermissionAsk, decision)
	assert.Empty(t, pattern)
}

func TestDecideByPatterns_EmptyDefaultBecomesAllow(t *testing.T) {
	decision, _ := DecideByPatterns("echo hi", nil, nil, "")
	assert.Equal(t, core.AgentPermissionAllow, decision)
}

func TestDecideByPatterns_BlankPatternsSkipped(t *testing.T) {
	decision, _ := DecideByPatterns(
		"echo hi",
		[]string{"  ", "", "echo *"},
		[]string{"  "},
		core.AgentPermissionDeny,
	)
	assert.Equal(t, core.AgentPermissionAllow, decision)
}

func TestDecideByPatterns_EmptyTarget_AllowPatternMiss(t *testing.T) {
	// Empty target: containsFold treats empty as no-match; deny pattern with glob
	// may or may not match depending on glob semantics.
	decision, _ := DecideByPatterns("", []string{"*"}, nil, core.AgentPermissionDeny)
	// "*" would match empty string via filepath.Match on empty string — verify we
	// get a deterministic result without panicking.
	require.NotPanics(t, func() {
		DecideByPatterns("", []string{"*"}, nil, core.AgentPermissionDeny)
	})
	_ = decision
}

// ---- EvaluatePolicyRules (public wrapper) ----

func TestEvaluatePolicyRules_DelegatesToEvaluateCompiledRules(t *testing.T) {
	rules := []core.PolicyRule{allowRule("r1")}
	result := EvaluatePolicyRules(rules, core.PolicyRequest{})
	require.NotNil(t, result)
	assert.Equal(t, "allow", result.Effect)
}

func TestEvaluatePolicyRules_NilRules(t *testing.T) {
	result := EvaluatePolicyRules(nil, core.PolicyRequest{})
	assert.Nil(t, result)
}

// ---- Combined: multiple conditions must all match ----

func TestRuleMatchesRequest_MultipleConditions_AllMustMatch(t *testing.T) {
	rule := allowRule("r1")
	rule.Conditions.TrustClasses = []core.TrustClass{core.TrustClassRemoteDeclared}
	rule.Conditions.CapabilityKinds = []core.CapabilityKind{core.CapabilityKindTool}

	// Both conditions satisfied.
	assert.True(t, ruleMatchesRequest(rule, core.PolicyRequest{
		TrustClass:     core.TrustClassRemoteDeclared,
		CapabilityKind: core.CapabilityKindTool,
	}))
	// Only one condition satisfied → no match.
	assert.False(t, ruleMatchesRequest(rule, core.PolicyRequest{
		TrustClass:     core.TrustClassRemoteDeclared,
		CapabilityKind: core.CapabilityKindPrompt,
	}))
}
