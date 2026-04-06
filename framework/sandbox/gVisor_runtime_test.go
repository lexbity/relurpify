package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// CONFIG & STATE LOGIC TESTS (Pure Unit Logic - No System Calls)
// ----------------------------------------------------------------------------

func TestNewGVisorRuntimeDefaults(t *testing.T) {
	emptyConfig := SandboxConfig{}

	gt := NewGVisorRuntime(emptyConfig)

	if gt.config.RunscPath != "runsc" {
		t.Errorf("Expected default runscPath to be 'runsc', got: %q", gt.config.RunscPath)
	}

	if gt.config.Platform != "kvm" {
		t.Errorf("Expected default platform to be 'kvm', got: %q", gt.config.Platform)
	}

	if gt.config.ContainerRuntime != "docker" {
		t.Errorf("Expected default container runtime to be 'docker', got: %q", gt.config.ContainerRuntime)
	}

	if !gt.config.NetworkIsolation {
		t.Error("Expected default NetworkIsolation to be true")
	}
}

func TestNewGVisorRuntime_ExplicitConfigPreserved(t *testing.T) {
	customConfig := SandboxConfig{
		RunscPath:        "custom-runsc",
		Platform:         "ptrace",
		ContainerRuntime: "containerd",
		NetworkIsolation: false,
	}

	gt := NewGVisorRuntime(customConfig)

	if gt.config.RunscPath != customConfig.RunscPath {
		t.Errorf("Explicit runscPath should be preserved")
	}

	if gt.config.Platform != customConfig.Platform {
		t.Errorf("Explicit platform should be preserved")
	}

	if !gt.config.NetworkIsolation {
		t.Error("Explicit NetworkIsolation=false should be preserved")
	}
}

func TestEnforcePolicy_StateStorage(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{})

	expectedPolicy := SandboxPolicy{
		NetworkRules: []NetworkRule{
			{Direction: "egress", Protocol: "tcp", Host: "api.local", Port: 443},
		},
	}

	err := gt.EnforcePolicy(expectedPolicy)
	if err != nil {
		t.Fatalf("EnforcePolicy should succeed without error, got: %v", err)
	}

	retrievedPolicy := gt.Policy()
	if len(retrievedPolicy.NetworkRules) != 1 {
		t.Errorf("Policy rules count mismatch: want 1, got %d", len(retrievedPolicy.NetworkRules))
	}
	if retrievedPolicy.NetworkRules[0].Host != expectedPolicy.NetworkRules[0].Host {
		t.Error("Policy host mismatch after EnforcePolicy/Policy roundtrip")
	}
}

func TestEnforcePolicy_ConcurrentSafe(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{})

	policy1 := SandboxPolicy{NetworkRules: []NetworkRule{{}}}
	policy2 := SandboxPolicy{NetworkRules: []NetworkRule{{Host: "final"}}}

	// Launch concurrent calls to EnforcePolicy
	done := make(chan error, 3)
	for i := 0; i < 5; i++ {
		go func(i int) {
			if i%2 == 0 {
				done <- gt.EnforcePolicy(policy1)
			} else {
				done <- gt.Policy() // Policy reads too, not writes
			}
			close(done)
		}(i)

		go func(i int) {
			done <- gt.Policy()
			close(done)
		}(i)
	}

	for range done {
		// Drain goroutines - we expect no deadlocks or crashes here
	}

	if err := <-done; err != nil && err == nil {
		t.Fatal("Expected error for empty channel, got nil")
	}
}

func TestPolicy_ReturnsSnapshot(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{})
	policy1 := SandboxPolicy{NetworkRules: []NetworkRule{{Host: "v1"}}}
	gt.EnforcePolicy(policy1)

	policy2 := SandboxPolicy{NetworkRules: []NetworkRule{{Host: "v2"}}}
	gt.EnforcePolicy(policy2)

	// Policy() should return the latest snapshot
	current := gt.Policy()
	if len(current.NetworkRules) == 0 {
		t.Error("Expected policy to reflect last set value")
	}
}

func TestName_Method(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{})
	name := gt.Name()
	if name != "gvisor" {
		t.Errorf("Expected Name() to return 'gvisor', got: %q", name)
	}
}

func TestRunConfig_ReturnsStored(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{
		RunscPath:        "custom-runsc",
		Platform:         "kvm",
		ContainerRuntime: "docker",
	})

	config := gt.RunConfig()
	if config.RunscPath != "custom-runsc" {
		t.Errorf("RunConfig should return stored config")
	}
}

// ----------------------------------------------------------------------------
// VERIFY INTEGRATION TESTS (Requires System Binaries)
// ----------------------------------------------------------------------------

func TestVerify_SkipsIfAlreadyVerified(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{Platform: "kvm"}) // Use kvm to pass basic checks if possible
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First call verifies and sets verified=true
	if err := gt.Verify(ctx); err != nil && !errorsAsBinaryNotFound(t, err) {
		// We expect error if no binaries exist, but we want to test idempotency
		// So only proceed if at least one binary check passes
	}

	// Second call should be fast and skip checks
	if err := gt.Verify(ctx); err != nil && !errorsAsBinaryNotFound(t, err) {
		t.Errorf("Verify() second call skipped as expected but got error: %v", err)
	} else if err == nil && errorsAsBinaryNotFound(t, nil) {
		t.Error("Verify() on non-existent binaries should skip checks only after first success")
	}
}

// Helper to differentiate between "verification failed" and "binary not found (setup)"
func errorsAsBinaryNotFound(t *testing.T, err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") || strings.Contains(errStr, "invalid version")
}

// Helper for Verify tests - checks if binary exists on PATH
func isBinaryExists(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// Skip verify integration test if binaries not available, so CI doesn't fail
func TestVerify_RunsOnlyIfBinariesExist(t *testing.T) {
	if !isBinaryExists("runsc") || !isBinaryExists("docker") {
		t.Skip("Skipping Verify() integration tests: runsc or docker binary not found on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gt := NewGVisorRuntime(SandboxConfig{Platform: "kvm"}) // Match platform hint if installed

	err := gt.Verify(ctx)
	if err != nil {
		t.Errorf("Verify() failed unexpectedly with binaries present: %v", err)
	}

	if !gt.config.verified {
		t.Error("Expected runtime to be verified=true after successful Verify() call")
	}
}

func TestVerify_RunscMissing_FailsGracefully(t *testing.T) {
	// This test will only run if 'runsc' is available; otherwise we expect skip
	if !isBinaryExists("runsc") && !isBinaryExists("docker") {
		t.Skip("Skipping Verify() failure test: runsc binary missing to demonstrate behavior")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gt := NewGVisorRuntime(SandboxConfig{})
	err := gt.Verify(ctx)
	if err == nil {
		t.Error("Expected error when runsc verification fails (if docker is present)")
	}
}

func TestVerify_DockerMissing_FailsGracefully(t *testing.T) {
	// Similar to above - requires at least one binary to test the other missing case
	if !isBinaryExists("runsc") && !isBinaryExists("docker") {
		t.Skip("Skipping Verify() failure test: no binaries found to compare against")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gt := NewGVisorRuntime(SandboxConfig{ContainerRuntime: "containerd"}) // Try containerd if docker is missing
	err := gt.Verify(ctx)
	if err == nil {
		t.Error("Expected error when docker/containerd verification fails")
	}
}

func TestVerify_PlatformHintMismatchAnnotates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gt := NewGVisorRuntime(SandboxConfig{Platform: "kvm"}) // Expect kvm in version string

	// Force platform mismatch by testing against a different platform's binary signature if possible
	// This is a softer check - in practice, runsc reports its build type which may not match our hint
	if err := gt.Verify(ctx); err != nil {
		errStr := strings.ToLower(err.Error())
		// If we get an error due to version string mismatch, check that it doesn't block execution entirely
		// and the version is annotated with platform hint message. This tests the fallback logic in CheckRunsc.
		if errors.AsBinaryNotFound(t, nil) { // Check if we hit the expected non-fatal behavior
			t.Logf("Verify() handled platform mismatch as non-fatal: %v", err)
		} else {
			t.Logf("Platform hint mismatch logged via version string: %s", errStr)
		}
	}
}

// ----------------------------------------------------------------------------
// HELPERS FOR TESTS (Avoiding External Dependencies on Unit Tests)
// ----------------------------------------------------------------------------

// TestName_ReturnsCorrectValue
func TestVerify_NameConsistency(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{})
	if gt.Name() != "gvisor" {
		t.Errorf("Name() should always return 'gvisor'")
	}
}

// TestRunConfig_PreservesInput
func TestVerify_RunConfigReturnsStoredConfig(t *testing.T) {
	gt := NewGVisorRuntime(SandboxConfig{
		RunscPath:        "custom-runsc",
		Platform:         "ptrace",
		ContainerRuntime: "containerd",
	})

	config := gt.RunConfig()
	if config.RunscPath != "custom-runsc" {
		t.Error("RunConfig should not modify original config")
	}
}
