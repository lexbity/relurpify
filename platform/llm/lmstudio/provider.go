package lmstudio

import (
	llm "codeburg.org/lexbit/relurpify/platform/llm"
)

func init() {
	llm.RegisterKind("lmstudio", func(cfg llm.ProviderConfig, secrets llm.ProviderSecrets) (llm.ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return NewBackend(Config{
			Endpoint:          cfg.Endpoint,
			Model:             cfg.Model,
			Timeout:           cfg.Timeout,
			NativeToolCalling: cfg.NativeToolCalling,
			Debug:             cfg.Debug,
		}, secrets.APIKey), nil
	})
}

var _ llm.ManagedBackend = (*Backend)(nil)
