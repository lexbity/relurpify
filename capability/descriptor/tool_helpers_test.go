package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func TestToolCapabilityID(t *testing.T) {
	t.Run("nil tool yields empty id", func(t *testing.T) {
		assert.Empty(t, ToolCapabilityID(nil))
	})
	t.Run("bare tool derives from trimmed name", func(t *testing.T) {
		tool := &bareTool{name: "  file-read  "}
		assert.Equal(t, "tool:file-read", ToolCapabilityID(tool))
	})
	t.Run("identity provider wins", func(t *testing.T) {
		tool := &stubTool{bareTool: bareTool{name: "fs"}, capabilityID: " custom.cap "}
		assert.Equal(t, "custom.cap", ToolCapabilityID(tool))
	})
	t.Run("blank provider id falls back to name", func(t *testing.T) {
		tool := &stubTool{bareTool: bareTool{name: "fs"}, capabilityID: "   "}
		assert.Equal(t, "tool:fs", ToolCapabilityID(tool))
	})
}

func TestToolVersion(t *testing.T) {
	assert.Empty(t, ToolVersion(nil))
	assert.Empty(t, ToolVersion(&bareTool{name: "x"}), "bare tool has no version provider")
	assert.Equal(t, "2.1.0", ToolVersion(&stubTool{version: " 2.1.0 "}))
	assert.Empty(t, ToolVersion(&stubTool{version: "   "}), "whitespace version normalizes to empty")
}

func TestToolCapabilitySource(t *testing.T) {
	t.Run("nil tool defaults to builtin", func(t *testing.T) {
		assert.Equal(t, CapabilitySource{Scope: classification.CapabilityScopeBuiltin}, ToolCapabilitySource(nil))
	})
	t.Run("bare tool defaults to builtin", func(t *testing.T) {
		assert.Equal(t, CapabilitySource{Scope: classification.CapabilityScopeBuiltin}, ToolCapabilitySource(&bareTool{name: "x"}))
	})
	t.Run("declared source is preserved", func(t *testing.T) {
		src := CapabilitySource{ProviderID: "prov", Scope: classification.CapabilityScopeProvider}
		assert.Equal(t, src, ToolCapabilitySource(&stubTool{source: src}))
	})
	t.Run("missing scope defaults to builtin", func(t *testing.T) {
		src := CapabilitySource{ProviderID: "prov"}
		got := ToolCapabilitySource(&stubTool{source: src})
		assert.Equal(t, classification.CapabilityScopeBuiltin, got.Scope)
		assert.Equal(t, "prov", got.ProviderID)
	})
}

func TestToolCapabilityRuntimeFamily(t *testing.T) {
	cases := []struct {
		name string
		tool ports.Tool
		want agentspec.CapabilityRuntimeFamily
	}{
		{"nil tool", nil, agentspec.CapabilityRuntimeFamilyLocalTool},
		{"bare tool", &bareTool{name: "x"}, agentspec.CapabilityRuntimeFamilyLocalTool},
		{"declared family", &stubTool{runtimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic}, agentspec.CapabilityRuntimeFamilyRelurpic},
		{"provider scope without declared family", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeProvider}}, agentspec.CapabilityRuntimeFamilyProvider},
		{"remote scope without declared family", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeRemote}}, agentspec.CapabilityRuntimeFamilyProvider},
		{"workspace scope defaults to local", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeWorkspace}}, agentspec.CapabilityRuntimeFamilyLocalTool},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ToolCapabilityRuntimeFamily(tc.tool))
		})
	}
	t.Run("empty declared family falls through to scope", func(t *testing.T) {
		tool := &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeRemote}}
		assert.Equal(t, agentspec.CapabilityRuntimeFamilyProvider, ToolCapabilityRuntimeFamily(tool))
	})
}

func TestToolTrustClass(t *testing.T) {
	cases := []struct {
		name string
		tool ports.Tool
		want agentspec.TrustClass
	}{
		{"nil tool", nil, agentspec.TrustClassBuiltinTrusted},
		{"bare tool", &bareTool{name: "x"}, agentspec.TrustClassBuiltinTrusted},
		{"declared trust", &stubTool{trust: agentspec.TrustClassRemoteApproved}, agentspec.TrustClassRemoteApproved},
		{"workspace scope", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeWorkspace}}, agentspec.TrustClassWorkspaceTrusted},
		{"provider scope", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeProvider}}, agentspec.TrustClassProviderLocalUntrusted},
		{"remote scope", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeRemote}}, agentspec.TrustClassRemoteDeclared},
		{"builtin scope", &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeBuiltin}}, agentspec.TrustClassBuiltinTrusted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ToolTrustClass(tc.tool))
		})
	}
	t.Run("empty declared trust falls through to scope", func(t *testing.T) {
		tool := &stubTool{source: CapabilitySource{Scope: classification.CapabilityScopeWorkspace}}
		assert.Equal(t, agentspec.TrustClassWorkspaceTrusted, ToolTrustClass(tool))
	})
}

func TestToolCapabilityTags(t *testing.T) {
	t.Run("nil tool yields nil", func(t *testing.T) {
		assert.Nil(t, ToolCapabilityTags(nil))
	})
	t.Run("tags are normalized, deduped and reserved tags dropped", func(t *testing.T) {
		tool := &bareTool{tags: []string{" Fast ", "fast", "READ_ONLY", "network", "My_Tag"}}
		got := ToolCapabilityTags(tool)
		// "read-only" and "network" are reserved security vocabulary;
		// duplicates collapse; casing and underscores fold.
		assert.Equal(t, []string{"fast", "my-tag"}, got)
	})
	t.Run("all reserved tags yields nil", func(t *testing.T) {
		tool := &bareTool{tags: []string{"destructive", "execute", "builtin-trusted", "process-spawn"}}
		assert.Nil(t, ToolCapabilityTags(tool))
	})
}

func TestToolEffectClasses(t *testing.T) {
	t.Run("nil tool yields nil", func(t *testing.T) {
		assert.Nil(t, ToolEffectClasses(nil))
	})
	t.Run("bare tool without permissions yields nil", func(t *testing.T) {
		assert.Nil(t, ToolEffectClasses(&bareTool{name: "x"}))
	})
	t.Run("filesystem write implies filesystem mutation", func(t *testing.T) {
		tool := &permissionedTool{bareTool: bareTool{perms: permSet(true, 0, 0)}}
		assert.Equal(t, []classification.EffectClass{classification.EffectClassFilesystemMutation}, ToolEffectClasses(tool))
	})
	t.Run("executables imply process spawn", func(t *testing.T) {
		tool := &permissionedTool{bareTool: bareTool{perms: permSet(false, 2, 0)}}
		assert.Equal(t, []classification.EffectClass{classification.EffectClassProcessSpawn}, ToolEffectClasses(tool))
	})
	t.Run("network implies egress and external state", func(t *testing.T) {
		tool := &permissionedTool{bareTool: bareTool{perms: permSet(false, 0, 1)}}
		assert.Equal(t, []classification.EffectClass{
			classification.EffectClassExternalState,
			classification.EffectClassNetworkEgress,
		}, ToolEffectClasses(tool))
	})
	t.Run("combined permissions union", func(t *testing.T) {
		tool := &permissionedTool{bareTool: bareTool{perms: permSet(true, 1, 1)}}
		assert.Equal(t, []classification.EffectClass{
			classification.EffectClassExternalState,
			classification.EffectClassFilesystemMutation,
			classification.EffectClassNetworkEgress,
			classification.EffectClassProcessSpawn,
		}, ToolEffectClasses(tool))
	})
	t.Run("session affinity provider implies session creation", func(t *testing.T) {
		tool := &affinityOnlyTool{bareTool: bareTool{name: "spawner"}, affinity: "sess-1"}
		assert.Equal(t, []classification.EffectClass{classification.EffectClassSessionCreation}, ToolEffectClasses(tool))
	})
	t.Run("declared effect classes are normalized", func(t *testing.T) {
		tool := &stubTool{effects: []classification.EffectClass{
			" FILESYSTEM_MUTATION ", "process-spawn", "", "process_spawn",
		}}
		// snake_case folds to the canonical kebab vocabulary and duplicates
		// collapse; only empty strings are dropped.
		assert.Equal(t, []classification.EffectClass{
			classification.EffectClassFilesystemMutation,
			classification.EffectClassProcessSpawn,
		}, ToolEffectClasses(tool))
	})
}

func TestToolInputSchema(t *testing.T) {
	t.Run("nil tool yields nil", func(t *testing.T) {
		assert.Nil(t, ToolInputSchema(nil))
	})
	t.Run("schema built from parameters", func(t *testing.T) {
		tool := &bareTool{params: []ports.ToolParameter{
			{Name: "force", Type: "boolean", Description: " overwrite "},
			{Name: "path", Type: "string", Description: "target", Required: true, Default: "/tmp"},
			{Name: "mystery", Required: true},
		}}
		schema := ToolInputSchema(tool)
		require.NotNil(t, schema)
		assert.Equal(t, "object", schema.Type)
		require.Len(t, schema.Properties, 3)

		force := schema.Properties["force"]
		require.NotNil(t, force)
		assert.Equal(t, "boolean", force.Type)
		assert.Equal(t, "overwrite", force.Description, "descriptions are trimmed")

		path := schema.Properties["path"]
		require.NotNil(t, path)
		assert.Equal(t, "/tmp", path.Default)

		mystery := schema.Properties["mystery"]
		require.NotNil(t, mystery)
		assert.Equal(t, "string", mystery.Type, "undeclared type defaults to string")

		// Required list is sorted regardless of declaration order.
		assert.Equal(t, []string{"mystery", "path"}, schema.Required)
	})
	t.Run("tool without parameters yields empty object schema", func(t *testing.T) {
		schema := ToolInputSchema(&bareTool{name: "x"})
		require.NotNil(t, schema)
		assert.Equal(t, "object", schema.Type)
		assert.Empty(t, schema.Properties)
		assert.Empty(t, schema.Required)
	})
}
