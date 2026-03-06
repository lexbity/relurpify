package runtime

import (
	"context"
	"fmt"
)

// ManagedProvider is the minimal lifecycle surface for long-lived runtime services.
type ManagedProvider interface {
	Close() error
}

// RuntimeProvider can attach tools or state to a runtime and will be closed
// when the runtime shuts down.
type RuntimeProvider interface {
	ManagedProvider
	Initialize(ctx context.Context, rt *Runtime) error
}

// RegisterProvider initializes a provider against the runtime and records it
// for deterministic shutdown.
func (r *Runtime) RegisterProvider(ctx context.Context, provider RuntimeProvider) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if provider == nil {
		return fmt.Errorf("provider required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := provider.Initialize(ctx, r); err != nil {
		return err
	}
	r.providersMu.Lock()
	r.providers = append(r.providers, provider)
	r.providersMu.Unlock()
	return nil
}

func (r *Runtime) registeredProviders() []RuntimeProvider {
	if r == nil {
		return nil
	}
	r.providersMu.Lock()
	defer r.providersMu.Unlock()
	providers := append([]RuntimeProvider(nil), r.providers...)
	r.providers = nil
	return providers
}
