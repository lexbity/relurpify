package testfu

import (
	"path/filepath"
	"sort"

	agenttestpkg "codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func suitePassed(report *agenttestpkg.SuiteReport) bool {
	if report == nil {
		return false
	}
	for _, c := range report.Cases {
		if c.Skipped {
			continue
		}
		if !c.Success {
			return false
		}
	}
	return true
}

func failedCaseNames(report map[string]any) []string {
	// Multi-suite result from actionRunAgent.
	if suites, ok := report["suites"].(map[string]*agenttestpkg.SuiteReport); ok {
		var out []string
		for _, sr := range suites {
			for _, c := range sr.Cases {
				if !c.Success && !c.Skipped {
					out = append(out, c.Name)
				}
			}
		}
		sort.Strings(out)
		return out
	}
	switch typed := report["suite"].(type) {
	case *agenttestpkg.SuiteReport:
		out := make([]string, 0)
		for _, c := range typed.Cases {
			if !c.Success && !c.Skipped {
				out = append(out, c.Name)
			}
		}
		return out
	case *agenttestpkg.CaseReport:
		if typed.Success || typed.Skipped {
			return nil
		}
		return []string{typed.Name}
	default:
		return nil
	}
}

func listSuites(workspace string) ([]map[string]any, error) {
	patterns := []string{
		filepath.Join(workspace, "testsuite", "agenttests", "*.testsuite.yaml"),
		filepath.Join(workspace, "relurpify_cfg", "testsuites", "*.testsuite.yaml"),
	}
	results := make([]map[string]any, 0)
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			results = append(results, map[string]any{
				"path": match,
				"name": filepath.Base(match),
			})
		}
	}
	return results, nil
}
