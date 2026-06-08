package services

import (
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// Registration provides Euclo's service registration functions.
// This is the primary entrypoint for framework integration.
type Registration struct {
	capabilityRegistrar CapabilityRegistrar
	promptRegistrar     PromptRegistrar
	thoughtrecipeLoader ThoughtRecipeLoader
}

// NewRegistration creates a new Registration with default implementations.
// Optional overrides are applied after the defaults are installed.
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

// AgentRegistrationFuncs returns the registration functions for agentenv.
func (r *Registration) AgentRegistrationFuncs() agentenv.AgentRegistrationFuncs {
	return agentenv.AgentRegistrationFuncs{
		RegisterCapabilities:    r.registerCapabilities,
		RegisterPromptProviders: r.registerPromptProviders,
		LoadThoughtRecipes:      r.loadThoughtRecipes,
	}
}

func (r *Registration) registerCapabilities(env agentenv.AgentContext) error {
	return r.capabilityRegistrar.RegisterAll(env)
}

func (r *Registration) registerPromptProviders(env agentenv.AgentContext) error {
	return r.promptRegistrar.RegisterAll(env)
}

func (r *Registration) loadThoughtRecipes() (interface{}, error) {
	return r.thoughtrecipeLoader.LoadAll()
}

// Option configures the Registration.
type Option func(*Registration)

// WithCapabilityRegistrar sets a custom capability registrar.
func WithCapabilityRegistrar(cr CapabilityRegistrar) Option {
	return func(r *Registration) {
		r.capabilityRegistrar = cr
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
	RegisterAll(env agentenv.AgentContext) error
}

// PromptRegistrar abstracts prompt provider registration.
type PromptRegistrar interface {
	RegisterAll(env agentenv.AgentContext) error
}

// ThoughtRecipeLoader abstracts thoughtrecipe loading.
type ThoughtRecipeLoader interface {
	LoadAll() (*thoughtrecipe.LoadResult, error)
}
