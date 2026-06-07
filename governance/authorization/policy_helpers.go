package authorization

import (
	"strings"

	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// DecideByPatterns returns allow/deny/ask based on deny-first then allow list.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision permissions.AgentPermissionLevel) (permissions.AgentPermissionLevel, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return permissions.AgentPermissionDeny, pattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return permissions.AgentPermissionAllow, pattern
		}
	}
	if defaultDecision == "" {
		defaultDecision = permissions.AgentPermissionAllow
	}
	return defaultDecision, ""
}
