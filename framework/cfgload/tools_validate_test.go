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

// --- v2 flag validation ---

func TestValidateV2TypedFlagEqualsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Parameters: []contracts.ToolParameter{
			{Name: "output_path", Type: "string"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: contracts.FlagStyleEquals,
						Type:  "string",
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagSeparateRepeatPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "cli_echo",
		Family:      "text",
		Description: "echo",
		Parameters: []contracts.ToolParameter{
			{Name: "globs", Type: "array"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
				Flags: map[string]contracts.ToolManifestFlag{
					"glob": {
						Param:  "globs",
						Style:  contracts.FlagStyleSeparate,
						Type:   "array",
						Repeat: true,
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagEmptyStyleDefaults(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "cli_jq",
		Family:      "text",
		Description: "jq",
		Parameters: []contracts.ToolParameter{
			{Name: "output", Type: "string"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"jq"},
				Flags: map[string]contracts.ToolManifestFlag{
					"output": {
						Param: "output",
						// Style intentionally empty — defaults to "equals"
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err, "empty style should be accepted (defaults to equals)")
}

func TestValidateV2BooleanFlagPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "cli_colordiff",
		Family:      "text",
		Description: "colorized diff",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]contracts.ToolManifestFlag{
					"hidden": {
						WhenTrue:  []string{"--hidden"},
						WhenFalse: []string{"--no-hidden"},
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2NoFlagsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "simple",
		Family:      "text",
		Description: "no flags",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"echo"},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2FlagMixedBooleanAndParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "mixed forms",
		Parameters: []contracts.ToolParameter{
			{Name: "output", Type: "string"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"out": {
						WhenTrue:  []string{"--out"},
						Param:     "output",
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": must use exactly one of boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagNeitherBooleanNorParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "empty flag",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"empty": {},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "empty": must specify either boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagParamUnknownFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "unknown param",
		Parameters: []contracts.ToolParameter{
			{Name: "known", Type: "string"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"out": {
						Param: "unknown_param",
						Style: contracts.FlagStyleEquals,
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": param "unknown_param" does not match any declared parameter`)
}

func TestValidateV2FlagBadStyleFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "bad style",
		Parameters: []contracts.ToolParameter{
			{Name: "out", Type: "string"},
		},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]contracts.ToolManifestFlag{
					"out": {
						Param: "out",
						Style: "invalid_style",
					},
				},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": style "invalid_style" must be "equals" or "separate"`)
}

func TestValidateV2ChunkingAndTelemetryPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{
				Base: []string{"rg"},
			},
		},
		Returns: contracts.ToolManifestReturns{
			Type: "json",
			Chunking: &contracts.ToolManifestReturnsChunking{
				Mode:      contracts.ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
		Telemetry: &contracts.ToolManifestTelemetry{
			SpanName:        "tool.cli_rg",
			ExtraAttributes: []string{"pattern"},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateCompositeBackendAccepts(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &contracts.ToolManifest{
		Name:        "pipe_tool",
		Family:      "shell",
		Description: "pipeline",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendComposite,
		},
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}
