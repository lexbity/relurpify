package llm

// Provider-boundary classification of budget rejections. The sentinel
// vocabulary lives in the leaf model package (model.ErrContextLength /
// model.ErrTokenBudget), because named/euclo consumers classify these failures
// but may not import platform/llm. The wrapping happens here, where the HTTP
// status code and provider body are available; callers elsewhere classify with
// errors.Is and never match provider text (FR-10).

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

// contextLengthPhrases are the provider phrasings that identify a
// context-length rejection. Matching lives here, at the provider boundary.
var contextLengthPhrases = []string{ //nolint:gochecknoglobals // immutable provider vocabulary
	"context length",
	"context_length_exceeded",
	"context window",
	"maximum context",
	"too many tokens",
	"token limit",
	"reduce the length",
}

// IsContextLengthMessage reports whether a provider error body names a
// context-length rejection.
func IsContextLengthMessage(detail string) bool {
	lower := strings.ToLower(detail)
	for _, phrase := range contextLengthPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// ContextLengthError returns a wrapped model.ErrContextLength error when the
// HTTP status or body detail indicates a context-length rejection, and nil
// otherwise. Providers call it at their response-error boundary and fall back
// to their ordinary error formatting when it returns nil.
func ContextLengthError(statusCode int, detail string) error {
	if statusCode != 413 && !IsContextLengthMessage(detail) {
		return nil
	}
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		trimmed = "request exceeds the model context window"
	}
	return fmt.Errorf("%w: %s", model.ErrContextLength, trimmed)
}

// RecordedBudgetError reconstructs a typed budget failure from a tape entry's
// recorded error string. Tape replay has only the serialized message, so the
// string match is confined here; a recorded context-length rejection replays as
// model.ErrTokenBudget.
func RecordedBudgetError(message string) error {
	if !IsContextLengthMessage(message) {
		return nil
	}
	return fmt.Errorf("%w: %s", model.ErrTokenBudget, strings.TrimSpace(message))
}
