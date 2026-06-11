package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestManifestResolution validates that manifest loading, parsing, and
// validation work correctly at the manifest seam.
func TestManifestResolution(t *testing.T) {
	t.Run("valid manifest resolves successfully", func(t *testing.T) {
		env := NewTestEnvironment(t)
		m := ValidManifest().Build()
		m.Policy.Permissions.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "${workspace}/**"}}

		if err := m.Validate(); err != nil {
			t.Fatalf("valid manifest should validate: %v", err)
		}

		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := config.SaveYAML(manifestPath, m); err != nil {
			t.Fatalf("failed to save manifest: %v", err)
		}

		data, err := os.ReadFile(filepath.Clean(manifestPath))
		if err != nil {
			t.Fatalf("failed to read manifest: %v", err)
		}
		var loaded config.ManifestSpec
		if err := yaml.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("failed to decode manifest: %v", err)
		}

		AssertNormalizedFileSystemPermissionsEqual(t, loaded.Policy.Permissions.FileSystem, []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "${workspace}/**"}})

		resolved := permissions.ResolveEffective(nil, &loaded.Policy.Permissions)
		AssertNormalizedFileSystemPermissionsEqual(t, resolved.FileSystem, []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "${workspace}/**"}})
	})

	t.Run("manifest defaults are applied correctly", func(t *testing.T) {
		m := NewManifestBuilder().Build()
		if err := m.Validate(); err != nil {
			t.Fatalf("manifest with defaults should validate: %v", err)
		}
		if m.Image != "test-image:latest" {
			t.Errorf("image should have default, got %s", m.Image)
		}
		if m.Runtime != "gvisor" {
			t.Errorf("runtime should have default, got %s", m.Runtime)
		}
	})

	t.Run("resolved manifest state is stable", func(t *testing.T) {
		m1 := ValidManifest().Build()
		m2 := ValidManifest().Build()
		AssertNormalizedFileSystemPermissionsEqual(t, m1.Policy.Permissions.FileSystem, m2.Policy.Permissions.FileSystem)
	})
}

// TestManifestValidationRejection validates that malformed or incomplete
// manifest inputs fail at the right boundary.
func TestManifestValidationRejection(t *testing.T) {
	t.Run("missing image rejection", func(t *testing.T) {
		m := InvalidManifestMissingImage().Build()
		if err := m.Validate(); err == nil {
			t.Error("manifest without image should fail validation")
		}
	})

	t.Run("wrong runtime rejection", func(t *testing.T) {
		m := InvalidManifestWrongRuntime().Build()
		if err := m.Validate(); err == nil {
			t.Error("manifest with wrong runtime should fail validation")
		}
	})

	t.Run("invalid filesystem permission rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Policy.Permissions.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "../escape"}}
		if err := m.Validate(); err == nil {
			t.Error("manifest with invalid filesystem permission should fail validation")
		}
	})

	t.Run("invalid network permission rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Policy.Permissions.Network = []permissions.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "", Port: 443}}
		if err := m.Validate(); err == nil {
			t.Error("manifest with invalid network permission should fail validation")
		}
	})

	t.Run("invalid capability selector rejection", func(t *testing.T) {
		m := ValidManifest().Build()
		m.Agent = &agentspec.AgentRuntimeSpec{
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
		m.Policy.Permissions.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "${workspace}/**"}}

		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := config.SaveYAML(manifestPath, m); err != nil {
			t.Fatalf("failed to save manifest: %v", err)
		}
		data, err := os.ReadFile(filepath.Clean(manifestPath))
		if err != nil {
			t.Fatalf("failed to read manifest: %v", err)
		}
		var loaded config.ManifestSpec
		if err := yaml.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("failed to decode manifest: %v", err)
		}

		resolved := permissions.ResolveEffective(nil, &loaded.Policy.Permissions)

		manager, err := authorization.NewPermissionManager(env.WorkspacePath, &resolved, env.AuditSink, nil)
		if err != nil {
			t.Fatalf("failed to build permission manager: %v", err)
		}

		testFile := filepath.Join(env.WorkspacePath, "allowed.txt")
		if err := fs.WriteFileSecure(testFile, []byte("content")); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		if err := manager.CheckFileAccess(context.Background(), "manifest-agent", permissions.FileSystemRead, testFile); err != nil {
			t.Fatalf("expected manifest-derived read permission to allow access: %v", err)
		}
		if err := manager.CheckFileAccess(context.Background(), "manifest-agent", permissions.FileSystemWrite, testFile); err == nil {
			t.Fatal("expected manifest-derived permissions to deny write access")
		}
	})
}
