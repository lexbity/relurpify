# Providers

This guide covers the supported local inference backends.

## Ollama

Use this provider when the backend is a local Ollama instance.

- Provider value: `ollama`
- Default endpoint: `http://localhost:11434`
- Key config fields: `InferenceProvider`, `InferenceEndpoint`, `InferenceModel`
- Native tool calling: controlled by `InferenceNativeToolCalling`

Capabilities:

- native chat/completions
- streaming
- model listing
- embeddings

Limitations:

- model availability depends on the local Ollama instance
- tool-call behavior is provider specific and may fall back to framework rendering

## LM Studio

Use this provider when the backend exposes an OpenAI-compatible local API.

- Provider value: `lmstudio`
- Default endpoint: `http://localhost:1234`
- Key config fields: `InferenceProvider`, `InferenceEndpoint`, `InferenceModel`
- Native tool calling: controlled by `InferenceNativeToolCalling`

Capabilities:

- chat/completions through the OpenAI-compatible transport
- streaming
- model listing
- embeddings

Limitations:

- functionality depends on the local LM Studio server and its model support
- OpenAI-compatible tool-call behavior is normalized through the shared
  transport layer
