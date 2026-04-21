package llm

import (
	ollamabackend "codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

func init() {
	RegisterProvider("ollama", func(cfg ProviderConfig) (ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return managedBackendAdapter{inner: ollamabackend.NewBackend(ollamabackend.Config{
			Endpoint:          cfg.Endpoint,
			Model:             cfg.Model,
			ModelPath:         cfg.ModelPath,
			APIKey:            cfg.APIKey,
			Timeout:           cfg.Timeout,
			NativeToolCalling: cfg.NativeToolCalling,
			Debug:             cfg.Debug,
			Config:            cfg.Config,
		})}, nil
	})
}
