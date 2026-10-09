package authorization

import (
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// Command-decision reason codes (typed, never bare strings at call sites).
const (
	commandReasonDenyPattern  = "deny_pattern"
	commandReasonAllowPattern = "allow_pattern"
	commandReasonDefault      = "default"
)

// CommandDecision is the typed result of a command-pattern decision.
type CommandDecision struct {
	Decision       permissions.Decision
	MatchedPattern string
	Reason         string
}

// DecideByPatterns returns the typed decision for a path target using
// deny-first then allow-list path-glob matching (matchGlob). An empty or
// invalid default resolves to ask.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision permissions.Decision) (permissions.Decision, string) {
	decision, matched, _ := decideByMatcher(target, allowPatterns, denyPatterns, defaultDecision, matchGlob)
	return decision, matched
}

// DecideCommandByPatterns returns the typed command decision using command-text
// glob semantics (MatchCommandGlob). Command patterns and path patterns use
// different grammars by design (D-8).
func DecideCommandByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision permissions.Decision) CommandDecision {
	decision, matched, reason := decideByMatcher(target, allowPatterns, denyPatterns, defaultDecision, MatchCommandGlob)
	return CommandDecision{Decision: decision, MatchedPattern: matched, Reason: reason}
}

// decideByMatcher is the shared deny-first evaluator parameterized by matcher.
func decideByMatcher(target string, allowPatterns, denyPatterns []string, defaultDecision permissions.Decision, match func(string, string) bool) (permissions.Decision, string, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if match(pattern, target) {
			return permissions.DecisionDeny, pattern, commandReasonDenyPattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if match(pattern, target) {
			return permissions.DecisionAllow, pattern, commandReasonAllowPattern
		}
	}
	decision, err := permissions.DecisionOr(string(defaultDecision), permissions.DecisionAsk)
	if err != nil {
		decision = permissions.DecisionAsk
	}
	return decision, "", commandReasonDefault
}
