package services

import (
	"io/fs"
	"testing"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestNewRegistrationAppliesOverrides(t *testing.T) {
	capReg := &stubCapabilityRegistrar{}
	promptReg := &stubPromptRegistrar{}
	thoughtrecipeLoader := &stubThoughtRecipeLoader{result: &thoughtrecipepkg.LoadResult{Registry: thoughtrecipepkg.NewThoughtRecipeRegistry()}}

	reg := NewRegistration(
		WithCapabilityRegistrar(capReg),
		WithPromptRegistrar(promptReg),
		WithThoughtRecipeLoader(thoughtrecipeLoader),
	)

	if err := reg.RegisterCapabilities(registry.NewRegistry()); err != nil {
		t.Fatalf("RegisterCapabilities returned error: %v", err)
	}
	if !capReg.called {
		t.Fatal("expected custom capability registrar to be called")
	}

	if err := reg.RegisterPromptProviders(&countingPromptRegistry{seen: make(map[string]bool)}); err != nil {
		t.Fatalf("RegisterPromptProviders returned error: %v", err)
	}
	if !promptReg.called {
		t.Fatal("expected custom prompt registrar to be called")
	}

	loaded, err := reg.LoadThoughtRecipes()
	if err != nil {
		t.Fatalf("LoadThoughtRecipes returned error: %v", err)
	}
	if loaded != thoughtrecipeLoader.result {
		t.Fatal("expected custom thoughtrecipe loader result to be returned")
	}
}

func TestDefaultCapabilityRegistrarNilRegistry(t *testing.T) {
	var reg defaultCapabilityRegistrar
	if err := reg.RegisterAll(nil); err == nil {
		t.Fatal("expected nil registry to fail")
	}
}

func TestDefaultCapabilityRegistrarRegistersCapabilities(t *testing.T) {
	var reg defaultCapabilityRegistrar
	r := registry.NewRegistry()
	if err := reg.RegisterAll(r); err != nil {
		t.Fatalf("RegisterAll returned error: %v", err)
	}
	if got := len(r.AllCapabilitySnapshots()); got == 0 {
		t.Fatal("expected capability registration to populate the registry")
	}
}

func TestDefaultPromptRegistrarRegistersAndSkipsDuplicates(t *testing.T) {
	var reg defaultPromptRegistrar
	registry := &countingPromptRegistry{seen: make(map[string]bool)}

	if err := reg.RegisterAll(registry); err != nil {
		t.Fatalf("first RegisterAll returned error: %v", err)
	}
	if got := registry.count(); got != 18 {
		t.Fatalf("expected 18 prompt providers, got %d", got)
	}

	if err := reg.RegisterAll(registry); err != nil {
		t.Fatalf("second RegisterAll returned error: %v", err)
	}
	if got := registry.count(); got != 18 {
		t.Fatalf("expected duplicate registration to be skipped, got %d providers", got)
	}
}

func TestDefaultThoughtRecipeLoaderLoadsRegistry(t *testing.T) {
	var loader defaultThoughtRecipeLoader

	result, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if got := result.Registry.Count(); got != 0 {
		t.Fatalf("expected empty registry, got %d thoughtrecipes", got)
	}
}

type stubCapabilityRegistrar struct {
	called bool
}

func (s *stubCapabilityRegistrar) RegisterAll(reg *registry.CapabilityRegistry) error {
	s.called = true
	return nil
}

type stubPromptRegistrar struct {
	called bool
}

func (s *stubPromptRegistrar) RegisterAll(registry prompt.Registry) error {
	s.called = true
	return nil
}

type stubThoughtRecipeLoader struct {
	called bool
	result *thoughtrecipepkg.LoadResult
}

func (s *stubThoughtRecipeLoader) LoadAll() (*thoughtrecipepkg.LoadResult, error) {
	s.called = true
	if s.result == nil {
		s.result = &thoughtrecipepkg.LoadResult{Registry: thoughtrecipepkg.NewThoughtRecipeRegistry()}
	}
	return s.result, nil
}

type countingPromptRegistry struct {
	seen map[string]bool
}

func (r *countingPromptRegistry) RegisterProvider(name string, p prompt.ContextProvider) error {
	if r.seen[name] {
		return prompt.ErrAlreadyRegistered(name)
	}
	r.seen[name] = true
	return nil
}

func (r *countingPromptRegistry) count() int {
	return len(r.seen)
}

func (r *countingPromptRegistry) LoadDir(string) error                        { return nil }
func (r *countingPromptRegistry) LoadFS(fs.FS, string) error                  { return nil }
func (r *countingPromptRegistry) ValidateProviders() []prompt.ValidationIssue { return nil }
func (r *countingPromptRegistry) Get(string) (*prompt.PromptConfig, bool)     { return nil, false }
func (r *countingPromptRegistry) All() []*prompt.PromptConfig                 { return nil }
func (r *countingPromptRegistry) Count() int                                  { return r.count() }
func (r *countingPromptRegistry) Filter(prompt.FilterOptions) []*prompt.PromptConfig {
	return nil
}
func (r *countingPromptRegistry) Resolve(string, prompt.RuntimeContext) (string, error) {
	return "", nil
}
func (r *countingPromptRegistry) ResolveDryRun(string, prompt.RuntimeContext) (prompt.DryRunResult, error) {
	return prompt.DryRunResult{}, nil
}
func (r *countingPromptRegistry) DependsOn(string) ([]string, error)    { return nil, nil }
func (r *countingPromptRegistry) DependentsOf(string) ([]string, error) { return nil, nil }
func (r *countingPromptRegistry) PromptVariables(string) (map[string]prompt.VariableDecl, error) {
	return nil, nil
}
func (r *countingPromptRegistry) Validate(string) []prompt.ValidationIssue { return nil }
func (r *countingPromptRegistry) ValidateAll() map[string][]prompt.ValidationIssue {
	return nil
}
