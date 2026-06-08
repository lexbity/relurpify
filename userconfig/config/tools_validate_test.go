package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
)

func TestValidateToolManifestRejectsMissingRequiredFields(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name: "bad_tool",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "family required")
	require.Contains(t, err.Error(), "description required")
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestValidateToolManifestAcceptsGoNativeManifest(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "ok_tool",
		Family:      "search",
		Description: "ok",
		Parameters: []ports.ToolParameter{
			{Name: "query", Type: "string", Required: true},
		},
		Execution: ports.ToolManifestExecution{
			Backend:        ports.ToolBackendGoNative,
			Implementation: "ok_tool",
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"read_only"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

// --- v2 flag validation ---

func TestValidateV2TypedFlagEqualsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Parameters: []ports.ToolParameter{
			{Name: "output_path", Type: "string"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]ports.ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: ports.FlagStyleEquals,
						Type:  "string",
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagSeparateRepeatPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_echo",
		Family:      "text",
		Description: "echo",
		Parameters: []ports.ToolParameter{
			{Name: "globs", Type: "array"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"echo"},
				Flags: map[string]ports.ToolManifestFlag{
					"glob": {
						Param:  "globs",
						Style:  ports.FlagStyleSeparate,
						Type:   "array",
						Repeat: true,
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagEmptyStyleDefaults(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_jq",
		Family:      "text",
		Description: "jq",
		Parameters: []ports.ToolParameter{
			{Name: "output", Type: "string"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"jq"},
				Flags: map[string]ports.ToolManifestFlag{
					"output": {
						Param: "output",
						// Style intentionally empty — defaults to "equals"
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err, "empty style should be accepted (defaults to equals)")
}

func TestValidateV2BooleanFlagPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_colordiff",
		Family:      "text",
		Description: "colorized diff",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]ports.ToolManifestFlag{
					"hidden": {
						WhenTrue:  []string{"--hidden"},
						WhenFalse: []string{"--no-hidden"},
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2NoFlagsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "simple",
		Family:      "text",
		Description: "no flags",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"echo"},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2FlagMixedBooleanAndParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "mixed forms",
		Parameters: []ports.ToolParameter{
			{Name: "output", Type: "string"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]ports.ToolManifestFlag{
					"out": {
						WhenTrue: []string{"--out"},
						Param:    "output",
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": must use exactly one of boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagNeitherBooleanNorParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "empty flag",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]ports.ToolManifestFlag{
					"empty": {},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "empty": must specify either boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagParamUnknownFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "unknown param",
		Parameters: []ports.ToolParameter{
			{Name: "known", Type: "string"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]ports.ToolManifestFlag{
					"out": {
						Param: "unknown_param",
						Style: ports.FlagStyleEquals,
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": param "unknown_param" does not match any declared parameter`)
}

func TestValidateV2FlagBadStyleFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "bad",
		Family:      "text",
		Description: "bad style",
		Parameters: []ports.ToolParameter{
			{Name: "out", Type: "string"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"tool"},
				Flags: map[string]ports.ToolManifestFlag{
					"out": {
						Param: "out",
						Style: "invalid_style",
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": style "invalid_style" must be "equals" or "separate"`)
}

func TestValidateV2ChunkingPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"rg"},
			},
		},
		Returns: ports.ToolManifestReturns{
			Type: "json",
			Chunking: &ports.ToolManifestReturnsChunking{
				Mode:      toolcapabilities.ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}

func TestValidateCompositeBackendAccepts(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "pipe_tool",
		Family:      "shell",
		Description: "pipeline",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendComposite,
		},
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	})
	require.NoError(t, err)
}
