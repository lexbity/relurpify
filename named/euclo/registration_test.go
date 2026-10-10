package euclo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestGetRegistrationFuncs(t *testing.T) {
	reg := services.NewRegistration()

	// Verify registration functions are method values on a valid receiver.
	_ = reg.RegisterCapabilities
	_ = reg.RegisterPromptProviders
	_ = reg.LoadThoughtRecipes
}

func TestLoadThoughtRecipes(t *testing.T) {
	reg := services.NewRegistration()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "relurpify_cfg", "euclo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "euclo", "probe.erpe"), []byte(testRecipeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	thoughtrecipes, err := reg.LoadThoughtRecipes(workspace, nil)
	if err != nil {
		t.Errorf("LoadThoughtRecipes returned error: %v", err)
	}
	if thoughtrecipes == nil {
		t.Error("LoadThoughtRecipes should not return nil")
	}
	if thoughtrecipes.Registry == nil {
		t.Error("LoadThoughtRecipes should return a non-nil registry")
	}
	if got := thoughtrecipes.Registry.Count(); got != 1 {
		t.Errorf("registry count = %d, want 1", got)
	}
}

// TestLoadThoughtRecipesMissingDirErrors: a resolved workspace without
// relurpify_cfg/euclo propagates ErrNoRecipeDir — the tolerance for absent
// recipe directories was deleted with the CWD-relative loading path (D-8).
func TestLoadThoughtRecipesMissingDirErrors(t *testing.T) {
	reg := services.NewRegistration()

	_, err := reg.LoadThoughtRecipes(t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for workspace without relurpify_cfg/euclo")
	}
	if !errors.Is(err, thoughtrecipepkg.ErrNoRecipeDir) {
		t.Fatalf("error %v is not ErrNoRecipeDir", err)
	}
}
