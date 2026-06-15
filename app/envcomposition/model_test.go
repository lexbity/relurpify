package envcomposition

import (
	"testing"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

func TestBuildModelRuntimeWithOfflineProvider(t *testing.T) {
	result, err := BuildModelRuntime(ModelRuntimeInput{
		Provider:  "offline",
		ModelName: "offline-synthetic",
		Secrets:   llm.ProviderSecrets{},
	})
	if err != nil {
		t.Fatalf("BuildModelRuntime with provider=offline failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ModelRuntime")
	}
	if result.Backend == nil {
		t.Fatal("expected non-nil Backend")
	}
	if result.ModelFactory == nil {
		t.Fatal("expected non-nil ModelFactory")
	}

	health, err := result.Backend.Health(nil)
	if err != nil {
		t.Fatalf("offline backend Health() failed: %v", err)
	}
	if health == nil {
		t.Fatal("expected non-nil HealthReport")
	}
	if health.State != "ready" {
		t.Fatalf("offline backend state = %q, want ready", health.State)
	}
}

func TestBuildModelRuntimeWithOfflineProviderAndProfile(t *testing.T) {
	profile := &model.ModelProfile{
		Pattern: "offline-synthetic",
	}
	result, err := BuildModelRuntime(ModelRuntimeInput{
		Provider:  "offline",
		ModelName: "offline-synthetic",
		Profile:   profile,
		Secrets:   llm.ProviderSecrets{},
	})
	if err != nil {
		t.Fatalf("BuildModelRuntime with offline+profile failed: %v", err)
	}
	if result.Backend == nil {
		t.Fatal("expected non-nil Backend")
	}
}
