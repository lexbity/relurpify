package authorization

import (
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/search"
)

// DecideByPatterns returns allow/deny/ask based on deny-first then allow list.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision core.AgentPermissionLevel) (core.AgentPermissionLevel, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return core.AgentPermissionDeny, pattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if search.MatchGlob(pattern, target) {
			return core.AgentPermissionAllow, pattern
		}
	}
	if defaultDecision == "" {
		defaultDecision = core.AgentPermissionAllow
	}
	return defaultDecision, ""
}
