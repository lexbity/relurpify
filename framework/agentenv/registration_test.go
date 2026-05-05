package agentenv

import (
	"testing"
)

func TestAgentRegistrationFuncs(t *testing.T) {
	// Test that AgentRegistrationFuncs can be created with nil functions
	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities:     nil,
		RegisterPromptProviders: nil,
		LoadRecipes:             nil,
	}

	if regFuncs.RegisterCapabilities != nil {
		t.Error("RegisterCapabilities should be nil")
	}
	if regFuncs.RegisterPromptProviders != nil {
		t.Error("RegisterPromptProviders should be nil")
	}
	if regFuncs.LoadRecipes != nil {
		t.Error("LoadRecipes should be nil")
	}
}

func TestAgentRegistrationFuncsWithFunctions(t *testing.T) {
	// Test that AgentRegistrationFuncs can be created with actual functions
	called := false
	regFuncs := AgentRegistrationFuncs{
		RegisterCapabilities: func(env WorkspaceEnvironment) error {
			called = true
			return nil
		},
		RegisterPromptProviders: nil,
		LoadRecipes:             nil,
	}

	if regFuncs.RegisterCapabilities == nil {
		t.Error("RegisterCapabilities should not be nil")
	}

	// Call the function
	env := WorkspaceEnvironment{}
	err := regFuncs.RegisterCapabilities(env)
	if err != nil {
		t.Errorf("RegisterCapabilities returned error: %v", err)
	}
	if !called {
		t.Error("RegisterCapabilities was not called")
	}
}
