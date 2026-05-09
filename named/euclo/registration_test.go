package euclo

import (
	"testing"
)

func TestGetRegistrationFuncs(t *testing.T) {
	regFuncs := GetRegistrationFuncs()

	// Verify that all registration functions are set
	if regFuncs.RegisterCapabilities == nil {
		t.Error("RegisterCapabilities should not be nil")
	}
	if regFuncs.RegisterPromptProviders == nil {
		t.Error("RegisterPromptProviders should not be nil")
	}
	if regFuncs.LoadThoughtRecipes == nil {
		t.Error("LoadThoughtRecipes should not be nil")
	}
}

func TestRegisterCapabilities(t *testing.T) {
	// Test that registerEucloCapabilities can be called
	// Note: This requires a full WorkspaceEnvironment setup, so we just verify
	// the function exists and has the correct signature
	regFuncs := GetRegistrationFuncs()
	if regFuncs.RegisterCapabilities == nil {
		t.Fatal("RegisterCapabilities should not be nil")
	}

	// We can't easily test this without a full WorkspaceEnvironment,
	// but we can verify the function is callable
	// This is more of an integration test scenario
}

func TestRegisterPromptProviders(t *testing.T) {
	regFuncs := GetRegistrationFuncs()
	if regFuncs.RegisterPromptProviders == nil {
		t.Fatal("RegisterPromptProviders should not be nil")
	}

	// Similar to RegisterCapabilities, this requires a full WorkspaceEnvironment
	// with a PromptRegistry to test properly
}

func TestLoadThoughtRecipes(t *testing.T) {
	regFuncs := GetRegistrationFuncs()
	if regFuncs.LoadThoughtRecipes == nil {
		t.Fatal("LoadThoughtRecipes should not be nil")
	}

	// Test that LoadThoughtRecipes can be called
	thoughtrecipes, err := regFuncs.LoadThoughtRecipes()
	if err != nil {
		t.Errorf("LoadThoughtRecipes returned error: %v", err)
	}
	if thoughtrecipes == nil {
		t.Error("LoadThoughtRecipes should not return nil")
	}
}
