package llm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type providerFactory func(ProviderConfig, ProviderSecrets) (ManagedBackend, error)

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
func New(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
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
	return factory(cfg, secrets)
}

// RegisteredProviders returns the list of provider names that have been
// registered (via init() or explicit RegisterProvider calls).
func RegisteredProviders() []string {
	providerFactoriesMu.RLock()
	defer providerFactoriesMu.RUnlock()
	out := make([]string, 0, len(providerFactories))
	for name := range providerFactories {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// DefaultProviders returns a copy of the currently registered provider
// factories as an explicit map. Callers can select from the map and pass
// the selected entries to New or any other construction path.
func DefaultProviders() map[string]providerFactory {
	providerFactoriesMu.RLock()
	defer providerFactoriesMu.RUnlock()
	out := make(map[string]providerFactory, len(providerFactories))
	for name, factory := range providerFactories {
		out[name] = factory
	}
	return out
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
