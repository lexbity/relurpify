package llm

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
	lmstudiobackend "codeburg.org/lexbit/relurpify/platform/llm/lmstudio"
)

func init() {
	RegisterProvider("lmstudio", func(cfg ProviderConfig) (ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return lmStudioBackendAdapter{
			inner: lmstudiobackend.NewBackend(lmstudiobackend.Config{
				Endpoint:          cfg.Endpoint,
				Model:             cfg.Model,
				APIKey:            cfg.APIKey,
				Timeout:           cfg.Timeout,
				NativeToolCalling: cfg.NativeToolCalling,
				Debug:             cfg.Debug,
			}),
		}, nil
	})
}

type lmStudioBackendAdapter struct {
	inner *lmstudiobackend.Backend
}

func (a lmStudioBackendAdapter) Model() core.LanguageModel {
	if a.inner == nil {
		return nil
	}
	return a.inner.Model()
}

func (a lmStudioBackendAdapter) Embedder() Embedder {
	if a.inner == nil {
		return nil
	}
	return a.inner.Embedder()
}

func (a lmStudioBackendAdapter) Capabilities() core.BackendCapabilities {
	if a.inner == nil {
		return core.BackendCapabilities{}
	}
	return a.inner.Capabilities()
}

func (a lmStudioBackendAdapter) Health(ctx context.Context) (*HealthReport, error) {
	if a.inner == nil {
		return nil, nil
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
		Resources:   convertResourceSnapshot(report.Resources),
	}, err
}

func (a lmStudioBackendAdapter) ListModels(ctx context.Context) ([]ModelInfo, error) {
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

func (a lmStudioBackendAdapter) Warm(ctx context.Context) error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Warm(ctx)
}

func (a lmStudioBackendAdapter) Close() error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Close()
}

func (a lmStudioBackendAdapter) SetDebugLogging(enabled bool) {
	if a.inner == nil {
		return
	}
	a.inner.SetDebugLogging(enabled)
}

func (a lmStudioBackendAdapter) SetProfile(profile *ModelProfile) {
	if a.inner == nil {
		return
	}
	a.inner.SetProfile(profile.AsOpenAICompatProfile())
}

func convertResourceSnapshot(src *lmstudiobackend.ResourceSnapshot) *ResourceSnapshot {
	if src == nil {
		return nil
	}
	return &ResourceSnapshot{
		VRAMUsedMB:      src.VRAMUsedMB,
		VRAMTotalMB:     src.VRAMTotalMB,
		SystemRAMUsedMB: src.SystemRAMUsedMB,
		ThreadsActive:   src.ThreadsActive,
		KVCacheSlots:    src.KVCacheSlots,
		KVCacheUsed:     src.KVCacheUsed,
		ModelLoaded:     src.ModelLoaded,
	}
}
