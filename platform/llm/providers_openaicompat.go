package llm

import (
	"context"
	"errors"

	openaicompatbackend "codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
)

func init() {
	RegisterKind("openai_compatible", func(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return openAICompatBackendAdapter{
			inner: openaicompatbackend.NewBackend(openaicompatbackend.BackendConfig{
				Endpoint:          cfg.Endpoint,
				Model:             cfg.Model,
				Timeout:           cfg.Timeout,
				NativeToolCalling: cfg.NativeToolCalling,
				Debug:             cfg.Debug,
			}, secrets.APIKey),
		}, nil
	})
}

type openAICompatBackendAdapter struct {
	inner *openaicompatbackend.Backend
}

func (a openAICompatBackendAdapter) Model() LanguageModel {
	if a.inner == nil {
		return nil
	}
	return a.inner.Model()
}

func (a openAICompatBackendAdapter) Embedder() Embedder {
	if a.inner == nil {
		return nil
	}
	return a.inner.Embedder()
}

func (a openAICompatBackendAdapter) Capabilities() BackendCapabilities {
	if a.inner == nil {
		return BackendCapabilities{}
	}
	return a.inner.Capabilities()
}

func (a openAICompatBackendAdapter) ModelContextSize(ctx context.Context) (int, error) {
	if a.inner == nil {
		return 0, nil
	}
	return a.inner.ModelContextSize(ctx)
}

func (a openAICompatBackendAdapter) Health(ctx context.Context) (*HealthReport, error) {
	if a.inner == nil {
		return nil, errors.New("openai_compatible backend adapter inner is nil")
	}
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
	}, err
}

func (a openAICompatBackendAdapter) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if a.inner == nil {
		return nil, nil
	}
	models, err := a.inner.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, ModelInfo{
			Name:          m.Name,
			Family:        m.Family,
			ParameterSize: m.ParameterSize,
			ContextSize:   m.ContextSize,
			Quantization:  m.Quantization,
			HasGPU:        m.HasGPU,
		})
	}
	return out, nil
}

func (a openAICompatBackendAdapter) Warm(ctx context.Context) error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Warm(ctx)
}

func (a openAICompatBackendAdapter) Close() error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Close()
}

func (a openAICompatBackendAdapter) SetDebugLogging(enabled bool) {
	if a.inner == nil {
		return
	}
	a.inner.SetDebugLogging(enabled)
}

func (a openAICompatBackendAdapter) SetProfile(profile *ModelProfile) {
	if a.inner == nil {
		return
	}
	a.inner.SetProfile(profile)
}

func (a openAICompatBackendAdapter) Reset(ctx context.Context, strategy string) error {
	// OpenAI-compatible backends have no reset API
	return nil
}

var _ ManagedBackend = openAICompatBackendAdapter{}
