package model

import "errors"

// Provider failure vocabulary. A backend reports a rejection caused by the
// request exceeding a model's context window or token budget with these typed
// sentinels, so callers can classify the failure with errors.Is instead of
// matching provider text. The wrapping happens at the provider boundary
// (platform/llm); the vocabulary lives here, in the leaf model-abstraction
// owner, because named/euclo consumers may not import platform/llm.

// ErrContextLength reports a request rejected for exceeding the model's
// context window (HTTP 413 or a provider context-length error body).
var ErrContextLength = errors.New("llm: context length exceeded")

// ErrTokenBudget reports a request rejected because a token budget was
// exhausted (for example a replayed tape entry recorded from such a rejection).
var ErrTokenBudget = errors.New("llm: token budget exhausted")
