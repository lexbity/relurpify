package server

import (
	"context"
	"testing"
)

func TestNewRexRuntimeProvider(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	provider, err := NewRexRuntimeProvider(ctx, workspace)
	if err != nil {
		t.Fatalf("NewRexRuntimeProvider returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("NewRexRuntimeProvider returned nil provider")
	}

	// Verify basic fields are populated
	if provider.Agent == nil {
		t.Error("Agent should not be nil")
	}
	if provider.Adapter == nil {
		t.Error("Adapter should not be nil")
	}
	if provider.WorkflowStore == nil {
		t.Error("WorkflowStore should not be nil")
	}
	if provider.Environment == nil {
		t.Error("Environment should not be nil")
	}

	// Verify environment has required services
	if provider.Environment.Registry == nil {
		t.Error("Environment.Registry should not be nil")
	}
	if provider.Environment.IndexManager == nil {
		t.Error("Environment.IndexManager should not be nil")
	}
	if provider.Environment.SearchEngine == nil {
		t.Error("Environment.SearchEngine should not be nil")
	}
	if provider.Environment.WorkingMemory == nil {
		t.Error("Environment.WorkingMemory should not be nil")
	}

	// Clean up
	provider.Close()
}

func TestNewRexRuntimeProviderEmptyWorkspace(t *testing.T) {
	ctx := context.Background()
	_, err := NewRexRuntimeProvider(ctx, "")
	if err == nil {
		t.Error("NewRexRuntimeProvider should return error for empty workspace")
	}
}

func TestRexRuntimeProviderClose(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	provider, err := NewRexRuntimeProvider(ctx, workspace)
	if err != nil {
		t.Fatalf("NewRexRuntimeProvider returned error: %v", err)
	}

	// Close should not panic
	provider.Close()

	// Double close should not panic
	provider.Close()
}

func TestRexRuntimeProviderRegistration(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	provider, err := NewRexRuntimeProvider(ctx, workspace)
	if err != nil {
		t.Fatalf("NewRexRuntimeProvider returned error: %v", err)
	}
	defer provider.Close()

	_ = provider.Registration()
	// Registration should return a valid struct (even if empty)
	// We can't assert specific values without knowing the agent configuration
}
