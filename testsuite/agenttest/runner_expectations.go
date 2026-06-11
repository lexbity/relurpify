package agenttest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)


func evaluateFileContentExpectations(expectations []FileContentExpectation, workspace string) ([]AssertionResult, []string) {
	if len(expectations) == 0 {
		return nil, nil
	}
	results := make([]AssertionResult, 0, len(expectations))
	failures := make([]string, 0)
	for _, expectation := range expectations {
		relPath := strings.TrimSpace(expectation.Path)
		if relPath == "" {
			continue
		}
		absPath := relPath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workspace, relPath)
		}
		data, err := os.ReadFile(filepath.Clean(absPath))
		if err != nil {
			failures = append(failures, fmt.Sprintf("expected file %s to be readable: %v", relPath, err))
			results = append(results, AssertionResult{
				AssertionID: fmt.Sprintf("outcome.files_contain[%s]", relPath),
				Tier:        "outcome",
				Passed:      false,
				Message:     absPath,
			})
			continue
		}
		content := string(data)
		passed := true
		for _, needle := range expectation.Contains {
			if !strings.Contains(content, needle) {
				passed = false
				failures = append(failures, fmt.Sprintf("file %s missing %q", relPath, needle))
			}
		}
		for _, banned := range expectation.NotContains {
			if strings.Contains(content, banned) {
				passed = false
				failures = append(failures, fmt.Sprintf("file %s unexpectedly contains %q", relPath, banned))
			}
		}
		results = append(results, AssertionResult{
			AssertionID: fmt.Sprintf("outcome.files_contain[%s]", relPath),
			Tier:        "outcome",
			Passed:      passed,
			Message:     absPath,
		})
	}
	return results, failures
}


