package euclo

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/services"
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
	thoughtrecipes, err := reg.LoadThoughtRecipes(workspace, nil)
	if err != nil {
		t.Errorf("LoadThoughtRecipes returned error: %v", err)
	}
	if thoughtrecipes == nil {
		t.Error("LoadThoughtRecipes should not return nil")
	}
	if got, want := thoughtrecipes.SourceRoot, filepath.Join(workspace, "relurpify_cfg", "euclo"); got != want {
		t.Fatalf("source root = %q, want %q", got, want)
	}
}
