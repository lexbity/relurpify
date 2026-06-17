package llm

import (
	ollamabackend "codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

func init() {
	RegisterKind("ollama", func(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return managedBackendAdapter{
			inner: ollamabackend.NewBackend(ollamabackend.Config{
				Endpoint:          cfg.Endpoint,
				Model:             cfg.Model,
				ModelPath:         cfg.ModelPath,
				Timeout:           cfg.Timeout,
				NativeToolCalling: cfg.NativeToolCalling,
				Debug:             cfg.Debug,
				Config:            cfg.Config,
			}, secrets.APIKey),
			modelName: cfg.Model,
		}, nil
	})
}
