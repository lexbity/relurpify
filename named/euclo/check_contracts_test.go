package euclo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// TestShippedRecipesLoadUnderContracts iterates the materialized
// relurpify_cfg/euclo/*.erpe tree through the real loader — parser, semantic
// pass (which now includes the paradigm contract pass), lowering, and plan
// registration (which re-validates the plan against the contract registry).
// Every shipped recipe must load clean and pass plan-level contract
// validation; this is the NFR-12 "shipped recipes load" gate for the contract
// change.
func TestShippedRecipesLoadUnderContracts(t *testing.T) {
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

	// The loader scans <root>/relurpify_cfg/euclo; named/euclo sits two
	// levels below the repository root, so the loader's working root is
	// ../..
	root := filepath.Join("..", "..")
	sourceRoot := filepath.Join(root, thoughtrecipe.ThoughtRecipeSourceRoot)
	if _, err := os.Stat(sourceRoot); err != nil {
		t.Fatalf("shipped recipe tree %q not found (run from repo root): %v", sourceRoot, err)
	}

	loader := thoughtrecipe.NewLoader().WithCapabilityRegistry(services.CapabilityLookup(caps))
	result, err := loader.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if result == nil || result.Registry == nil {
		t.Fatal("no registry")
	}

	entries := result.Registry.Entries()
	if len(entries) == 0 {
		t.Fatal("expected shipped recipes to be registered")
	}
	for _, entry := range entries {
		if entry.Plan == nil {
			t.Errorf("recipe %q has no compiled plan", entry.Name)
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			if err := thoughtrecipe.ValidatePlanContracts(entry.Plan, paradigm.Registry); err != nil {
				t.Fatalf("plan contract validation failed: %v", err)
			}
		})
	}
}
