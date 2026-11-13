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

// Gate failure vocabulary for P03 (one physical model lane enforcement).

// ErrNestedModelCall is returned when a model call attempts to acquire
// the gate while another call is already active on the same context per D-04.
var ErrNestedModelCall = errors.New("model: nested model call on active context")

// ErrGatePoisoned is returned when the gate has been poisoned and
// no new physical model requests can be admitted per D-04.
var ErrGatePoisoned = errors.New("model: gate poisoned")

// ErrGateClosed is returned when the gate has been closed for shutdown.
var ErrGateClosed = errors.New("model: gate closed")

// ErrQueueSaturated is returned when the queue is at capacity (128 per D-04).
var ErrQueueSaturated = errors.New("model: queue saturated")

// ErrQueueTimeout is returned when a queued call expires (300s default per D-04).
var ErrQueueTimeout = errors.New("model: queue timeout")

// ErrActiveTimeout is returned when an active call exceeds its deadline (900s default per D-04).
var ErrActiveTimeout = errors.New("model: active timeout")

// ErrLanePoisoned is returned when the model lane has been poisoned
// due to a non-compliant producer that could not be canceled per D-04.
var ErrLanePoisoned = errors.New("model: lane poisoned")
