package llm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

type kindFactory func(ProviderConfig, ProviderSecrets) (ManagedBackend, error)

var (
	kindFactoriesMu sync.RWMutex
	kindFactories   = map[string]kindFactory{}
)

// RegisterKind makes a backend kind available to the managed factory.
// Provider subpackages call this from init without creating an import cycle.
func RegisterKind(kind string, factory kindFactory) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || factory == nil {
		return
	}
	kindFactoriesMu.Lock()
	defer kindFactoriesMu.Unlock()
	if _, exists := kindFactories[kind]; exists {
		if testing.Testing() {
			panic(fmt.Sprintf("provider kind %q already registered", kind))
		}
	}
	kindFactories[kind] = factory
}

// New builds a managed backend from the provided transport configuration.
// Dispatch is on cfg.Kind; when Kind is empty, cfg.Provider is used as the kind
// (name-as-kind fallback for back-compat and CLI fakes).
func New(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(cfg.Provider))
	}
	if kind == "" {
		kind = "ollama"
	}
	// Ensure Provider is set for downstream Validate() calls
	// that still check the Provider field (transitional — Slice 2
	// rewrites Validate to check Kind instead).
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = kind
	}
	kindFactoriesMu.RLock()
	factory, ok := kindFactories[kind]
	kindFactoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider kind %q", kind)
	}
	backend, err := factory(cfg, secrets)
	if err != nil {
		return nil, err
	}
	return newCallingModeManagedBackend(backend), nil
}

// RegisteredKinds returns the list of provider kinds that have been
// registered (via init() or explicit RegisterKind calls).
func RegisteredKinds() []string {
	kindFactoriesMu.RLock()
	defer kindFactoriesMu.RUnlock()
	out := make([]string, 0, len(kindFactories))
	for kind := range kindFactories {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// IsRegisteredKind reports whether the given kind has been registered.
func IsRegisteredKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return false
	}
	kindFactoriesMu.RLock()
	defer kindFactoriesMu.RUnlock()
	_, ok := kindFactories[kind]
	return ok
}

// DefaultKindFactories returns a copy of the currently registered provider
// kind factories as an explicit map.
func DefaultKindFactories() map[string]kindFactory {
	kindFactoriesMu.RLock()
	defer kindFactoriesMu.RUnlock()
	out := make(map[string]kindFactory, len(kindFactories))
	for kind, factory := range kindFactories {
		out[kind] = factory
	}
	return out
}
