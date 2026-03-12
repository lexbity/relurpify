package agenttest

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func evaluateExpectations(expect ExpectSpec, output string, changed []string, toolCalls map[string]int, snapshot *core.ContextSnapshot) error {
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
