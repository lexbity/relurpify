# Migration Guide

This guide covers the final neutral inference surface.

## Field Renames

Replace legacy Ollama-branded config names with neutral inference names:

- `OllamaEndpoint` -> `InferenceEndpoint`
- `OllamaModel` -> `InferenceModel`
- `OllamaToolCalling` -> `NativeToolCalling`

The root runtime and manifest surfaces use the neutral names only.

## CLI Flag Renames

Replace the retired CLI flags with neutral equivalents:

- `--ollama-endpoint` -> `--inference-endpoint`
- `--ollama-model` -> `--inference-model`
- `--ollama-reset` -> `--backend-reset`
- `--ollama-reset-between` -> `--backend-reset-between`
- `--ollama-reset-on` -> `--backend-reset-on`
- `--ollama-bin` -> `--backend-bin`
- `--ollama-service` -> `--backend-service`

## Manifest Rename

Replace `ollama_tool_calling` with `native_tool_calling` in agent manifests.

## Notes

- Provider-specific names remain inside the Ollama implementation package and
  related provenance helpers.
- Workspace and CLI code should use neutral runtime fields; provider selection
  happens through the managed backend factory.
