package llm

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/llm/offline"
)

func init() {
	RegisterProvider("offline", func(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
		_ = secrets
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		applyProviderDefaults(&cfg)
		return offlineBackend{
			model:     offline.NewModel(),
			modelName: strings.TrimSpace(cfg.Model),
		}, nil
	})
}

type offlineBackend struct {
	model     offline.Model
	modelName string
}

func (b offlineBackend) Model() LanguageModel {
	return b.model
}

func (b offlineBackend) Embedder() Embedder {
	return nil
}

func (b offlineBackend) Capabilities() BackendCapabilities {
	return BackendCapabilities{
		Streaming:            true,
		ModelListing:         true,
		BackendClass:         BackendClassTransport,
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

func (b offlineBackend) SetProfile(*ModelProfile) {}

func (b offlineBackend) Reset(context.Context, string) error {
	return nil
}

var _ ManagedBackend = offlineBackend{}
