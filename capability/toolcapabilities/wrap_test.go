package toolcapabilities

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
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
