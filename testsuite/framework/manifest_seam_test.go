package framework

import (
	"context"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	manifestWorkspaceGlob = "${workspace}/**"
	manifestKindName      = "AgentManifest"
)

// TestManifestResolution validates that manifest loading, parsing, and
// validation work correctly at the manifest seam.
func TestManifestResolution(t *testing.T) {
	t.Run("valid manifest resolves successfully", func(t *testing.T) {
		env := NewTestEnvironment(t)
		doc := ValidDocument().Build()
		if doc == nil {
			t.Fatal("valid document should not be nil")
		}
		permissionsNode, ok := doc.Section("permissions")
		if !ok {
			t.Fatal("permissions section missing")
		}
		var permSpec permissions.PermissionSet
		if err := permissionsNode.Decode(&permSpec); err != nil {
			t.Fatalf("decode permissions: %v", err)
		}
		permSpec.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: manifestWorkspaceGlob}}
		var updated yaml.Node
		if err := updated.Encode(permSpec); err != nil {
			t.Fatalf("encode permissions: %v", err)
		}
		doc.Spec["permissions"] = updated
		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := config.SaveYAML(manifestPath, doc); err != nil {
			t.Fatalf("failed to save document: %v", err)
		}
		snapshot, err := config.LoadDocument(manifestPath)
		if err != nil {
			t.Fatalf("failed to load document: %v", err)
		}
		resolved, err := config.ResolveEffectiveAgentContract(env.WorkspacePath, snapshot.Document, config.ResolveOptions{})
		if err != nil {
			t.Fatalf("resolve effective contract: %v", err)
		}
		AssertNormalizedFileSystemPermissionsEqual(t, resolved.Permissions.FileSystem, []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: manifestWorkspaceGlob}})
	})

	t.Run("manifest defaults are applied correctly", func(t *testing.T) {
		doc := NewDocumentBuilder().Build()
		if doc == nil {
			t.Fatal("document builder should return a document")
		}
		if doc.APIVersion == "" || doc.Kind == "" {
			t.Fatalf("document missing envelope fields: %#v", doc)
		}
	})

	t.Run("resolved manifest state is stable", func(t *testing.T) {
		m1 := ValidDocument().Build()
		m2 := ValidDocument().Build()
		p1, _ := m1.Section("permissions")
		p2, _ := m2.Section("permissions")
		var ps1, ps2 permissions.PermissionSet
		_ = p1.Decode(&ps1)
		_ = p2.Decode(&ps2)
		AssertNormalizedFileSystemPermissionsEqual(t, ps1.FileSystem, ps2.FileSystem)
	})
}

// TestManifestValidationRejection validates that malformed or incomplete
// manifest inputs fail at the right boundary.
func TestManifestValidationRejection(t *testing.T) {
	t.Run("missing image rejection", func(t *testing.T) {
		m := InvalidDocumentMissingMetadata().Build()
		if m != nil && m.Metadata.Name != "" {
			t.Error("document without metadata should fail helper validation")
		}
	})

	t.Run("wrong runtime rejection", func(t *testing.T) {
		m := InvalidDocumentWrongKind().Build()
		if m == nil || m.Kind == manifestKindName {
			t.Error("document with wrong kind should fail helper validation")
		}
	})

	t.Run("invalid filesystem permission rejection", func(t *testing.T) {
		m := ValidDocument().Build()
		permNode, _ := m.Section("permissions")
		var permSpec permissions.PermissionSet
		_ = permNode.Decode(&permSpec)
		permSpec.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "../escape"}}
		if err := permSpec.Validate(); err == nil {
			t.Error("document with invalid filesystem permission should fail permission validation")
		}
	})

	t.Run("invalid network permission rejection", func(t *testing.T) {
		m := ValidDocument().Build()
		permNode, _ := m.Section("permissions")
		var permSpec permissions.PermissionSet
		_ = permNode.Decode(&permSpec)
		permSpec.Network = []permissions.NetworkPermission{{Direction: frameworkNetEgress, Protocol: frameworkExampleProtocol, Host: "", Port: 443}}
		if err := permSpec.Validate(); err == nil {
			t.Error("document with invalid network permission should fail permission validation")
		}
	})

	t.Run("invalid capability selector rejection", func(t *testing.T) {
		agentSpec := &agentspec.AgentRuntimeSpec{
			Mode:                agentspec.AgentModePrimary,
			Model:               agentspec.AgentModelConfig{Provider: "test-provider", Name: "test-model"},
			AllowedCapabilities: []agentspec.CapabilitySelector{{}},
		}
		if err := agentSpec.Validate(); err == nil {
			t.Error("document with invalid capability selector should fail agent validation")
		}
	})
}

// TestManifestPermissionPropagation validates that manifest permissions become
// the effective permissions consumed by the runtime permission manager.
func TestManifestPermissionPropagation(t *testing.T) {
	t.Run("resolved manifest permissions drive authorization", func(t *testing.T) {
		env := NewTestEnvironment(t)
		m := ValidDocument().Build()
		permNode, _ := m.Section("permissions")
		var permSpec permissions.PermissionSet
		_ = permNode.Decode(&permSpec)
		permSpec.FileSystem = []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: manifestWorkspaceGlob}}
		var updated yaml.Node
		_ = updated.Encode(permSpec)
		m.Spec["permissions"] = updated

		manifestPath := filepath.Join(env.WorkspacePath, "agent.yaml")
		if err := config.SaveYAML(manifestPath, m); err != nil {
			t.Fatalf("failed to save document: %v", err)
		}
		snapshot, err := config.LoadDocument(manifestPath)
		if err != nil {
			t.Fatalf("failed to load document: %v", err)
		}
		resolved, err := config.ResolveEffectiveAgentContract(env.WorkspacePath, snapshot.Document, config.ResolveOptions{})
		if err != nil {
			t.Fatalf("resolve effective contract: %v", err)
		}

		manager, err := authorization.NewPermissionManager(env.WorkspacePath, &resolved.Permissions, env.AuditSink, nil)
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
