package contextdata

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/model"
)

// modelUsageKey is the working-memory key carrying the run's model-call
// accounting. Envelope-owned (runtime state, never persisted): resume flows
// re-accumulate from zero, which is honest — the resumed turn's calls are its
// own.
const modelUsageKey = "euclo.run.model_usage"

// ModelUsage is the run's honest model-call record (D7): one entry per model
// invocation, retries included (a retry IS a call).
type ModelUsage struct {
	Calls            int
	PromptTokens     int
	CompletionTokens int
}

// TokenUsage returns the summed token consumption of the recorded calls.
func (m ModelUsage) TokenUsage() int {
	return m.PromptTokens + m.CompletionTokens
}

// RecordModelUsage accumulates one model response's usage into the run state.
// Called from the model-invocation boundary — every site that calls a
// LanguageModel with the run state in hand records here, so the completion
// emitters read a real counter instead of hardcoding zero. Both *Envelope
// (via State()) and the capability handlers' ports.State reach the same
// working store.
func RecordModelUsage(state ports.State, usage model.TokenUsage) {
	if state == nil {
		return
	}
	current := modelUsageFromState(state)
	current.Calls++
	current.PromptTokens += usage.PromptTokens
	current.CompletionTokens += usage.CompletionTokens
	state.SetWorkingValue(modelUsageKey, current)
}

// ModelUsageFromState returns the run's accumulated model usage (zero value
// when nothing was recorded — e.g. a turn that genuinely made no model call).
func ModelUsageFromState(state ports.State) ModelUsage {
	if state == nil {
		return ModelUsage{}
	}
	return modelUsageFromState(state)
}

// ModelUsageFromEnv is the envelope-side read for the completion emitters.
func ModelUsageFromEnv(env *Envelope) ModelUsage {
	if env == nil {
		return ModelUsage{}
	}
	return modelUsageFromState(env.State())
}

func modelUsageFromState(state ports.State) ModelUsage {
	usage, _ := state.Snapshot()[modelUsageKey].(ModelUsage)
	return usage
}
