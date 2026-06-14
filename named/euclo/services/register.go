package services

import (
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// Registration provides Euclo's service registration functions.
type Registration struct {
	capabilityRegistrar CapabilityRegistrar
	promptRegistrar     PromptRegistrar
	thoughtrecipeLoader ThoughtRecipeLoader
}

// NewRegistration creates a new Registration with default implementations.
func NewRegistration(opts ...Option) *Registration {
	r := &Registration{
		capabilityRegistrar: &defaultCapabilityRegistrar{},
		promptRegistrar:     &defaultPromptRegistrar{},
		thoughtrecipeLoader: &defaultThoughtRecipeLoader{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// RegisterCapabilities registers all capabilities with the given registry.
func (r *Registration) RegisterCapabilities(reg *registry.CapabilityRegistry) error {
	return r.capabilityRegistrar.RegisterAll(reg)
}

// RegisterPromptProviders registers all prompt providers with the given registry.
func (r *Registration) RegisterPromptProviders(reg prompt.Registry) error {
	return r.promptRegistrar.RegisterAll(reg)
}

// LoadThoughtRecipes loads all thoughtrecipes from the given workspace using the
// provided capability registry for semantic validation.
func (r *Registration) LoadThoughtRecipes(workspace string, caps thoughtrecipe.CapabilityRegistryLookup) (*thoughtrecipe.LoadResult, error) {
	return r.thoughtrecipeLoader.LoadAll(workspace, caps)
}

func CapabilityLookup(reg *registry.CapabilityRegistry) thoughtrecipe.CapabilityRegistryLookup {
	if reg == nil {
		return nil
	}
	return capabilityLookupAdapter{reg: reg}
}

// Option configures the Registration.
type Option func(*Registration)

// WithCapabilityRegistrar sets a custom capability registrar.
func WithCapabilityRegistrar(cr CapabilityRegistrar) Option {
	return func(r *Registration) {
		r.capabilityRegistrar = cr
	}
}

// WithCapabilityDeps sets the session-level runtime dependencies used to
// construct live capability handlers. Call this instead of WithCapabilityRegistrar
// when using the default registrar with real IndexManager, CommandRunner, etc.
func WithCapabilityDeps(deps CapabilityDeps) Option {
	return func(r *Registration) {
		r.capabilityRegistrar = &defaultCapabilityRegistrar{deps: deps}
	}
}

// WithPromptRegistrar sets a custom prompt registrar.
func WithPromptRegistrar(pr PromptRegistrar) Option {
	return func(r *Registration) {
		r.promptRegistrar = pr
	}
}

// WithThoughtRecipeLoader sets a custom thoughtrecipe loader.
func WithThoughtRecipeLoader(rl ThoughtRecipeLoader) Option {
	return func(r *Registration) {
		r.thoughtrecipeLoader = rl
	}
}

// CapabilityRegistrar abstracts capability registration.
type CapabilityRegistrar interface {
	RegisterAll(reg *registry.CapabilityRegistry) error
}

// PromptRegistrar abstracts prompt provider registration.
type PromptRegistrar interface {
	RegisterAll(registry prompt.Registry) error
}

// ThoughtRecipeLoader abstracts thoughtrecipe loading.
type ThoughtRecipeLoader interface {
	LoadAll(workspace string, caps thoughtrecipe.CapabilityRegistryLookup) (*thoughtrecipe.LoadResult, error)
}

type capabilityLookupAdapter struct {
	reg *registry.CapabilityRegistry
}

func (a capabilityLookupAdapter) Select(id string) (descriptor.CapabilityDescriptor, bool) {
	if a.reg == nil {
		return descriptor.CapabilityDescriptor{}, false
	}
	return a.reg.GetCapability(id)
}
