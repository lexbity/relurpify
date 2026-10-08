package agenttest

import "fmt"

// OSB assertion coverage tables (Phase 7).
//
// These maps are the single source of truth for which assertion fields the
// harness actually enforces, which it recognizes as runtime-enforced, and which
// it treats as advisory. The committed suite catalog is validated against them
// by osb_coverage_test.go so no suite can silently pass an unevaluated
// assertion — the "no silent pass" gate.
//
// Adding a field to SecuritySpec or BenchmarkSpec without updating these tables
// makes the coverage gate fail with a precise message instead of letting the
// assertion pass unverified.

// EvaluatedSecurityAssertions are SecuritySpec fields the OSB security
// evaluator re-asserts from telemetry-derived case data (FR-9).
var EvaluatedSecurityAssertions = map[string]bool{
	"tools_must_not_call":     true,
	"expected_violations":     true,
	"no_writes_outside_scope": true,
	"no_reads_outside_scope":  true,
	"mutation_enforced":       true,
}

// RuntimeEnforcedSecurityAssertions are SecuritySpec fields enforced at the
// runtime boundary — the governance command policy denies out-of-manifest
// executables (no_exec_outside_manifest) and the sandbox runs with
// --network none (no_network_outside_manifest). There is no harness-telemetry
// stream that can re-assert them, so the harness recognises them as
// runtime-enforced rather than claiming telemetry-based evaluation or ignoring
// them.
var RuntimeEnforcedSecurityAssertions = map[string]bool{
	"no_exec_outside_manifest":    true,
	"no_network_outside_manifest": true,
}

// EvaluatedBenchmarkAssertions are BenchmarkSpec fields the OSB benchmark
// evaluator records and enforces (thresholds and negative assertions are hard,
// presence and measurement mismatches are advisory) (FR-10).
var EvaluatedBenchmarkAssertions = map[string]bool{
	"tools_expected":              true,
	"tools_not_expected":          true,
	"tool_sequence_expected":      true,
	"tool_success_rate":           true,
	"tool_call_latency_ms":        true,
	"tool_dependencies":           true,
	"tool_recovery_observed":      true,
	"llm_calls_expected":          true,
	"max_tool_calls_hint":         true,
	"max_total_tool_time_hint_ms": true,
	"token_budget":                true,
}

// AdvisoryBenchmarkFields are BenchmarkSpec fields that are parsed but produce
// no pass/fail signal: euclo routing hints (extensions) and stability hints.
var AdvisoryBenchmarkFields = map[string]bool{
	"extensions":               true,
	"determinism_score_hint":   true,
	"llm_response_stable_hint": true,
}

// ValidateOSBCaseCoverage returns a list of errors for every OSB assertion a
// case declares that the harness neither evaluates nor recognises as
// runtime-enforced or advisory. An empty result means the case's OSB block is
// fully covered.
func ValidateOSBCaseCoverage(c CaseSpec) []string {
	var errs []string
	if c.Expect.Security != nil {
		sec := *c.Expect.Security
		assertField("security", "tools_must_not_call", len(sec.ToolsMustNotCall) > 0, &errs)
		assertField("security", "expected_violations", len(sec.ExpectedViolations) > 0, &errs)
		assertField("security", "no_writes_outside_scope", sec.NoWritesOutsideScope, &errs)
		assertField("security", "no_reads_outside_scope", sec.NoReadsOutsideScope, &errs)
		assertField("security", "mutation_enforced", sec.MutationEnforced, &errs)
		assertField("security", "no_exec_outside_manifest", sec.NoExecOutsideManifest, &errs)
		assertField("security", "no_network_outside_manifest", sec.NoNetworkOutsideManifest, &errs)
	}
	if c.Expect.Benchmark != nil {
		ben := *c.Expect.Benchmark
		assertField("benchmark", "tools_expected", len(ben.ToolsExpected) > 0, &errs)
		assertField("benchmark", "tools_not_expected", len(ben.ToolsNotExpected) > 0, &errs)
		assertField("benchmark", "tool_sequence_expected", len(ben.ToolSequenceExpected) > 0, &errs)
		assertField("benchmark", "tool_success_rate", len(ben.ToolSuccessRate) > 0, &errs)
		assertField("benchmark", "tool_call_latency_ms", len(ben.ToolCallLatencyMs) > 0, &errs)
		assertField("benchmark", "tool_dependencies", len(ben.ToolDependencies) > 0, &errs)
		assertField("benchmark", "tool_recovery_observed", ben.ToolRecoveryObserved, &errs)
		assertField("benchmark", "llm_calls_expected", ben.LLMCallsExpected > 0, &errs)
		assertField("benchmark", "max_tool_calls_hint", ben.MaxToolCallsHint > 0, &errs)
		assertField("benchmark", "max_total_tool_time_hint_ms", ben.MaxTotalToolTimeHintMs > 0, &errs)
		assertField("benchmark", "token_budget", ben.TokenBudget != nil, &errs)
		assertField("benchmark", "llm_response_stable_hint", ben.LLMResponseStableHint, &errs)
		assertField("benchmark", "determinism_score_hint", ben.DeterminismScoreHint != "", &errs)
		// ben.Extensions is advisory by construction (euclo routing hints); it
		// produces no pass/fail signal, so there is nothing to assert.
	}
	return errs
}

func assertField(tier, field string, declared bool, errs *[]string) {
	if !declared {
		return
	}
	if tier == "security" {
		if EvaluatedSecurityAssertions[field] || RuntimeEnforcedSecurityAssertions[field] {
			return
		}
	} else {
		if EvaluatedBenchmarkAssertions[field] || AdvisoryBenchmarkFields[field] {
			return
		}
	}
	*errs = append(*errs, fmt.Sprintf(
		"unsupported %s assertion %q: the harness neither evaluates it nor recognises it as runtime-enforced or advisory",
		tier, field))
}
