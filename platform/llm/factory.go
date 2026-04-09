package llm

import (
	"fmt"
	"strings"
	"sync"
)

type providerFactory func(ProviderConfig) (ManagedBackend, error)

var (
	providerFactoriesMu sync.RWMutex
	providerFactories   = map[string]providerFactory{}
)

// RegisterProvider makes a backend provider available to the managed factory.
// Provider subpackages call this from init without creating an import cycle.
func RegisterProvider(name string, factory providerFactory) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || factory == nil {
		return
	}
	providerFactoriesMu.Lock()
	defer providerFactoriesMu.Unlock()
	providerFactories[name] = factory
}

// New builds a managed backend from the provided transport configuration.
func New(cfg ProviderConfig) (ManagedBackend, error) {
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = "ollama"
	}
	applyProviderDefaults(&cfg)
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	providerFactoriesMu.RLock()
	factory, ok := providerFactories[provider]
	providerFactoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
	return factory(cfg)
}

func applyProviderDefaults(cfg *ProviderConfig) {
	if cfg == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "ollama":
		if strings.TrimSpace(cfg.Endpoint) == "" {
			cfg.Endpoint = "http://localhost:11434"
		}
	case "lmstudio":
		if strings.TrimSpace(cfg.Endpoint) == "" {
			cfg.Endpoint = "http://localhost:1234"
		}
	}
}
