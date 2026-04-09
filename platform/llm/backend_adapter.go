package llm

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	ollamabackend "github.com/lexcodex/relurpify/platform/llm/ollama"
)

type managedBackendAdapter struct {
	inner *ollamabackend.Backend
}

func (a managedBackendAdapter) Model() core.LanguageModel {
	return a.inner.Model()
}

func (a managedBackendAdapter) Embedder() Embedder {
	return a.inner.Embedder()
}

func (a managedBackendAdapter) Capabilities() core.BackendCapabilities {
	caps := a.inner.Capabilities()
	return core.BackendCapabilities{
		NativeToolCalling: caps.NativeToolCalling,
		Streaming:         caps.Streaming,
		Embeddings:        caps.Embeddings,
		ModelListing:      caps.ModelListing,
		BackendClass:      caps.BackendClass,
	}
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

var _ ManagedBackend = managedBackendAdapter{}
