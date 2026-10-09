package permissions

import (
	"fmt"
	"strings"
)

// Decision is the canonical three-value authorization decision vocabulary.
// Every default in the authorization layer is a non-empty Decision; the
// implicit terminal default is DecisionAsk (never allow).
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

// ParseDecision parses a decision string strictly. It trims surrounding
// whitespace and accepts any case; every other value is an error. This is the
// only way an untrusted string becomes a Decision.
func ParseDecision(s string) (Decision, error) {
	normalized := Decision(strings.ToLower(strings.TrimSpace(s)))
	switch normalized {
	case DecisionAllow, DecisionAsk, DecisionDeny:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid decision %q: want one of %q, %q, %q", s, DecisionAllow, DecisionAsk, DecisionDeny)
	}
}

// DecisionOr is the loader entry point. An empty (or whitespace-only) input
// yields def; an empty def yields DecisionAsk. A non-empty input is parsed
// strictly, so vocabulary violations surface as errors rather than silently
// defaulting.
func DecisionOr(s string, def Decision) (Decision, error) {
	if strings.TrimSpace(s) == "" {
		if def == "" {
			return DecisionAsk, nil
		}
		return def, nil
	}
	return ParseDecision(s)
}

// Valid reports whether v is a member of the decision vocabulary.
func (d Decision) Valid() bool {
	switch d {
	case DecisionAllow, DecisionAsk, DecisionDeny:
		return true
	default:
		return false
	}
}
