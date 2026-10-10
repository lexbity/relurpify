package orchestrate

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// TestCanonicalRecipesLoadClean: the materialized relurpify_cfg/euclo tree
// loads through the real loader (parser + semantics + capability resolution)
// and every canonical ID registers — the same path production recipe loading
// takes.
func TestCanonicalRecipesLoadClean(t *testing.T) {
	caps := registry.NewRegistry()
	for _, capID := range []string{
		"euclo:cap.test_run", "euclo:cap.ast_query", "euclo:cap.code_review",
		"euclo:cap.diff_summary",
	} {
		desc := descriptor.CapabilityDescriptor{
			ID:           capID,
			Name:         capID,
			Kind:         agentspec.CapabilityKindTool,
			Availability: descriptor.AvailabilitySpec{Available: true},
		}
		if err := caps.RegisterCapability(context.Background(), desc); err != nil {
			t.Fatalf("register %s: %v", capID, err)
		}
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
