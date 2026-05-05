package services

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	recipe "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// Registration provides Euclo's service registration functions.
// This is the primary entrypoint for framework integration.
type Registration struct {
	capabilityRegistrar CapabilityRegistrar
	promptRegistrar     PromptRegistrar
	recipeLoader        RecipeLoader
}

// NewRegistration creates a new Registration with default implementations.
// Optional overrides are applied after the defaults are installed.
func NewRegistration(opts ...Option) *Registration {
	r := &Registration{
		capabilityRegistrar: &defaultCapabilityRegistrar{},
		promptRegistrar:     &defaultPromptRegistrar{},
		recipeLoader:        &defaultRecipeLoader{},
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
		LoadRecipes:             r.loadRecipes,
	}
}

func (r *Registration) registerCapabilities(env agentenv.WorkspaceEnvironment) error {
	return r.capabilityRegistrar.RegisterAll(env)
}

func (r *Registration) registerPromptProviders(env agentenv.WorkspaceEnvironment) error {
	return r.promptRegistrar.RegisterAll(env)
}

func (r *Registration) loadRecipes() (interface{}, error) {
	return r.recipeLoader.LoadAll()
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

// WithRecipeLoader sets a custom recipe loader.
func WithRecipeLoader(rl RecipeLoader) Option {
	return func(r *Registration) {
		r.recipeLoader = rl
	}
}

// CapabilityRegistrar abstracts capability registration.
type CapabilityRegistrar interface {
	RegisterAll(env agentenv.WorkspaceEnvironment) error
}

// PromptRegistrar abstracts prompt provider registration.
type PromptRegistrar interface {
	RegisterAll(env agentenv.WorkspaceEnvironment) error
}

// RecipeLoader abstracts recipe loading.
type RecipeLoader interface {
	LoadAll() (*recipe.RecipeRegistry, error)
}
