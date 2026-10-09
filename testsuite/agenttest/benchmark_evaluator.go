package agenttest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BenchmarkEvaluation is the outcome of evaluating a case's benchmark
// assertions. Observations are advisory measurements; Failures capture
// threshold and negative-assertion violations that MUST fail the case.
type BenchmarkEvaluation struct {
	Observations []BenchmarkObservation
	Failures     []string
}

// EvaluateBenchmark checks the benchmark block of a case against the
// telemetry-derived report and the paired tool transcript. A nil spec produces
// an empty evaluation.
//
// Presence mismatches (tools_expected) and derived measurements (success rate,
// recovery, sequence ordering, dependencies, llm call count) are recorded as
// observations only. Thresholds (max_tool_calls_hint, max_total_tool_time_hint_ms,
// tool_call_latency_ms, token_budget) and negative assertions
// (tools_not_expected) fail the case when violated (FR-10).
func EvaluateBenchmark(report *CaseReport, spec *BenchmarkSpec, transcript *ToolTranscriptArtifact) BenchmarkEvaluation {
	if spec == nil {
		return BenchmarkEvaluation{}
	}
	var eval BenchmarkEvaluation
	observed := map[string]int{}
	totalCalls := 0
	if report != nil {
		for tool, count := range report.ToolCalls {
			observed[tool] = count
			totalCalls += count
		}
	}

	for _, tool := range spec.ToolsExpected {
		count := observed[tool]
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tools_expected",
			Expected: tool,
			Actual:   strconv.Itoa(count),
			Matched:  count > 0,
		})
	}

	for _, tool := range spec.ToolsNotExpected {
		count := observed[tool]
		matched := count == 0
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tools_not_expected",
			Expected: tool,
			Actual:   strconv.Itoa(count),
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("tool %q was called %d time(s) but is not expected", tool, count))
		}
	}

	if len(spec.ToolSequenceExpected) > 0 {
		actual := toolSequence(transcript)
		orderingFailures := ValidateToolOrdering(transcript, spec.ToolSequenceExpected, false)
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tool_sequence_expected",
			Expected: strings.Join(spec.ToolSequenceExpected, ","),
			Actual:   strings.Join(actual, ","),
			Matched:  len(orderingFailures) == 0,
			Note:     strings.Join(orderingFailures, "; "),
		})
	}

	if len(spec.ToolSuccessRate) > 0 {
		stats := toolSuccessStats(transcript)
		for tool, expected := range spec.ToolSuccessRate {
			stat := stats[tool]
			rate := 0.0
			if stat.total() > 0 {
				rate = float64(stat.success) / float64(stat.total()) * 100
			}
			eval.Observations = append(eval.Observations, BenchmarkObservation{
				Category: "tool_usage",
				Field:    "tool_success_rate." + tool,
				Expected: strconv.Itoa(expected) + "%",
				Actual:   fmt.Sprintf("%.0f%%", rate),
				Matched:  rate >= float64(expected),
			})
		}
	}

	if len(spec.ToolCallLatencyMs) > 0 && report != nil {
		for tool, limit := range spec.ToolCallLatencyMs {
			maxMs := report.ToolLatencies[tool].MaxMs
			matched := maxMs <= int64(limit)
			eval.Observations = append(eval.Observations, BenchmarkObservation{
				Category: "performance",
				Field:    "tool_call_latency_ms." + tool,
				Expected: "<= " + strconv.Itoa(limit) + "ms",
				Actual:   strconv.FormatInt(maxMs, 10) + "ms",
				Matched:  matched,
			})
			if !matched {
				eval.Failures = append(eval.Failures, fmt.Sprintf("tool %q latency %dms exceeds %dms", tool, maxMs, limit))
			}
		}
	}

	if len(spec.ToolDependencies) > 0 {
		validator := NewDependencyValidator(spec.ToolDependencies)
		for _, failure := range validator.Validate(transcript) {
			eval.Observations = append(eval.Observations, BenchmarkObservation{
				Category: "tool_usage",
				Field:    "tool_dependencies",
				Expected: "satisfied",
				Actual:   "violated",
				Matched:  false,
				Note:     failure,
			})
		}
	}

	if spec.ToolRecoveryObserved {
		recovered := transcriptShowsRecovery(transcript)
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tool_recovery_observed",
			Expected: "true",
			Actual:   strconv.FormatBool(recovered),
			Matched:  recovered,
		})
	}

	if spec.LLMCallsExpected > 0 && report != nil {
		actual := report.TokenUsage.LLMCalls
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "token_usage",
			Field:    "llm_calls_expected",
			Expected: strconv.Itoa(spec.LLMCallsExpected),
			Actual:   strconv.Itoa(actual),
			Matched:  actual == spec.LLMCallsExpected,
		})
	}

	if spec.MaxToolCallsHint > 0 {
		matched := totalCalls <= spec.MaxToolCallsHint
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "max_tool_calls_hint",
			Expected: "<= " + strconv.Itoa(spec.MaxToolCallsHint),
			Actual:   strconv.Itoa(totalCalls),
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("tool calls %d exceed limit %d", totalCalls, spec.MaxToolCallsHint))
		}
	}

	if spec.MaxTotalToolTimeHintMs > 0 && report != nil {
		actual := report.TotalToolTimeMs
		matched := actual <= int64(spec.MaxTotalToolTimeHintMs)
		eval.Observations = append(eval.Observations, BenchmarkObservation{
			Category: "performance",
			Field:    "max_total_tool_time_hint_ms",
			Expected: "<= " + strconv.Itoa(spec.MaxTotalToolTimeHintMs) + "ms",
			Actual:   strconv.FormatInt(actual, 10) + "ms",
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("total tool time %dms exceeds %dms", actual, spec.MaxTotalToolTimeHintMs))
		}
	}

	if budget := spec.TokenBudget; budget != nil && report != nil {
		eval.Observations = append(eval.Observations, tokenBudgetObservation("prompt", budget.MaxPrompt, report.TokenUsage.PromptTokens, &eval))
		eval.Observations = append(eval.Observations, tokenBudgetObservation("completion", budget.MaxCompletion, report.TokenUsage.CompletionTokens, &eval))
		eval.Observations = append(eval.Observations, tokenBudgetObservation("total", budget.MaxTotal, report.TokenUsage.TotalTokens, &eval))
	}

	if spec.Selection != nil {
		eval.Observations = append(eval.Observations, evaluateSelectionAssertions(spec.Selection, report, &eval)...)
	}

	sort.Strings(eval.Failures)
	return eval
}

// evaluateSelectionAssertions checks the Benchmark-axis `selection:` assertion
// against the recorded route-selection outcome (FR-22). A declared dimension
// that mismatches the euclo.route.selected event is a behavioral regression:
// route selection is deterministic, recorded provenance, so the mismatch fails
// the case rather than being a soft observation.
func evaluateSelectionAssertions(sel *SelectionAssertion, report *CaseReport, eval *BenchmarkEvaluation) []BenchmarkObservation {
	if sel == nil {
		return nil
	}
	actual := RouteSelectionReport{}
	if report != nil {
		actual = report.RouteSelection
	}
	var observations []BenchmarkObservation

	if expected := strings.TrimSpace(sel.ChosenRoute); expected != "" {
		matched := actual.ChosenRoute == expected
		observations = append(observations, BenchmarkObservation{
			Category: "euclo_routing",
			Field:    "selection.chosen_route",
			Expected: expected,
			Actual:   actual.ChosenRoute,
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("selection chose route %q, expected %q", actual.ChosenRoute, expected))
		}
	}

	if expected := strings.TrimSpace(sel.DecidedBy); expected != "" {
		matched := actual.DecidedBy == expected
		observations = append(observations, BenchmarkObservation{
			Category: "euclo_routing",
			Field:    "selection.decided_by",
			Expected: expected,
			Actual:   actual.DecidedBy,
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("selection decided by %q, expected %q", actual.DecidedBy, expected))
		}
	}

	if sel.FallbackTaken != nil {
		matched := actual.FallbackTaken == *sel.FallbackTaken
		observations = append(observations, BenchmarkObservation{
			Category: "euclo_routing",
			Field:    "selection.fallback_taken",
			Expected: strconv.FormatBool(*sel.FallbackTaken),
			Actual:   strconv.FormatBool(actual.FallbackTaken),
			Matched:  matched,
		})
		if !matched {
			eval.Failures = append(eval.Failures, fmt.Sprintf("selection fallback_taken=%t, expected %t", actual.FallbackTaken, *sel.FallbackTaken))
		}
	}

	return observations
}

func tokenBudgetObservation(metric string, max, actual int, eval *BenchmarkEvaluation) BenchmarkObservation {
	obs := BenchmarkObservation{
		Category: "token_usage",
		Field:    "token_budget." + metric,
		Expected: "<= " + strconv.Itoa(max),
		Actual:   strconv.Itoa(actual),
	}
	if max <= 0 {
		obs.Matched = true
		return obs
	}
	obs.Matched = actual <= max
	if !obs.Matched {
		eval.Failures = append(eval.Failures, fmt.Sprintf("%s tokens %d exceed budget %d", metric, actual, max))
	}
	return obs
}

func toolSequence(transcript *ToolTranscriptArtifact) []string {
	if transcript == nil {
		return nil
	}
	sequence := make([]string, 0, len(transcript.Entries))
	for _, entry := range transcript.Entries {
		if tool := strings.TrimSpace(entry.Tool); tool != "" {
			sequence = append(sequence, tool)
		}
	}
	return sequence
}

type toolStat struct {
	success int
	fail    int
}

func (s toolStat) total() int { return s.success + s.fail }

func toolSuccessStats(transcript *ToolTranscriptArtifact) map[string]toolStat {
	stats := map[string]toolStat{}
	if transcript == nil {
		return stats
	}
	for _, entry := range transcript.Entries {
		tool := strings.TrimSpace(entry.Tool)
		if tool == "" {
			continue
		}
		stat := stats[tool]
		if entry.Success {
			stat.success++
		} else {
			stat.fail++
		}
		stats[tool] = stat
	}
	return stats
}

// transcriptShowsRecovery reports whether any tool failed and later succeeded.
func transcriptShowsRecovery(transcript *ToolTranscriptArtifact) bool {
	if transcript == nil {
		return false
	}
	failed := map[string]bool{}
	for _, entry := range transcript.Entries {
		tool := strings.TrimSpace(entry.Tool)
		if tool == "" {
			continue
		}
		if !entry.Success {
			failed[tool] = true
			continue
		}
		if failed[tool] {
			return true
		}
	}
	return false
}
