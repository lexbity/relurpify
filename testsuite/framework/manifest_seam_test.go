package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestManifestResolution validates that manifest loading, parsing, and
// validation work correctly at the manifest seam.
func TestManifestResolution(t *testing.T) {
	t.Run("valid manifest resolves successfully", func(t *testing.T) {
		env := NewTestEnvironment(t)
		m := ValidManifest().Build()
		m.Metadata.Name = "manifest-resolution-agent"
		m.Metadata.Version = "1.0.0"
		m.Spec.Policy.Permissions.FileSystem = []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "${workspace}/**"}}

		if err := m.Validate(); err != nil {
			t.Fatalf("valid manifest should validate: %v", err)
		}

		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := cfgload.SaveAgentManifest(manifestPath, m); err != nil {
			t.Fatalf("failed to save manifest: %v", err)
		}

		loaded, err := cfgload.LoadAgentManifest(manifestPath)
		if err != nil {
			t.Fatalf("failed to load manifest: %v", err)
		}
		if loaded == nil {
			t.Fatal("loaded manifest should not be nil")
		}

		AssertNormalizedFileSystemPermissionsEqual(t, loaded.Spec.Permissions.FileSystem, []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "${workspace}/**"}})

		resolved, err := cfgload.ResolveEffectivePermissions(env.WorkspacePath, loaded)
		if err != nil {
			t.Fatalf("resolve effective permissions failed: %v", err)
		}
		AssertNormalizedFileSystemPermissionsEqual(t, resolved.FileSystem, []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "${workspace}/**"}})
	})

	t.Run("defaults are applied correctly", func(t *testing.T) {
		m := NewManifestBuilder().WithName("test-agent").WithVersion("1.0.0").Build()
		if err := m.Validate(); err != nil {
			t.Fatalf("manifest with defaults should validate: %v", err)
		}
		if m.APIVersion != "relurpify/v1alpha1" {
			t.Errorf("apiVersion should have default, got %s", m.APIVersion)
		}
		if m.Kind != "AgentManifest" {
			t.Errorf("kind should have default, got %s", m.Kind)
		}
	})

	t.Run("resolved manifest state is stable", func(t *testing.T) {
		m1 := ValidManifest().Build()
		m2 := ValidManifest().Build()
		AssertNormalizedFileSystemPermissionsEqual(t, m1.Spec.Policy.Permissions.FileSystem, m2.Spec.Policy.Permissions.FileSystem)
	})
}

// TestManifestValidationRejection validates that malformed or incomplete
// manifest inputs fail at the right boundary.
func TestManifestValidationRejection(t *testing.T) {
	t.Run("missing apiVersion rejection", func(t *testing.T) {
		m := InvalidManifestMissingAPIVersion().Build()
		if err := m.Validate(); err == nil {
			t.Error("manifest without apiVersion should fail validation")
		}
	})

	t.Run("missing kind rejection", func(t *testing.T) {
		m := InvalidManifestMissingKind().Build()
		if err := m.Validate(); err == nil {
			t.Error("manifest without kind should fail validation")
		}
	})

	t.Run("missing metadata name rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Metadata.Name = ""
		if err := m.Validate(); err == nil {
			t.Error("manifest without metadata name should fail validation")
		}
	})

	t.Run("invalid filesystem permission rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Spec.Policy.Permissions.FileSystem = []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "../escape"}}
		if err := m.Validate(); err == nil {
			t.Error("manifest with invalid filesystem permission should fail validation")
		}
	})

	t.Run("invalid network permission rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Spec.Policy.Permissions.Network = []contracts.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "", Port: 443}}
		if err := m.Validate(); err == nil {
			t.Error("manifest with invalid network permission should fail validation")
		}
	})

	t.Run("invalid capability selector rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Spec.Agent = &agentspec.AgentRuntimeSpec{
			Mode:                agentspec.AgentModePrimary,
			Model:               agentspec.AgentModelConfig{Provider: "test-provider", Name: "test-model"},
			AllowedCapabilities: []agentspec.CapabilitySelector{{}},
		}
		if err := m.Validate(); err == nil {
			t.Error("manifest with invalid capability selector should fail validation")
		}
	})
}

// TestManifestPermissionPropagation validates that manifest permissions become
// the effective permissions consumed by the runtime permission manager.
func TestManifestPermissionPropagation(t *testing.T) {
	t.Run("resolved manifest permissions drive authorization", func(t *testing.T) {
		env := NewTestEnvironment(t)
		m := ValidManifest().Build()
		m.Metadata.Name = "manifest-propagation-agent"
		m.Metadata.Version = "1.0.0"
		m.Spec.Policy.Permissions.FileSystem = []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "${workspace}/**"}}

		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := cfgload.SaveAgentManifest(manifestPath, m); err != nil {
			t.Fatalf("failed to save manifest: %v", err)
		}
		loaded, err := cfgload.LoadAgentManifest(manifestPath)
		if err != nil {
			t.Fatalf("failed to load manifest: %v", err)
		}

		resolved, err := cfgload.ResolveEffectivePermissions(env.WorkspacePath, loaded)
		if err != nil {
			t.Fatalf("resolve effective permissions failed: %v", err)
		}

		manager, err := authorization.NewPermissionManager(env.WorkspacePath, &resolved, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to build permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "allowed.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		if err := manager.CheckFileAccess(context.Background(), "manifest-agent", contracts.FileSystemRead, testFile); err != nil {
			t.Fatalf("expected manifest-derived read permission to allow access: %v", err)
		}
		if err := manager.CheckFileAccess(context.Background(), "manifest-agent", contracts.FileSystemWrite, testFile); err == nil {
			t.Fatal("expected manifest-derived permissions to deny write access")
		}
	})
}
