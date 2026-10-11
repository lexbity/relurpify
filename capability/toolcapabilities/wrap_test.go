package toolcapabilities

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// TestWrapWithCapabilityNormalizesSnakeCaseClasses is the regression test for
// the class-string vocabulary split: manifests declare snake_case classes while
// the canonical constants are kebab-case. The load boundary must normalize.
//
// The manifest spellings are derived from the canonical constants so the test
// has a single source of truth for each class name.
func TestWrapWithCapabilityNormalizesSnakeCaseClasses(t *testing.T) {
	snake := func(canonical string) string { return strings.ReplaceAll(canonical, "-", "_") }

	tool := wrapWithCapability(
		&testTool{name: "shell_tool"},
		ports.ToolManifest{
			Capability: ports.ToolManifestCapability{
				TrustClass: snake(string(agentspec.TrustClassBuiltinTrusted)),
				RiskClass:  []string{snake(string(risk.RiskClassReadOnly))},
				EffectClass: []string{
					snake(string(classification.EffectClassProcessSpawn)),
					snake(string(classification.EffectClassFilesystemMutation)),
				},
			},
		},
	)

	trust, ok := tool.(interface{ TrustClass() agentspec.TrustClass })
	require.True(t, ok, "wrapped tool must implement TrustClass provider")
	require.Equal(t, agentspec.TrustClassBuiltinTrusted, trust.TrustClass())

	riskProv, ok := tool.(interface{ RiskClasses() []risk.RiskClass })
	require.True(t, ok, "wrapped tool must implement RiskClasses provider")
	require.Equal(t, []risk.RiskClass{risk.RiskClassReadOnly}, riskProv.RiskClasses())

	effectProv, ok := tool.(interface {
		EffectClasses() []classification.EffectClass
	})
	require.True(t, ok, "wrapped tool must implement EffectClasses provider")
	require.Equal(t,
		[]classification.EffectClass{
			classification.EffectClassProcessSpawn,
			classification.EffectClassFilesystemMutation,
		},
		effectProv.EffectClasses(),
	)
}

// scopeAwareTool mimics the concrete file tools (scopedFileTool): the sandbox
// scope lands on the tool itself and its enforcement reads its own field.
type scopeAwareTool struct {
	ports.Tool
	name  string
	scope *permissions.FileScopePolicy
}

func (t *scopeAwareTool) Name() string { return t.name }

func (t *scopeAwareTool) SetSandboxScope(scope *permissions.FileScopePolicy) {
	t.scope = scope
}

// TestWithCapabilityUnwrapReachesConcreteTool pins the decoration contract:
// the manifest wrapper exposes the decorated tool so registry stamping can
// reach the concrete implementation (F1: the wrapper silently swallowed the
// sandbox-scope stamp).
func TestWithCapabilityUnwrapReachesConcreteTool(t *testing.T) {
	inner := &scopeAwareTool{name: "file_read"}
	wrapped := wrapWithCapability(inner, ports.ToolManifest{
		Capability: ports.ToolManifestCapability{
			TrustClass: string(agentspec.TrustClassBuiltinTrusted),
			RiskClass:  []string{string(risk.RiskClassReadOnly)},
		},
	})

	unwrapper, ok := wrapped.(ports.ToolUnwrapper)
	require.True(t, ok, "withCapability must implement ports.ToolUnwrapper")
	require.Same(t, inner, unwrapper.Unwrap())
}

// TestWithCapabilityForwardsSandboxScope pins the depth-one stamp forwarding:
// setting the scope through the wrapper lands on the decorated tool's field,
// because that is where the tool's own enforcement reads it.
func TestWithCapabilityForwardsSandboxScope(t *testing.T) {
	inner := &scopeAwareTool{name: "file_read"}
	wrapped := wrapWithCapability(inner, ports.ToolManifest{
		Capability: ports.ToolManifestCapability{
			TrustClass: string(agentspec.TrustClassBuiltinTrusted),
			RiskClass:  []string{string(risk.RiskClassReadOnly)},
		},
	})

	aware, ok := wrapped.(interface {
		SetSandboxScope(*permissions.FileScopePolicy)
	})
	require.True(t, ok, "withCapability must forward SetSandboxScope")
	scope := permissions.NewFileScopePolicy(t.TempDir(), nil)
	aware.SetSandboxScope(scope)
	require.Same(t, scope, inner.scope)
}
