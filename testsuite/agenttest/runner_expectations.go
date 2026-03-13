package agenttest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
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
