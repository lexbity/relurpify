package cfgload

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestValidateToolManifestRejectsMissingRequiredFields(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name: "bad_tool",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "family required")
	require.Contains(t, err.Error(), "description required")
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestValidateToolManifestAcceptsGoNativeManifest(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "ok_tool",
		Family:      "search",
		Description: "ok",
		Parameters: []contracts.ToolParameter{
			{Name: "query", Type: "string", Required: true},
		},
		Execution: contracts.ToolManifestExecution{
			Backend:        contracts.ToolBackendGoNative,
			Implementation: "ok_tool",
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"read_only"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}
