package llm

import (
	"context"

	ollamabackend "codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

type managedBackendAdapter struct {
	inner *ollamabackend.Backend
}

func (a managedBackendAdapter) Model() LanguageModel {
	return a.inner.Model()
}

func (a managedBackendAdapter) Embedder() Embedder {
	return a.inner.Embedder()
}

func (a managedBackendAdapter) Capabilities() BackendCapabilities {
	caps := a.inner.Capabilities()
	return BackendCapabilities{
		NativeToolCalling:    caps.NativeToolCalling,
		Streaming:            caps.Streaming,
		Embeddings:           caps.Embeddings,
		ModelListing:         caps.ModelListing,
		BackendClass:         caps.BackendClass,
		UsageReporting:       caps.UsageReporting,
		ContextSizeDiscovery: caps.ContextSizeDiscovery,
	}
}

func (a managedBackendAdapter) ModelContextSize(ctx context.Context) (int, error) {
	if a.inner == nil {
		return 0, nil
	}
	return a.inner.ModelContextSize(ctx)
}

func (a managedBackendAdapter) Health(ctx context.Context) (*HealthReport, error) {
	report, err := a.inner.Health(ctx)
	if report == nil {
		return nil, err
	}
	return &HealthReport{
		State:       BackendHealthState(report.State),
		Message:     report.Message,
		LastError:   report.LastError,
		LastErrorAt: report.LastErrorAt,
		ErrorCount:  report.ErrorCount,
		UptimeSince: report.UptimeSince,
		Resources:   nil,
	}, err
}

func (a managedBackendAdapter) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := a.inner.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, len(models))
	for i, model := range models {
		out[i] = ModelInfo{
			Name:          model.Name,
			Family:        model.Family,
			ParameterSize: model.ParameterSize,
			ContextSize:   model.ContextSize,
			Quantization:  model.Quantization,
			HasGPU:        model.HasGPU,
		}
	}
	return out, nil
}

func (a managedBackendAdapter) Warm(ctx context.Context) error {
	return a.inner.Warm(ctx)
}

func (a managedBackendAdapter) Close() error {
	return a.inner.Close()
}

func (a managedBackendAdapter) SetDebugLogging(enabled bool) {
	a.inner.SetDebugLogging(enabled)
}

func (a managedBackendAdapter) SetProfile(profile *ModelProfile) {
	if a.inner == nil {
		return
	}
	a.inner.SetProfile(profile.AsOllamaProfile())
}

var _ ManagedBackend = managedBackendAdapter{}
