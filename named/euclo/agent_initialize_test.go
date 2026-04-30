package euclo

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

func TestAgentInitializeDoesNotPanic(t *testing.T) {
	// Create a minimal WorkspaceEnvironment
	env := agentenv.WorkspaceEnvironment{
		Registry: capability.NewCapabilityRegistry(),
	}

	// Create agent with the environment
	agent := New(env)

	// Initialize should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize panicked: %v", r)
		}
	}()

	err := agent.Initialize(nil)
	if err != nil {
		t.Fatalf("agent.Initialize failed: %v", err)
	}

	// Verify initialization state
	if !agent.initialized {
		t.Fatal("agent should be initialized after Initialize call")
	}

	// Verify recipeRegistry is set
	if agent.recipeRegistry == nil {
		t.Fatal("recipeRegistry should be set after Initialize")
	}
}

func TestAgentInitializeWithNilRegistry(t *testing.T) {
	// Test with nil registry - should error gracefully
	env := agentenv.WorkspaceEnvironment{
		Registry: nil,
	}

	agent := New(env)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize with nil registry panicked: %v", r)
		}
	}()

	err := agent.Initialize(nil)
	// Should error because registry is nil
	if err == nil {
		t.Fatal("expected error when env.Registry is nil")
	}
}
