package euclo

import (
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

	thoughtrecipes, err := reg.LoadThoughtRecipes()
	if err != nil {
		t.Errorf("LoadThoughtRecipes returned error: %v", err)
	}
	if thoughtrecipes == nil {
		t.Error("LoadThoughtRecipes should not return nil")
	}
}
