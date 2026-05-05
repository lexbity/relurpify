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
	if regFuncs.LoadRecipes == nil {
		t.Error("LoadRecipes should not be nil")
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

func TestLoadRecipes(t *testing.T) {
	regFuncs := GetRegistrationFuncs()
	if regFuncs.LoadRecipes == nil {
		t.Fatal("LoadRecipes should not be nil")
	}

	// Test that LoadRecipes can be called
	recipes, err := regFuncs.LoadRecipes()
	if err != nil {
		t.Errorf("LoadRecipes returned error: %v", err)
	}
	if recipes == nil {
		t.Error("LoadRecipes should not return nil")
	}
}
