// Package llm provides the Ollama LLM client and supporting wrappers for
// the Relurpify platform layer.
//
// # Ollama client
//
// ollama.go implements the framework/core.LanguageModel interface against a
// locally running Ollama instance. LLM parameters (temperature, top_p,
// num_predict, etc.) are passed inside the Ollama-required options sub-object.
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
// integration tests to run without a live Ollama instance.
package llm
