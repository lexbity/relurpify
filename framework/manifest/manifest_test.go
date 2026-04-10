package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers ----

// validManifest builds a fully valid AgentManifest for use as a baseline.
func validManifest() *AgentManifest {
	return &AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   ManifestMetadata{Name: "test-agent", Version: "1.0.0"},
		Spec: ManifestSpec{
			Image:   "ghcr.io/test/image:latest",
			Runtime: "gvisor",
			Permissions: core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemRead, Path: "/workspace/**"},
				},
			},
		},
	}
}

// writeManifestYAML writes a raw YAML string to a temp file and returns the path.
func writeManifestYAML(t *testing.T, yaml string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(yaml)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// minimalValidYAML is the smallest YAML that passes Validate.
const minimalValidYAML = `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
spec:
  image: test-image:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: /workspace/**
`

// ---- AgentManifest.Validate ----

func TestAgentManifestValidate_Valid(t *testing.T) {
	require.NoError(t, validManifest().Validate())
}

func TestAgentManifestValidate_MissingAPIVersion(t *testing.T) {
	m := validManifest()
	m.APIVersion = ""
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiVersion")
}

func TestAgentManifestValidate_MissingKind(t *testing.T) {
	m := validManifest()
	m.Kind = ""
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")
}

func TestAgentManifestValidate_MissingName(t *testing.T) {
	m := validManifest()
	m.Metadata.Name = ""
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestAgentManifestValidate_MissingImage(t *testing.T) {
	m := validManifest()
	m.Spec.Image = ""
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestAgentManifestValidate_NonGvisorRuntime(t *testing.T) {
	m := validManifest()
	m.Spec.Runtime = "docker"
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime")
}

func TestAgentManifestValidate_RuntimeCaseInsensitive(t *testing.T) {
	m := validManifest()
	m.Spec.Runtime = "GVisor"
	require.NoError(t, m.Validate())
}

func TestAgentManifestValidate_EmptyRuntimeRejected(t *testing.T) {
	m := validManifest()
	m.Spec.Runtime = ""
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime")
}

func TestAgentManifestValidate_SpecPermissionsInvalid(t *testing.T) {
	m := validManifest()
	// Binary containing a slash is invalid.
	m.Spec.Permissions = core.PermissionSet{
		Executables: []core.ExecutablePermission{
			{Binary: "/usr/bin/go"},
		},
	}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permissions invalid")
}

func TestAgentManifestValidate_NoPermissionsAtAll(t *testing.T) {
	m := validManifest()
	m.Spec.Permissions = core.PermissionSet{} // no scopes
	m.Spec.Defaults = nil
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing permissions")
}

func TestAgentManifestValidate_DefaultsPermissionsOnlySuffices(t *testing.T) {
	m := validManifest()
	m.Spec.Permissions = core.PermissionSet{} // no spec-level scopes
	m.Spec.Defaults = &ManifestDefaults{
		Permissions: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{
				{Action: core.FileSystemRead, Path: "/workspace/**"},
			},
		},
	}
	require.NoError(t, m.Validate())
}

func TestAgentManifestValidate_InvalidDefaultsPermissions(t *testing.T) {
	m := validManifest()
	m.Spec.Permissions = core.PermissionSet{}
	m.Spec.Defaults = &ManifestDefaults{
		Permissions: &core.PermissionSet{
			Executables: []core.ExecutablePermission{
				{Binary: "/usr/bin/go"}, // slash in binary → invalid
			},
		},
	}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaults permissions invalid")
}

func TestAgentManifestValidate_EmptySkillEntry(t *testing.T) {
	m := validManifest()
	m.Spec.Skills = []string{"gocoder", "  ", "pycoder"}
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty entry")
}

func TestAgentManifestValidate_NonEmptySkillsOK(t *testing.T) {
	m := validManifest()
	m.Spec.Skills = []string{"gocoder", "pycoder"}
	require.NoError(t, m.Validate())
}

// ---- LoadAgentManifest / SaveAgentManifest ----

func TestLoadAgentManifest_Valid(t *testing.T) {
	path := writeManifestYAML(t, minimalValidYAML)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "relurpify/v1alpha1", m.APIVersion)
	assert.Equal(t, "test-agent", m.Metadata.Name)
	assert.Equal(t, path, m.SourcePath)
}

func TestLoadAgentManifest_SetsSourcePath(t *testing.T) {
	path := writeManifestYAML(t, minimalValidYAML)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	assert.Equal(t, path, m.SourcePath)
}

func TestLoadAgentManifest_NativeToolCalling(t *testing.T) {
	path := writeManifestYAML(t, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
spec:
  image: test-image:latest
  runtime: gvisor
  agent:
    mode: primary
    model:
      provider: ollama
      name: test-model
    native_tool_calling: false
  permissions:
    filesystem:
      - action: fs:read
        path: /workspace/**
`)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Spec.Agent)
	require.NotNil(t, m.Spec.Agent.NativeToolCalling)
	require.False(t, *m.Spec.Agent.NativeToolCalling)
}

func TestLoadAgentManifest_ToolCallingIntent(t *testing.T) {
	path := writeManifestYAML(t, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
spec:
  image: test-image:latest
  runtime: gvisor
  agent:
    mode: primary
    model:
      provider: ollama
      name: test-model
    tool_calling_intent: prefer_prompt
  policy:
    permissions:
      filesystem:
        - action: fs:read
          path: /workspace/**
`)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	require.NotNil(t, m.Spec.Agent)
	require.Equal(t, core.ToolCallingIntentPreferPrompt, m.Spec.Agent.ToolCallingIntent)
	require.False(t, m.Spec.Agent.NativeToolCallingEnabled())
}

func TestLoadAgentManifest_SplitPolicyShape(t *testing.T) {
	path := writeManifestYAML(t, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: split-agent
spec:
  image: split-image:latest
  runtime: gvisor
  policy:
    permissions:
      filesystem:
        - action: fs:read
          path: /workspace/**
    security:
      run_as_user: 1000
      read_only_root: true
    audit:
      level: verbose
      retention_days: 7
    defaults:
      permissions:
        executables:
          - binary: git
  agent:
    mode: primary
    model:
      provider: ollama
      name: split-model
    tool_calling_intent: prefer_native
`)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	require.NotNil(t, m.Spec.Policy)
	require.Equal(t, "/workspace/**", m.Spec.Permissions.FileSystem[0].Path)
	require.True(t, m.Spec.Security.ReadOnlyRoot)
	require.Equal(t, "verbose", m.Spec.Audit.Level)
	require.NotNil(t, m.Spec.Defaults)
	require.NotNil(t, m.Spec.Defaults.Permissions)
	require.Len(t, m.Spec.Defaults.Permissions.Executables, 1)
	require.NotNil(t, m.Spec.Agent)
	require.Equal(t, core.ToolCallingIntentPreferNative, m.Spec.Agent.ToolCallingIntent)
	require.True(t, m.Spec.Agent.NativeToolCallingEnabled())
}

func TestLoadAgentManifest_LegacyPolicyWarnings(t *testing.T) {
	path := writeManifestYAML(t, minimalValidYAML)
	snapshot, err := LoadAgentManifestSnapshot(path)
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.Warnings)
	require.Contains(t, snapshot.Warnings[0], "spec.policy")
}

func TestLoadAgentManifest_MixedPolicyPrefersNested(t *testing.T) {
	path := writeManifestYAML(t, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: split-agent
spec:
  image: split-image:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: /flat/**
  policy:
    permissions:
      filesystem:
        - action: fs:read
          path: /nested/**
  agent:
    mode: primary
    model:
      provider: ollama
      name: split-model
`)
	m, err := LoadAgentManifest(path)
	require.NoError(t, err)
	require.Len(t, m.Spec.Permissions.FileSystem, 1)
	require.Equal(t, "/nested/**", m.Spec.Permissions.FileSystem[0].Path)
}

func TestLoadAgentManifest_MissingFile(t *testing.T) {
	_, err := LoadAgentManifest("/nonexistent/path/manifest.yaml")
	require.Error(t, err)
}

func TestLoadAgentManifest_InvalidYAML(t *testing.T) {
	path := writeManifestYAML(t, ":::: not valid yaml ::::")
	_, err := LoadAgentManifest(path)
	require.Error(t, err)
}

func TestLoadAgentManifest_ValidationFailure(t *testing.T) {
	// Valid YAML but fails Validate (missing image).
	yaml := `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
spec:
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: /workspace/**
`
	path := writeManifestYAML(t, yaml)
	_, err := LoadAgentManifest(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestLoadAgentManifestSnapshot_FingerprintAndImmutability(t *testing.T) {
	path := writeManifestYAML(t, minimalValidYAML)

	first, err := LoadAgentManifestSnapshot(path)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.Manifest)
	assert.Equal(t, path, first.SourcePath)
	assert.Equal(t, path, first.Manifest.SourcePath)
	assert.False(t, first.LoadedAt.IsZero())

	second, err := LoadAgentManifestSnapshot(path)
	require.NoError(t, err)
	assert.Equal(t, first.Fingerprint, second.Fingerprint)
	assert.True(t, bytes.Equal(first.Fingerprint[:], second.Fingerprint[:]))

	updated := `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: updated-agent
spec:
  image: test-image:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: /workspace/**
`
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))

	third, err := LoadAgentManifestSnapshot(path)
	require.NoError(t, err)
	assert.NotEqual(t, first.Fingerprint, third.Fingerprint)
	assert.Equal(t, "test-agent", first.Manifest.Metadata.Name)
	assert.Equal(t, "updated-agent", third.Manifest.Metadata.Name)
}

func TestSaveAndLoadAgentManifest_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")

	original := validManifest()
	original.Spec.Skills = []string{"gocoder"}
	original.Spec.Resources = ResourceSpec{
		Limits: ResourceLimit{CPU: "500m", Memory: "256Mi"},
	}

	require.NoError(t, SaveAgentManifest(path, original))

	loaded, err := LoadAgentManifest(path)
	require.NoError(t, err)
	assert.Equal(t, original.APIVersion, loaded.APIVersion)
	assert.Equal(t, original.Kind, loaded.Kind)
	assert.Equal(t, original.Metadata.Name, loaded.Metadata.Name)
	assert.Equal(t, original.Spec.Image, loaded.Spec.Image)
	assert.Equal(t, original.Spec.Runtime, loaded.Spec.Runtime)
	assert.Equal(t, original.Spec.Skills, loaded.Spec.Skills)
	assert.Equal(t, original.Spec.Resources.Limits.CPU, loaded.Spec.Resources.Limits.CPU)
	assert.Equal(t, original.Spec.Resources.Limits.Memory, loaded.Spec.Resources.Limits.Memory)
}

func TestSaveAgentManifest_ToNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new-manifest.yaml")

	m := validManifest()
	require.NoError(t, SaveAgentManifest(path, m))

	_, err := os.Stat(path)
	require.NoError(t, err)
}

// ---- MergePermissionSets ----

func TestMergePermissionSets_Empty(t *testing.T) {
	result := MergePermissionSets()
	assert.Empty(t, result.FileSystem)
	assert.Empty(t, result.Executables)
	assert.Empty(t, result.Network)
	assert.Empty(t, result.Capabilities)
	assert.Empty(t, result.IPC)
}

func TestMergePermissionSets_NilInputsSkipped(t *testing.T) {
	result := MergePermissionSets(nil, nil)
	assert.Empty(t, result.FileSystem)
}

func TestMergePermissionSets_SingleSet(t *testing.T) {
	set := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/**"},
		},
		Executables: []core.ExecutablePermission{
			{Binary: "go"},
		},
	}
	result := MergePermissionSets(set)
	require.Len(t, result.FileSystem, 1)
	require.Len(t, result.Executables, 1)
}

func TestMergePermissionSets_DedupFileSystem(t *testing.T) {
	perm := core.FileSystemPermission{Action: core.FileSystemRead, Path: "/workspace/**"}
	a := &core.PermissionSet{FileSystem: []core.FileSystemPermission{perm}}
	b := &core.PermissionSet{FileSystem: []core.FileSystemPermission{perm}}
	result := MergePermissionSets(a, b)
	// Same action+path key: only one entry should appear.
	assert.Len(t, result.FileSystem, 1)
}

func TestMergePermissionSets_DedupExecutables(t *testing.T) {
	perm := core.ExecutablePermission{Binary: "go", Args: []string{"test"}}
	a := &core.PermissionSet{Executables: []core.ExecutablePermission{perm}}
	b := &core.PermissionSet{Executables: []core.ExecutablePermission{perm}}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.Executables, 1)
}

func TestMergePermissionSets_DedupExecutables_HITLFlagDistinguished(t *testing.T) {
	base := core.ExecutablePermission{Binary: "make"}
	withHITL := core.ExecutablePermission{Binary: "make", HITLRequired: true}
	a := &core.PermissionSet{Executables: []core.ExecutablePermission{base}}
	b := &core.PermissionSet{Executables: []core.ExecutablePermission{withHITL}}
	result := MergePermissionSets(a, b)
	// Different keys (one has :hitl suffix), both should appear.
	assert.Len(t, result.Executables, 2)
}

func TestMergePermissionSets_DedupNetwork(t *testing.T) {
	perm := core.NetworkPermission{Direction: "egress", Protocol: "tcp", Host: "api.example.com"}
	a := &core.PermissionSet{Network: []core.NetworkPermission{perm}}
	b := &core.PermissionSet{Network: []core.NetworkPermission{perm}}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.Network, 1)
}

func TestMergePermissionSets_NetworkWithPort_Dedup(t *testing.T) {
	perm := core.NetworkPermission{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443}
	a := &core.PermissionSet{Network: []core.NetworkPermission{perm}}
	b := &core.PermissionSet{Network: []core.NetworkPermission{perm}}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.Network, 1)
	assert.Equal(t, 443, result.Network[0].Port)
}

func TestMergePermissionSets_DedupCapabilities(t *testing.T) {
	perm := core.CapabilityPermission{Capability: "CAP_NET_ADMIN"}
	a := &core.PermissionSet{Capabilities: []core.CapabilityPermission{perm}}
	b := &core.PermissionSet{Capabilities: []core.CapabilityPermission{perm}}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.Capabilities, 1)
}

func TestMergePermissionSets_DedupIPC(t *testing.T) {
	perm := core.IPCPermission{Kind: "socket", Target: "agent-bus"}
	a := &core.PermissionSet{IPC: []core.IPCPermission{perm}}
	b := &core.PermissionSet{IPC: []core.IPCPermission{perm}}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.IPC, 1)
}

func TestMergePermissionSets_UniqueEntriesAllPresent(t *testing.T) {
	a := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/workspace/**"},
		},
	}
	b := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemWrite, Path: "/workspace/**"},
		},
	}
	result := MergePermissionSets(a, b)
	// Different actions → different keys, both retained.
	assert.Len(t, result.FileSystem, 2)
}

func TestMergePermissionSets_HITLRequiredAppended(t *testing.T) {
	a := &core.PermissionSet{
		FileSystem:   []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/**"}},
		HITLRequired: []string{"tool:shell_exec"},
	}
	b := &core.PermissionSet{
		FileSystem:   []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "/out/**"}},
		HITLRequired: []string{"tool:file_write"},
	}
	result := MergePermissionSets(a, b)
	assert.Len(t, result.HITLRequired, 2)
	assert.Contains(t, result.HITLRequired, "tool:shell_exec")
	assert.Contains(t, result.HITLRequired, "tool:file_write")
}

func TestMergePermissionSets_NilMixedWithValid(t *testing.T) {
	set := &core.PermissionSet{
		Executables: []core.ExecutablePermission{{Binary: "go"}},
	}
	result := MergePermissionSets(nil, set, nil)
	require.Len(t, result.Executables, 1)
	assert.Equal(t, "go", result.Executables[0].Binary)
}

// ---- MergeResourceSpecs ----

func TestMergeResourceSpecs_NoOverlays(t *testing.T) {
	base := ResourceSpec{Limits: ResourceLimit{CPU: "500m", Memory: "256Mi"}}
	result := MergeResourceSpecs(base)
	assert.Equal(t, "500m", result.Limits.CPU)
	assert.Equal(t, "256Mi", result.Limits.Memory)
}

func TestMergeResourceSpecs_NilOverlaySkipped(t *testing.T) {
	base := ResourceSpec{Limits: ResourceLimit{CPU: "500m"}}
	result := MergeResourceSpecs(base, nil)
	assert.Equal(t, "500m", result.Limits.CPU)
}

func TestMergeResourceSpecs_OverlayReplacesCPU(t *testing.T) {
	base := ResourceSpec{Limits: ResourceLimit{CPU: "500m", Memory: "256Mi"}}
	overlay := &ResourceSpec{Limits: ResourceLimit{CPU: "1000m"}}
	result := MergeResourceSpecs(base, overlay)
	assert.Equal(t, "1000m", result.Limits.CPU)
	assert.Equal(t, "256Mi", result.Limits.Memory) // unchanged
}

func TestMergeResourceSpecs_EmptyOverlayFieldPreservesBase(t *testing.T) {
	base := ResourceSpec{Limits: ResourceLimit{CPU: "500m", Memory: "256Mi", DiskIO: "100Mi"}}
	// Overlay sets Memory but leaves CPU and DiskIO empty → they keep base values.
	overlay := &ResourceSpec{Limits: ResourceLimit{Memory: "512Mi"}}
	result := MergeResourceSpecs(base, overlay)
	assert.Equal(t, "500m", result.Limits.CPU)
	assert.Equal(t, "512Mi", result.Limits.Memory)
	assert.Equal(t, "100Mi", result.Limits.DiskIO)
}

func TestMergeResourceSpecs_MultipleOverlaysLastWins(t *testing.T) {
	base := ResourceSpec{Limits: ResourceLimit{CPU: "250m"}}
	first := &ResourceSpec{Limits: ResourceLimit{CPU: "500m"}}
	second := &ResourceSpec{Limits: ResourceLimit{CPU: "1000m"}}
	result := MergeResourceSpecs(base, first, second)
	assert.Equal(t, "1000m", result.Limits.CPU)
}

func TestMergeResourceSpecs_NetworkLimit(t *testing.T) {
	base := ResourceSpec{}
	overlay := &ResourceSpec{Limits: ResourceLimit{Network: "10Mbps"}}
	result := MergeResourceSpecs(base, overlay)
	assert.Equal(t, "10Mbps", result.Limits.Network)
}

// ---- ResolveEffectivePermissions ----

func TestResolveEffectivePermissions_NilManifest(t *testing.T) {
	result, err := ResolveEffectivePermissions("agent", nil)
	require.NoError(t, err)
	assert.Empty(t, result.FileSystem)
	assert.Empty(t, result.Executables)
}

func TestResolveEffectivePermissions_SpecOnly(t *testing.T) {
	m := &AgentManifest{
		Spec: ManifestSpec{
			Permissions: core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemRead, Path: "/workspace/**"},
				},
			},
		},
	}
	result, err := ResolveEffectivePermissions("agent", m)
	require.NoError(t, err)
	require.Len(t, result.FileSystem, 1)
	assert.Equal(t, "/workspace/**", result.FileSystem[0].Path)
}

func TestResolveEffectivePermissions_DefaultsOnly(t *testing.T) {
	m := &AgentManifest{
		Spec: ManifestSpec{
			Defaults: &ManifestDefaults{
				Permissions: &core.PermissionSet{
					Executables: []core.ExecutablePermission{{Binary: "go"}},
				},
			},
		},
	}
	result, err := ResolveEffectivePermissions("agent", m)
	require.NoError(t, err)
	require.Len(t, result.Executables, 1)
	assert.Equal(t, "go", result.Executables[0].Binary)
}

func TestResolveEffectivePermissions_DefaultsAndSpecMerged(t *testing.T) {
	m := &AgentManifest{
		Spec: ManifestSpec{
			Permissions: core.PermissionSet{
				Executables: []core.ExecutablePermission{{Binary: "git"}},
			},
			Defaults: &ManifestDefaults{
				Permissions: &core.PermissionSet{
					Executables: []core.ExecutablePermission{{Binary: "go"}},
				},
			},
		},
	}
	result, err := ResolveEffectivePermissions("agent", m)
	require.NoError(t, err)
	binaries := make([]string, len(result.Executables))
	for i, e := range result.Executables {
		binaries[i] = e.Binary
	}
	assert.Contains(t, binaries, "go")
	assert.Contains(t, binaries, "git")
}

func TestResolveEffectivePermissions_SpecDedupsWithDefaults(t *testing.T) {
	perm := core.ExecutablePermission{Binary: "go"}
	m := &AgentManifest{
		Spec: ManifestSpec{
			Permissions: core.PermissionSet{
				Executables: []core.ExecutablePermission{perm},
			},
			Defaults: &ManifestDefaults{
				Permissions: &core.PermissionSet{
					Executables: []core.ExecutablePermission{perm},
				},
			},
		},
	}
	result, err := ResolveEffectivePermissions("agent", m)
	require.NoError(t, err)
	// Same entry in both defaults and spec → deduplicated to one.
	assert.Len(t, result.Executables, 1)
}

// ---- ResolveEffectiveResources ----

func TestResolveEffectiveResources_NilManifest(t *testing.T) {
	result, err := ResolveEffectiveResources("agent", nil)
	require.NoError(t, err)
	assert.Equal(t, ResourceSpec{}, result)
}

func TestResolveEffectiveResources_DefaultsOnly(t *testing.T) {
	m := &AgentManifest{
		Spec: ManifestSpec{
			Defaults: &ManifestDefaults{
				Resources: &ResourceSpec{Limits: ResourceLimit{CPU: "500m", Memory: "256Mi"}},
			},
		},
	}
	result, err := ResolveEffectiveResources("agent", m)
	require.NoError(t, err)
	assert.Equal(t, "500m", result.Limits.CPU)
	assert.Equal(t, "256Mi", result.Limits.Memory)
}

func TestResolveEffectiveResources_SpecOverridesDefaults(t *testing.T) {
	m := &AgentManifest{
		Spec: ManifestSpec{
			Resources: ResourceSpec{Limits: ResourceLimit{CPU: "1000m"}},
			Defaults: &ManifestDefaults{
				Resources: &ResourceSpec{Limits: ResourceLimit{CPU: "500m", Memory: "256Mi"}},
			},
		},
	}
	result, err := ResolveEffectiveResources("agent", m)
	require.NoError(t, err)
	assert.Equal(t, "1000m", result.Limits.CPU)
	assert.Equal(t, "256Mi", result.Limits.Memory) // from defaults, not overridden
}

func TestResolveEffectiveResources_NoDefaultsNoSpec(t *testing.T) {
	m := &AgentManifest{}
	result, err := ResolveEffectiveResources("agent", m)
	require.NoError(t, err)
	assert.Equal(t, ResourceSpec{}, result)
}
