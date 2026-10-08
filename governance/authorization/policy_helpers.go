package authorization

import (
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// DecideByPatterns returns the typed decision for a target using deny-first
// then allow-list matching. An empty default resolves to the terminal ask
// default; an invalid (non-empty) default also resolves to ask rather than
// silently allowing.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision permissions.Decision) (permissions.Decision, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchGlob(pattern, target) {
			return permissions.DecisionDeny, pattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchGlob(pattern, target) {
			return permissions.DecisionAllow, pattern
		}
	}
	decision, err := permissions.DecisionOr(string(defaultDecision), permissions.DecisionAsk)
	if err != nil {
		return permissions.DecisionAsk, ""
	}
	return decision, ""
}
