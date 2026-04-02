package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
)

func evaluateExpectations(expect ExpectSpec, workspace, output string, changed []string, toolCalls map[string]int, events []core.Event, tokenUsage TokenUsageReport, memoryOutcome MemoryOutcomeReport, snapshot *core.ContextSnapshot) error {
	var failures []string

	if expect.NoFileChanges && len(changed) > 0 {
		failures = append(failures, fmt.Sprintf("expected no file changes, got %d", len(changed)))
	}
	if len(expect.FilesChanged) > 0 {
		for _, pat := range expect.FilesChanged {
			found := false
			for _, path := range changed {
				if ok, _ := filepath.Match(filepath.Clean(pat), filepath.Clean(path)); ok || filepath.Clean(pat) == filepath.Clean(path) {
					found = true
					break
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("expected changed file %s", pat))
			}
		}
	}
	for _, sub := range expect.OutputContains {
		if !strings.Contains(output, sub) {
			failures = append(failures, fmt.Sprintf("output missing %q", sub))
		}
	}
	for _, expr := range expect.OutputRegex {
		re, err := regexp.Compile(expr)
		if err != nil {
			failures = append(failures, fmt.Sprintf("invalid output regex %q: %v", expr, err))
			continue
		}
		if !re.MatchString(output) {
			failures = append(failures, fmt.Sprintf("output did not match %q", expr))
		}
	}
	for _, fileExpect := range expect.FilesContain {
		if strings.TrimSpace(fileExpect.Path) == "" {
			failures = append(failures, "expected file content path")
			continue
		}
		path, err := resolvePathWithin(workspace, fileExpect.Path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("expected file %s: %v", fileExpect.Path, err))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("expected file %s readable: %v", fileExpect.Path, err))
			continue
		}
		content := string(data)
		for _, sub := range fileExpect.Contains {
			if !strings.Contains(content, sub) {
				failures = append(failures, fmt.Sprintf("file %s missing %q", fileExpect.Path, sub))
			}
		}
	}
	for _, tool := range expect.ToolCallsMustInclude {
		if toolCalls[tool] == 0 {
			failures = append(failures, fmt.Sprintf("expected tool call %s", tool))
		}
	}
	for _, tool := range expect.ToolCallsMustExclude {
		if toolCalls[tool] > 0 {
			failures = append(failures, fmt.Sprintf("tool %s should not have been called", tool))
		}
	}
	if expect.MaxToolCalls > 0 {
		total := 0
		for _, n := range toolCalls {
			total += n
		}
		if total > expect.MaxToolCalls {
			failures = append(failures, fmt.Sprintf("expected at most %d tool calls, got %d", expect.MaxToolCalls, total))
		}
	}
	if len(expect.ToolCallsInOrder) > 0 {
		if !toolCallsAppearInOrder(events, expect.ToolCallsInOrder) {
			failures = append(failures, fmt.Sprintf("expected tool calls in order %v", expect.ToolCallsInOrder))
		}
	}
	if expect.LLMCalls > 0 {
		got := countLLMCalls(events)
		if got != expect.LLMCalls {
			failures = append(failures, fmt.Sprintf("expected exactly %d llm calls, got %d", expect.LLMCalls, got))
		}
	}
	if expect.MaxPromptTokens > 0 && tokenUsage.PromptTokens > expect.MaxPromptTokens {
		failures = append(failures, fmt.Sprintf("expected at most %d prompt tokens, got %d", expect.MaxPromptTokens, tokenUsage.PromptTokens))
	}
	if expect.MaxCompletionTokens > 0 && tokenUsage.CompletionTokens > expect.MaxCompletionTokens {
		failures = append(failures, fmt.Sprintf("expected at most %d completion tokens, got %d", expect.MaxCompletionTokens, tokenUsage.CompletionTokens))
	}
	if expect.MaxTotalTokens > 0 && tokenUsage.TotalTokens > expect.MaxTotalTokens {
		failures = append(failures, fmt.Sprintf("expected at most %d total tokens, got %d", expect.MaxTotalTokens, tokenUsage.TotalTokens))
	}
	if expect.MemoryRecordsCreated > 0 && memoryOutcome.MemoryRecordsCreated < expect.MemoryRecordsCreated {
		failures = append(failures, fmt.Sprintf("expected at least %d memory records created, got %d", expect.MemoryRecordsCreated, memoryOutcome.MemoryRecordsCreated))
	}
	if expect.WorkflowStateUpdated && !memoryOutcome.WorkflowStateUpdated {
		failures = append(failures, "expected workflow state updated")
	}
	for _, key := range expect.StateKeysMustExist {
		if !contextSnapshotHasKey(snapshot, key) {
			failures = append(failures, fmt.Sprintf("expected state key %s", key))
		}
	}
	for _, key := range expect.StateKeysNotEmpty {
		if !contextSnapshotKeyNotEmpty(snapshot, key) {
			failures = append(failures, fmt.Sprintf("expected non-empty state key %s", key))
		}
	}
	if len(expect.WorkflowHasTensions) > 0 {
		workflowFailures := evaluateWorkflowTensionExpectations(workspace, expect.WorkflowHasTensions)
		failures = append(failures, workflowFailures...)
	}

	// Euclo-specific expectations.
	if expect.Euclo != nil {
		failures = append(failures, evaluateEucloExpectations(expect.Euclo, snapshot)...)
	}

	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
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

func evaluateWorkflowTensionExpectations(workspace string, workflowIDs []string) []string {
	var failures []string
	if strings.TrimSpace(workspace) == "" {
		return []string{"workflow tension expectations require workspace"}
	}
	storePath := config.New(workspace).WorkflowStateFile()
	store, err := memorydb.NewSQLiteWorkflowStateStore(storePath)
	if err != nil {
		return []string{fmt.Sprintf("workflow tension expectations: open workflow store: %v", err)}
	}
	defer store.Close()

	svc := archaeotensions.Service{Store: store}
	for _, workflowID := range workflowIDs {
		workflowID = strings.TrimSpace(workflowID)
		if workflowID == "" {
			failures = append(failures, "workflow_has_tensions requires workflow id")
			continue
		}
		tensions, err := svc.ActiveByWorkflow(context.Background(), workflowID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("workflow %s active tensions: %v", workflowID, err))
			continue
		}
		if len(tensions) == 0 {
			failures = append(failures, fmt.Sprintf("expected workflow %s to have active tensions", workflowID))
		}
	}
	return failures
}

func evaluateEucloExpectations(euclo *EucloExpectSpec, snapshot *core.ContextSnapshot) []string {
	var failures []string
	if snapshot == nil {
		if euclo.Mode != "" || euclo.Profile != "" || len(euclo.PhasesExecuted) > 0 {
			failures = append(failures, "euclo expectations set but no context snapshot")
		}
		return failures
	}

	// Extract euclo interaction state from snapshot.
	interactionState := toStringAnyMap(snapshot.State["euclo.interaction_state"])
	modeResolution := toStringAnyMap(snapshot.State["euclo.mode_resolution"])
	profileSelection := toStringAnyMap(snapshot.State["euclo.execution_profile_selection"])
	interactionRecording := toStringAnyMap(snapshot.State["euclo.interaction_recording"])
	interactionRecords := toAnySlice(snapshot.State["euclo.interaction_records"])
	profilePhaseRecords := toAnySlice(snapshot.State["euclo.profile_phase_records"])
	recoveryTrace := toStringAnyMap(snapshot.State["euclo.recovery_trace"])

	// Mode check.
	if euclo.Mode != "" {
		got := ""
		if interactionState != nil {
			got, _ = interactionState["mode"].(string)
		}
		if got == "" && modeResolution != nil {
			got, _ = modeResolution["mode_id"].(string)
		}
		if got != euclo.Mode {
			failures = append(failures, fmt.Sprintf("euclo.mode: got %q, want %q", got, euclo.Mode))
		}
	}

	// Profile check.
	if euclo.Profile != "" {
		got := ""
		if profileSelection != nil {
			got, _ = profileSelection["profile_id"].(string)
		}
		if got != euclo.Profile {
			failures = append(failures, fmt.Sprintf("euclo.profile: got %q, want %q", got, euclo.Profile))
		}
	}

	// Phases executed check.
	if len(euclo.PhasesExecuted) > 0 {
		gotPhases := toStringSlice(interactionState["phases_executed"])
		for _, expected := range euclo.PhasesExecuted {
			found := false
			for _, got := range gotPhases {
				if got == expected {
					found = true
					break
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("euclo.phases_executed: missing %q", expected))
			}
		}
	}

	// Phases skipped check.
	if len(euclo.PhasesSkipped) > 0 {
		gotSkipped := toStringSlice(interactionState["skipped_phases"])
		for _, expected := range euclo.PhasesSkipped {
			found := false
			for _, got := range gotSkipped {
				if got == expected {
					found = true
					break
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("euclo.phases_skipped: missing %q", expected))
			}
		}
	}

	// Artifacts produced check.
	if len(euclo.ArtifactsProduced) > 0 {
		artifactKinds := collectArtifactKinds(snapshot)
		for _, expected := range euclo.ArtifactsProduced {
			if !stringSliceContains(artifactKinds, expected) {
				failures = append(failures, fmt.Sprintf("euclo.artifacts_produced: missing %q", expected))
			}
		}
	}
	if len(euclo.ArtifactChain) > 0 {
		failures = append(failures, evaluateArtifactChainExpectations(euclo.ArtifactChain, append(append([]any{}, interactionRecords...), profilePhaseRecords...))...)
	}

	if euclo.RecoveryAttempted && recoveryTrace == nil {
		failures = append(failures, "expected recovery to be attempted but euclo.recovery_trace is nil")
	}
	if len(euclo.RecoveryStrategies) > 0 {
		for _, strategy := range euclo.RecoveryStrategies {
			if !recoveryTraceHasStrategy(recoveryTrace, strategy) {
				failures = append(failures, fmt.Sprintf("expected recovery strategy %q but not found in trace", strategy))
			}
		}
	}

	// Frame emission checks (from interaction recording).
	frames := toAnySlice(interactionRecording["frames"])

	if euclo.MinFramesEmitted > 0 && len(frames) < euclo.MinFramesEmitted {
		failures = append(failures, fmt.Sprintf("euclo.min_frames_emitted: got %d, want >= %d", len(frames), euclo.MinFramesEmitted))
	}
	if euclo.MaxFramesEmitted > 0 && len(frames) > euclo.MaxFramesEmitted {
		failures = append(failures, fmt.Sprintf("euclo.max_frames_emitted: got %d, want <= %d", len(frames), euclo.MaxFramesEmitted))
	}

	if len(euclo.FrameKindsEmitted) > 0 {
		gotKinds := collectFrameKinds(frames)
		for _, expected := range euclo.FrameKindsEmitted {
			if !stringSliceContains(gotKinds, expected) {
				failures = append(failures, fmt.Sprintf("euclo.frame_kinds_emitted: missing %q", expected))
			}
		}
	}

	if len(euclo.FrameKindsMustExclude) > 0 {
		gotKinds := collectFrameKinds(frames)
		for _, excluded := range euclo.FrameKindsMustExclude {
			if stringSliceContains(gotKinds, excluded) {
				failures = append(failures, fmt.Sprintf("euclo.frame_kinds_must_exclude: found %q", excluded))
			}
		}
	}

	// Transition checks.
	transitions := toAnySlice(interactionRecording["transitions"])
	if euclo.MinTransitionsProposed > 0 && len(transitions) < euclo.MinTransitionsProposed {
		failures = append(failures, fmt.Sprintf("euclo.min_transitions_proposed: got %d, want >= %d", len(transitions), euclo.MinTransitionsProposed))
	}
	if euclo.MaxTransitionsProposed > 0 && len(transitions) > euclo.MaxTransitionsProposed {
		failures = append(failures, fmt.Sprintf("euclo.max_transitions_proposed: got %d, want <= %d", len(transitions), euclo.MaxTransitionsProposed))
	}

	return failures
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

func findInteractionPhaseRecord(records []interactionPhaseRecord, phase string) (interactionPhaseRecord, bool) {
	for _, record := range records {
		if strings.EqualFold(strings.TrimSpace(record.Phase), strings.TrimSpace(phase)) {
			return record, true
		}
	}
	return interactionPhaseRecord{}, false
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
