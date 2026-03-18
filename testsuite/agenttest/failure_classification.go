package agenttest

import "strings"

func classifyCaseFailure(execErr error, caseErr string) string {
	if strings.TrimSpace(caseErr) == "" && execErr == nil {
		return ""
	}
	if execErr != nil {
		if isInfrastructureError(execErr.Error()) {
			return "infra"
		}
	}
	if isAssertionFailure(caseErr) {
		return "assertion"
	}
	if isInfrastructureError(caseErr) {
		return "infra"
	}
	return "agent"
}

func isAssertionFailure(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	fragments := []string{
		"expected no file changes",
		"expected changed file",
		"output missing",
		"output did not match",
		"file ",
		"invalid output regex",
		"expected tool call",
		"tool ",
		"expected tool calls in order",
		"expected exactly",
		"expected at most",
		"expected workflow state updated",
		"expected at least",
		"expected state key",
		"case marked must_succeed but failed",
		"mismatch for",
		"tape exhausted",
		"first request fingerprint mismatch",
	}
	for _, fragment := range fragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}

func isInfrastructureError(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if msg == "" {
		return false
	}
	fragments := []string{
		"context deadline exceeded",
		"connection refused",
		"connection reset",
		"no such file or directory",
		"replay tape unavailable",
		"unknown tape mode",
		"tape path required",
		"target workspace required",
		"command runner required",
		"ollama error:",
		"dial tcp",
		"timeout",
	}
	for _, fragment := range fragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}
