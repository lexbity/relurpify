package llm

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm/offline"
)

func init() {
	RegisterKind("offline", func(cfg ProviderConfig, secrets ProviderSecrets, gate *model.CallGate) (ManagedBackend, error) {
		_ = secrets
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(cfg.Model) == "" {
			cfg.Model = "offline-synthetic"
		}
		return offlineBackend{
			model:     offline.NewModel(),
			modelName: strings.TrimSpace(cfg.Model),
			gate:      gate,
		}, nil
	})
}

type offlineBackend struct {
	model     offline.Model
	modelName string
	gate      *model.CallGate
}

func (b offlineBackend) Model() model.LanguageModel {
	return NewGatedModel(b.model, b.gate)
}

func (b offlineBackend) Embedder() Embedder {
	return nil
}

func (b offlineBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		Streaming:            true,
		ModelListing:         true,
		BackendClass:         model.BackendClassTransport,
		UsageReporting:       false,
		ContextSizeDiscovery: false,
	}
}

func (b offlineBackend) ModelContextSize(context.Context) (int, error) {
	return 0, nil
}

func (b offlineBackend) Health(context.Context) (*HealthReport, error) {
	return &HealthReport{
		State:       BackendHealthReady,
		Message:     "offline backend ready",
		UptimeSince: time.Unix(0, 0).UTC(),
	}, nil
}

func (b offlineBackend) ListModels(context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{Name: b.modelName, Family: "offline", ContextSize: 0}}, nil
}

func (b offlineBackend) Warm(context.Context) error {
	return nil
}

func (b offlineBackend) Close() error {
	return nil
}

func (b offlineBackend) SetDebugLogging(bool) {}

func (b offlineBackend) SetProfile(*model.ModelProfile) {}

func (b offlineBackend) Reset(context.Context, string) error {
	return nil
}

var _ ManagedBackend = offlineBackend{}
