package orchestrate

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// TestCanonicalRecipesLoadClean: the materialized relurpify_cfg/euclo tree
// loads through the real loader (parser + semantics + capability resolution)
// and every canonical ID registers — the same path production recipe loading
// takes.
func TestCanonicalRecipesLoadClean(t *testing.T) {
	caps := registry.NewRegistry()
	// Declared vocabulary: the full go_native catalog plus the shipped
	// relurpic capability set, registered the way the runtime does (FR-5).
	// Registry coherence with the manifests is the composition gate's
	// contract (app/envcomposition); this corpus gate exercises the loader.
	catalog, err := toolcapabilities.CatalogForWorkspace("../../../")
	if err != nil {
		t.Fatalf("compose native catalog: %v", err)
	}
	for _, tool := range catalog {
		if err := caps.Register(context.Background(), tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	if err := relurpicabilities.RegisterAll(relurpicabilities.RegistrationDeps{
		Registry: caps,
		Declared: services.EucloCapabilityIDs(),
	}); err != nil {
		t.Fatalf("register relurpic capabilities: %v", err)
	}
	loader := thoughtrecipepkg.NewLoader().WithCapabilityRegistry(services.CapabilityLookup(caps))
	result, err := loader.LoadWorkspace("../../../")
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if result == nil || result.Registry == nil {
		t.Fatal("no registry")
	}
	for _, id := range canonicalTestRecipeIDs {
		if _, ok := result.Registry.Get(id); !ok {
			t.Errorf("canonical recipe %q not registered", id)
		}
	}
}

var canonicalTestRecipeIDs = []string{
	"euclo.thoughtrecipe.default",
	"euclo.thoughtrecipe.code_review",
	"euclo.thoughtrecipe.investigation",
	"euclo.thoughtrecipe.debug_tdd_repair",
	"euclo.thoughtrecipe.dep_upgrade",
	"euclo.thoughtrecipe.test_synthesis",
	"euclo.thoughtrecipe.extract_func",
}

// TestLoadWorkspaceMissingRecipeDirErrors pins the empty-vs-missing
// distinction (§5.5): under a resolved, non-empty workspace the loader
// returns typed ErrNoRecipeDir when relurpify_cfg/euclo is absent — the
// os.ErrNotExist tolerance was deleted with CWD-relative loading (D-8).
func TestLoadWorkspaceMissingRecipeDirErrors(t *testing.T) {
	loader := thoughtrecipepkg.NewLoader()
	_, err := loader.LoadWorkspace(t.TempDir())
	if err == nil {
		t.Fatal("expected ErrNoRecipeDir for workspace without relurpify_cfg/euclo")
	}
	if !errors.Is(err, thoughtrecipepkg.ErrNoRecipeDir) {
		t.Fatalf("error %v is not ErrNoRecipeDir", err)
	}

	if _, err := loader.LoadWorkspace(""); err == nil {
		t.Fatal("expected error for empty workspace root (CWD-relative scan forbidden)")
	}
}
