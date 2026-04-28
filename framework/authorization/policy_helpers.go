package authorization

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// DecideByPatterns returns allow/deny/ask based on deny-first then allow list.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision contracts.AgentPermissionLevel) (contracts.AgentPermissionLevel, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return contracts.AgentPermissionDeny, pattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return contracts.AgentPermissionAllow, pattern
		}
	}
	if defaultDecision == "" {
		defaultDecision = contracts.AgentPermissionAllow
	}
	return defaultDecision, ""
}
