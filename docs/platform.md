# Platform Overview

## Scope

The platform layer contains concrete implementations for tools, browsers,
shell, search, LSP, and model backends.

The full platform package reference remains under [docs/framework/platform.md](framework/platform.md).

## LLM Facade

`platform/llm` is a provider-neutral facade over local and OpenAI-compatible
backends.

The root package provides:

- `New(cfg ProviderConfig) (ManagedBackend, error)`
- `ManagedBackend` lifecycle and health reporting
- `InstrumentedModel` telemetry wrapping
- `TapeModel` capture and replay

Provider-specific implementations live in subpackages:

- `platform/llm/ollama`
- `platform/llm/lmstudio`
- `platform/llm/openaicompat`

## Provider Selection

`ProviderConfig.Provider` selects the backend. If it is empty, the factory
defaults to Ollama. Provider defaults are applied in the root factory, not by
callers.

Use neutral runtime fields in application code:

- `InferenceProvider`
- `InferenceEndpoint`
- `InferenceModel`
- `InferenceNativeToolCalling`

Those values are normalized into the provider config before backend creation.
