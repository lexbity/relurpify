// Package llm provides the managed inference backend facade, transport
// adapters, and instrumentation wrappers for the Relurpify platform layer.
//
// # Managed backends
//
// Provider subpackages implement the framework/contracts.LanguageModel interface
// against local or OpenAI-compatible backends. LLM parameters are normalized
// by the transport-specific adapters.
//
// # InstrumentedModel
//
// instrumented_model.go wraps any LanguageModel to emit telemetry events for
// each call: token counts, latency, model name, and a truncated prompt digest.
//
// # TapeModel
//
// tape_model.go records LLM request/response pairs to a tape file (capture
// mode) and plays them back deterministically (replay mode), enabling agent
// integration tests to run without a live backend.
package llm
