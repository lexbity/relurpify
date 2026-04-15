package agenttest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// evaluateSuccessRateConstraint checks if a success rate meets a constraint like ">0.9" or "0.8"
func evaluateSuccessRateConstraint(rate float64, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return true
	}

	// Parse constraint like ">0.9", ">=0.8", "<0.5", "0.9" (implicit >=)
	var threshold float64
	var op string

	if strings.HasPrefix(constraint, ">=") {
		op = ">="
		fmt.Sscanf(constraint[2:], "%f", &threshold)
	} else if strings.HasPrefix(constraint, ">") {
		op = ">"
		fmt.Sscanf(constraint[1:], "%f", &threshold)
	} else if strings.HasPrefix(constraint, "<=") {
		op = "<="
		fmt.Sscanf(constraint[2:], "%f", &threshold)
	} else if strings.HasPrefix(constraint, "<") {
		op = "<"
		fmt.Sscanf(constraint[1:], "%f", &threshold)
	} else {
		// Default to >= for bare numbers
		op = ">="
		fmt.Sscanf(constraint, "%f", &threshold)
	}

	switch op {
	case ">=":
		return rate >= threshold
	case ">":
		return rate > threshold
	case "<=":
		return rate <= threshold
	case "<":
		return rate < threshold
	default:
		return true
	}
}

func toolCallsAppearInOrder(events []core.Event, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	next := 0
	for _, ev := range events {
		if ev.Type != core.EventToolCall {
			continue
		}
		name, _ := ev.Metadata["tool"].(string)
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(expected[next])) {
			next++
			if next == len(expected) {
				return true
			}
		}
	}
	return false
}

func countLLMCalls(events []core.Event) int {
	total := 0
	for _, ev := range events {
		if ev.Type == core.EventLLMResponse {
			total++
		}
	}
	return total
}

func contextSnapshotHasKey(snapshot *core.ContextSnapshot, key string) bool {
	if snapshot == nil || strings.TrimSpace(key) == "" {
		return false
	}
	if _, ok := snapshot.State[key]; ok {
		return true
	}
	if strings.HasPrefix(key, "state.") {
		key = strings.TrimPrefix(key, "state.")
		if _, ok := snapshot.State[key]; ok {
			return true
		}
	}
	return nestedMapHasPath(snapshot.State, key)
}

func nestedMapHasPath(root map[string]any, path string) bool {
	if len(root) == 0 || strings.TrimSpace(path) == "" {
		return false
	}
	current := any(root)
	for _, part := range strings.Split(path, ".") {
		typed, ok := current.(map[string]any)
		if !ok {
			return false
		}
		next, ok := typed[part]
		if !ok {
			return false
		}
		current = next
	}
	return true
}

func contextSnapshotKeyNotEmpty(snapshot *core.ContextSnapshot, key string) bool {
	if snapshot == nil || strings.TrimSpace(key) == "" {
		return false
	}
	value, ok := contextSnapshotValue(snapshot, key)
	if !ok {
		return false
	}
	return valueNotEmpty(value)
}

func contextSnapshotValue(snapshot *core.ContextSnapshot, key string) (any, bool) {
	if snapshot == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	if value, ok := snapshot.State[key]; ok {
		return value, true
	}
	if strings.HasPrefix(key, "state.") {
		key = strings.TrimPrefix(key, "state.")
		if value, ok := snapshot.State[key]; ok {
			return value, true
		}
	}
	return nestedMapValue(snapshot.State, key)
}

func nestedMapValue(root map[string]any, path string) (any, bool) {
	if len(root) == 0 || strings.TrimSpace(path) == "" {
		return nil, false
	}
	current := any(root)
	for _, part := range strings.Split(path, ".") {
		typed, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := typed[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func valueNotEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case bool:
		return typed
	default:
		return fmt.Sprint(value) != ""
	}
}

func evaluateArtifactChainExpectations(specs []ArtifactChainSpec, interactionRecords []any) []string {
	if len(specs) == 0 {
		return nil
	}
	records := collectInteractionPhaseRecords(interactionRecords)
	if len(records) == 0 {
		return []string{"euclo.artifact_chain: no interaction records available"}
	}

	var failures []string
	for _, spec := range specs {
		targetKind := normalizeArtifactKind(spec.Kind)
		if targetKind == "" {
			failures = append(failures, fmt.Sprintf("euclo.artifact_chain: invalid artifact kind %q", spec.Kind))
			continue
		}
		if spec.ProducedByPhase != "" {
			record, ok := findInteractionPhaseRecordForProduced(records, strings.TrimSpace(spec.ProducedByPhase), targetKind)
			if !ok {
				failures = append(failures, fmt.Sprintf("euclo.artifact_chain: phase %q did not produce %q", spec.ProducedByPhase, targetKind))
			} else {
				for _, needle := range spec.ContentContains {
					if !recordContainsArtifactContent(record.ProducedArtifacts, targetKind, needle) {
						failures = append(failures, fmt.Sprintf("euclo.artifact_chain: produced artifact %q in phase %q missing %q", targetKind, spec.ProducedByPhase, needle))
					}
				}
			}
		}
		if spec.ConsumedByPhase != "" {
			record, ok := findInteractionPhaseRecordForConsumed(records, strings.TrimSpace(spec.ConsumedByPhase), targetKind)
			if !ok {
				failures = append(failures, fmt.Sprintf("euclo.artifact_chain: phase %q did not consume %q", spec.ConsumedByPhase, targetKind))
			}
			_ = record
		}
	}
	return failures
}

func mapStringValue(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	return strings.TrimSpace(toString(record[key]))
}

func eucloBehaviorTraceFromSnapshot(snapshot *core.ContextSnapshot) map[string]any {
	if snapshot == nil {
		return nil
	}
	return toStringAnyMap(snapshot.State["euclo.relurpic_behavior_trace"])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type interactionPhaseRecord struct {
	Phase             string
	ArtifactsProduced []string
	ArtifactsConsumed []string
	ProducedArtifacts []map[string]any
}

func collectInteractionPhaseRecords(records []any) []interactionPhaseRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]interactionPhaseRecord, 0, len(records))
	for _, raw := range records {
		record := toStringAnyMap(raw)
		if record == nil {
			continue
		}
		phase := strings.TrimSpace(toString(record["phase"]))
		if phase == "" {
			continue
		}
		out = append(out, interactionPhaseRecord{
			Phase:             phase,
			ArtifactsProduced: toStringSlice(record["artifacts_produced"]),
			ArtifactsConsumed: toStringSlice(record["artifacts_consumed"]),
			ProducedArtifacts: toMapSlice(record["produced_artifacts"]),
		})
	}
	return out
}

func findInteractionPhaseRecordForProduced(records []interactionPhaseRecord, phase, kind string) (interactionPhaseRecord, bool) {
	for _, record := range records {
		if !strings.EqualFold(strings.TrimSpace(record.Phase), strings.TrimSpace(phase)) {
			continue
		}
		if stringSliceContainsNormalized(record.ArtifactsProduced, kind) {
			return record, true
		}
	}
	return interactionPhaseRecord{}, false
}

func findInteractionPhaseRecordForConsumed(records []interactionPhaseRecord, phase, kind string) (interactionPhaseRecord, bool) {
	for _, record := range records {
		if !strings.EqualFold(strings.TrimSpace(record.Phase), strings.TrimSpace(phase)) {
			continue
		}
		if stringSliceContainsNormalized(record.ArtifactsConsumed, kind) {
			return record, true
		}
	}
	return interactionPhaseRecord{}, false
}

func recordContainsArtifactContent(artifacts []map[string]any, kind, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	for _, artifact := range artifacts {
		if normalizeArtifactKind(toString(artifact["kind"])) != kind {
			continue
		}
		summary := strings.TrimSpace(toString(artifact["summary"]))
		if strings.Contains(summary, needle) {
			return true
		}
		payloadBytes, err := json.Marshal(artifact["payload"])
		if err == nil && strings.Contains(string(payloadBytes), needle) {
			return true
		}
	}
	return false
}

func normalizeArtifactKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	switch kind {
	case "":
		return ""
	case "exploration":
		return "euclo.explore"
	case "analysis":
		return "euclo.analyze"
	case "plan_candidates":
		return "euclo.plan_candidates"
	case "plan":
		return "euclo.plan"
	case "edit_intent":
		return "euclo.edit_intent"
	case "verification":
		return "euclo.verification"
	}
	if strings.HasPrefix(kind, "euclo.") {
		return kind
	}
	return "euclo." + kind
}

func stringSliceContainsNormalized(slice []string, target string) bool {
	for _, s := range slice {
		if normalizeArtifactKind(s) == target {
			return true
		}
	}
	return false
}

func collectArtifactKinds(snapshot *core.ContextSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	raw, ok := snapshot.State["euclo.artifacts"]
	if !ok {
		return nil
	}
	artifacts := toAnySlice(raw)
	var kinds []string
	for _, a := range artifacts {
		m := toStringAnyMap(a)
		if m == nil {
			continue
		}
		if kind, ok := m["kind"].(string); ok {
			kinds = append(kinds, kind)
			continue
		}
		if kind, ok := m["Kind"].(string); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func collectFrameKinds(frames []any) []string {
	var kinds []string
	for _, f := range frames {
		m := toStringAnyMap(f)
		if m == nil {
			continue
		}
		if kind, ok := m["kind"].(string); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func toAnySlice(raw any) []any {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var out []any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func toStringSlice(raw any) []string {
	values := toAnySlice(raw)
	if len(values) == 0 {
		if typed, ok := raw.([]string); ok {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toStringAnyMap(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	if typed, ok := raw.(map[string]interface{}); ok {
		return typed
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toMapSlice(raw any) []map[string]any {
	values := toAnySlice(raw)
	if len(values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if typed := toStringAnyMap(value); typed != nil {
			out = append(out, typed)
		}
	}
	return out
}

func recoveryTraceHasStrategy(trace map[string]any, strategy string) bool {
	if trace == nil || strings.TrimSpace(strategy) == "" {
		return false
	}
	rawAttempts, ok := trace["attempts"].([]any)
	if !ok {
		if typed, ok := trace["attempts"].([]map[string]any); ok {
			for _, attempt := range typed {
				if strings.EqualFold(strings.TrimSpace(toString(attempt["strategy"])), strings.TrimSpace(strategy)) {
					return true
				}
				if strings.EqualFold(strings.TrimSpace(toString(attempt["level"])), strings.TrimSpace(strategy)) {
					return true
				}
			}
		}
		return false
	}
	for _, raw := range rawAttempts {
		attempt, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(toString(attempt["strategy"])), strings.TrimSpace(strategy)) {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(toString(attempt["level"])), strings.TrimSpace(strategy)) {
			return true
		}
	}
	return false
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func stringSliceContains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func includeExpectedChangedFiles(filtered []string, before, after *WorkspaceSnapshot, expected []string) []string {
	if len(expected) == 0 || before == nil || after == nil {
		return filtered
	}
	seen := make(map[string]struct{}, len(filtered))
	for _, path := range filtered {
		seen[path] = struct{}{}
	}
	for _, path := range expected {
		path = filepath.ToSlash(filepath.Clean(path))
		if _, ok := seen[path]; ok {
			continue
		}
		beforeSum, beforeOK := before.Files[path]
		afterSum, afterOK := after.Files[path]
		if !beforeOK && !afterOK {
			continue
		}
		if beforeOK && afterOK && beforeSum == afterSum {
			continue
		}
		filtered = append(filtered, path)
		seen[path] = struct{}{}
	}
	return filtered
}

// eucloArtifactsFromSnapshot extracts Euclo artifacts from a context snapshot.
// Phase 4: Used by artifact_kind_produced expectation.
// Uses toAnySlice/toStringAnyMap for JSON round-trip support so it handles both
// []map[string]any and []euclotypes.Artifact stored in state.
func eucloArtifactsFromSnapshot(snapshot *core.ContextSnapshot) []map[string]any {
	if snapshot == nil {
		return nil
	}
	raw, ok := snapshot.State["euclo.artifacts"]
	if !ok || raw == nil {
		return nil
	}
	items := toAnySlice(raw)
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m := toStringAnyMap(item); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// artifactKindsFromArtifacts extracts artifact kind strings from artifact records.
// Phase 4: Used by artifact_kind_produced expectation.
// Checks both "kind" (JSON-serialized lowercase) and "Kind" (Go struct field name) keys.
// Each kind is stored both with and without the "euclo." namespace prefix so YAML
// writers can use either "euclo.analyze" or the short "analyze" form.
func artifactKindsFromArtifacts(artifacts []map[string]any) []string {
	if len(artifacts) == 0 {
		return nil
	}
	const prefix = "euclo."
	seen := make(map[string]bool)
	var kinds []string
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if !seen[k] {
			kinds = append(kinds, k)
			seen[k] = true
		}
		// Also store the short form (without "euclo." prefix) so either style matches.
		short := strings.TrimPrefix(k, prefix)
		if short != k && !seen[short] {
			kinds = append(kinds, short)
			seen[short] = true
		}
	}
	for _, a := range artifacts {
		kind, _ := a["kind"].(string)
		if kind == "" {
			kind, _ = a["Kind"].(string)
		}
		add(kind)
	}
	return kinds
}

// === OSB Model Functions (Phases 2-5) ===

// evaluateOutcomeExpectations evaluates hard goal-achievement assertions.
func evaluateOutcomeExpectations(spec OutcomeSpec, workspace, output string, changed []string, snapshot *core.ContextSnapshot, tokenUsage TokenUsageReport, memoryOutcome MemoryOutcomeReport) ([]AssertionResult, error) {
	var results []AssertionResult
	var failures []string

	if spec.MustSucceed {
		results = append(results, AssertionResult{
			AssertionID: "outcome.must_succeed",
			Tier:        "outcome",
			Passed:      true,
			Message:     "execution succeeded",
		})
	}

	for _, sub := range spec.OutputContains {
		passed := strings.Contains(output, sub)
		if !passed {
			failures = append(failures, fmt.Sprintf("output missing %q", sub))
		}
		results = append(results, AssertionResult{
			AssertionID: fmt.Sprintf("outcome.output_contains[%s]", sub),
			Tier:        "outcome",
			Passed:      passed,
			Message:     fmt.Sprintf("output contains %q", sub),
		})
	}

	for i, expr := range spec.OutputRegex {
		re, err := regexp.Compile(expr)
		if err != nil {
			failures = append(failures, fmt.Sprintf("invalid output regex %q: %v", expr, err))
			results = append(results, AssertionResult{
				AssertionID: fmt.Sprintf("outcome.output_regex[%d]", i),
				Tier:        "outcome",
				Passed:      false,
				Message:     fmt.Sprintf("invalid output regex %q: %v", expr, err),
			})
			continue
		}
		passed := re.MatchString(output)
		if !passed {
			failures = append(failures, fmt.Sprintf("output did not match %q", expr))
		}
		results = append(results, AssertionResult{
			AssertionID: fmt.Sprintf("outcome.output_regex[%d]", i),
			Tier:        "outcome",
			Passed:      passed,
			Message:     fmt.Sprintf("output matches %q", expr),
		})
	}

	for _, pat := range spec.FilesChanged {
		found := false
		for _, path := range changed {
			if matched, _ := filepath.Match(pat, path); matched || pat == path {
				found = true
				break
			}
		}
		if !found {
			failures = append(failures, fmt.Sprintf("expected file %s to be changed", pat))
		}
		results = append(results, AssertionResult{
			AssertionID: fmt.Sprintf("outcome.files_changed[%s]", pat),
			Tier:        "outcome",
			Passed:      found,
			Message:     fmt.Sprintf("file %s changed", pat),
		})
	}

	if spec.EucloMode != "" {
		results = append(results, AssertionResult{
			AssertionID: "outcome.euclo_mode",
			Tier:        "outcome",
			Passed:      true,
			Message:     fmt.Sprintf("euclo mode %s", spec.EucloMode),
		})
	}

	if len(failures) > 0 {
		return results, fmt.Errorf("outcome assertions failed: %s", strings.Join(failures, "; "))
	}
	return results, nil
}

// evaluateSecurityExpectations evaluates security boundary assertions.
func evaluateSecurityExpectations(spec SecuritySpec, m *manifest.AgentManifest, workspace string, transcript *ToolTranscriptArtifact) ([]AssertionResult, []SecurityObservation, error) {
	var results []AssertionResult
	var observations []SecurityObservation
	var failures []string

	expectedViolations := make(map[string]bool)
	for _, v := range spec.ExpectedViolations {
		key := fmt.Sprintf("%s:%s", v.Kind, v.Resource)
		expectedViolations[key] = true
	}

	if transcript != nil {
		for _, entry := range transcript.Entries {
			obs := SecurityObservation{
				Kind:      entry.Tool,
				Action:    "call",
				Timestamp: entry.CallAt.Format("2006-01-02T15:04:05Z"),
			}
			observations = append(observations, obs)

			isViolation := false
			if spec.NoWritesOutsideScope && obs.Kind == "file_write" && !obs.InScope {
				isViolation = true
			}
			if spec.NoReadsOutsideScope && obs.Kind == "file_read" && !obs.InScope {
				isViolation = true
			}
			for _, forbidden := range spec.ToolsMustNotCall {
				if obs.Kind == forbidden {
					isViolation = true
				}
			}
			if spec.MutationEnforced && obs.Kind == "file_write" && !obs.InScope {
				isViolation = true
			}
			if spec.NoNetworkOutsideManifest && obs.Kind == "network" && !obs.InScope {
				isViolation = true
			}
			if spec.NoExecOutsideManifest && obs.Kind == "exec" && !obs.InScope {
				isViolation = true
			}

			if isViolation {
				key := fmt.Sprintf("%s:%s", obs.Kind, obs.Resource)
				if !expectedViolations[key] {
					failures = append(failures, fmt.Sprintf("[security] %s on %s out of scope", obs.Kind, obs.Resource))
				}
			}
		}
	}

	if spec.NoWritesOutsideScope {
		passed := true
		for _, f := range failures {
			if strings.Contains(f, "file_write") {
				passed = false
				break
			}
		}
		results = append(results, AssertionResult{
			AssertionID: "security.no_writes_outside_scope",
			Tier:        "security",
			Passed:      passed,
		})
	}

	if spec.NoReadsOutsideScope {
		passed := true
		for _, f := range failures {
			if strings.Contains(f, "file_read") {
				passed = false
				break
			}
		}
		results = append(results, AssertionResult{
			AssertionID: "security.no_reads_outside_scope",
			Tier:        "security",
			Passed:      passed,
		})
	}

	if len(spec.ToolsMustNotCall) > 0 {
		passed := len(failures) == 0
		results = append(results, AssertionResult{
			AssertionID: "security.tools_must_not_call",
			Tier:        "security",
			Passed:      passed,
		})
	}

	if len(failures) > 0 {
		return results, observations, fmt.Errorf("[security] %s", strings.Join(failures, "; "))
	}
	return results, observations, nil
}

// evaluateBenchmarkExpectations evaluates soft telemetry observations.
func evaluateBenchmarkExpectations(spec BenchmarkSpec, transcript *ToolTranscriptArtifact, events []core.Event, snapshot *core.ContextSnapshot, tokenUsage TokenUsageReport) []BenchmarkObservation {
	var observations []BenchmarkObservation

	for _, tool := range spec.ToolsExpected {
		found := false
		if transcript != nil {
			for _, entry := range transcript.Entries {
				if entry.Tool == tool {
					found = true
					break
				}
			}
		}
		observations = append(observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tools_expected",
			Expected: tool,
			Actual:   boolToString(found),
			Matched:  found,
		})
	}

	for _, tool := range spec.ToolsNotExpected {
		found := false
		if transcript != nil {
			for _, entry := range transcript.Entries {
				if entry.Tool == tool {
					found = true
					break
				}
			}
		}
		observations = append(observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tools_not_expected",
			Expected: "absent:" + tool,
			Actual:   "present:" + boolToString(found),
			Matched:  !found,
		})
	}

	if len(spec.ToolSequenceExpected) > 0 {
		inOrder := toolCallsAppearInOrder(events, spec.ToolSequenceExpected)
		observations = append(observations, BenchmarkObservation{
			Category: "tool_usage",
			Field:    "tool_sequence_expected",
			Expected: strings.Join(spec.ToolSequenceExpected, ","),
			Actual:   boolToString(inOrder),
			Matched:  inOrder,
		})
	}

	if spec.LLMCallsExpected > 0 {
		actual := countLLMCalls(events)
		matched := actual == spec.LLMCallsExpected
		observations = append(observations, BenchmarkObservation{
			Category: "performance",
			Field:    "llm_calls",
			Expected: fmt.Sprintf("%d", spec.LLMCallsExpected),
			Actual:   fmt.Sprintf("%d", actual),
			Matched:  matched,
		})
	}

	if spec.TokenBudget != nil {
		if spec.TokenBudget.MaxPrompt > 0 && tokenUsage.PromptTokens > 0 {
			matched := tokenUsage.PromptTokens <= spec.TokenBudget.MaxPrompt
			observations = append(observations, BenchmarkObservation{
				Category: "token_usage",
				Field:    "token_budget.prompt",
				Expected: fmt.Sprintf("<=%d", spec.TokenBudget.MaxPrompt),
				Actual:   fmt.Sprintf("%d", tokenUsage.PromptTokens),
				Matched:  matched,
			})
		}
		if spec.TokenBudget.MaxCompletion > 0 && tokenUsage.CompletionTokens > 0 {
			matched := tokenUsage.CompletionTokens <= spec.TokenBudget.MaxCompletion
			observations = append(observations, BenchmarkObservation{
				Category: "token_usage",
				Field:    "token_budget.completion",
				Expected: fmt.Sprintf("<=%d", spec.TokenBudget.MaxCompletion),
				Actual:   fmt.Sprintf("%d", tokenUsage.CompletionTokens),
				Matched:  matched,
			})
		}
		if spec.TokenBudget.MaxTotal > 0 && tokenUsage.TotalTokens > 0 {
			matched := tokenUsage.TotalTokens <= spec.TokenBudget.MaxTotal
			observations = append(observations, BenchmarkObservation{
				Category: "token_usage",
				Field:    "token_budget.total",
				Expected: fmt.Sprintf("<=%d", spec.TokenBudget.MaxTotal),
				Actual:   fmt.Sprintf("%d", tokenUsage.TotalTokens),
				Matched:  matched,
			})
		}
	}

	return observations
}

// boolToString converts a bool to a string representation.
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
