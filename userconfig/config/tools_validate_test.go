package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
)

const (
	validateToolName       = "ok_tool"
	validateToolKind       = schemaKindTool
	validateToolFamily     = "text"
	validateToolTrustClass = "builtin_trusted"
	validateToolRiskClass  = "execute"
	validateToolEffect     = "filesystem_read"
	validateStringType     = "string"
	validateEchoBinary     = "echo"
	validateBadName        = "bad"
	validateOutputField    = "output"
	validateOutField       = "out"
)

func TestValidateToolManifestRejectsMissingRequiredFields(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name: "bad_tool",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "family required")
	require.Contains(t, err.Error(), "description required")
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestValidateToolManifestAcceptsGoNativeManifest(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        validateToolName,
		Family:      "search",
		Description: "ok",
		Parameters: []ToolParameter{
			{Name: "query", Type: validateStringType, Required: true},
		},
		Execution: ToolManifestExecution{
			Backend:        ToolBackendGoNative,
			Implementation: "ok_tool",
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{"read_only"},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

// --- v2 flag validation ---

func TestValidateV2TypedFlagEqualsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Parameters: []ToolParameter{
			{Name: "output_path", Type: validateStringType},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]ToolManifestFlag{
					validateOutputField: {
						Param: "output_path",
						Style: FlagStyleEquals,
						Type:  testStringType,
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagSeparateRepeatPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "cli_echo",
		Family:      validateToolFamily,
		Description: validateEchoBinary,
		Parameters: []ToolParameter{
			{Name: "globs", Type: "array"},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateEchoBinary},
				Flags: map[string]ToolManifestFlag{
					"glob": {
						Param:  "globs",
						Style:  FlagStyleSeparate,
						Type:   "array",
						Repeat: true,
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagEmptyStyleDefaults(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "cli_jq",
		Family:      validateToolFamily,
		Description: "jq",
		Parameters: []ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{"jq"},
				Flags: map[string]ToolManifestFlag{
					validateOutputField: {
						Param: validateOutputField,
						// Style intentionally empty — defaults to "equals"
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err, "empty style should be accepted (defaults to equals)")
}

func TestValidateV2BooleanFlagPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "cli_colordiff",
		Family:      validateToolFamily,
		Description: "colorized diff",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]ToolManifestFlag{
					"hidden": {
						WhenTrue:  []string{"--hidden"},
						WhenFalse: []string{"--no-hidden"},
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2NoFlagsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "simple",
		Family:      validateToolFamily,
		Description: "no flags",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateEchoBinary},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2FlagMixedBooleanAndParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "mixed forms",
		Parameters: []ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ToolManifestFlag{
					validateOutField: {
						WhenTrue: []string{"--out"},
						Param:    validateOutputField,
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": must use exactly one of boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagNeitherBooleanNorParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "empty flag",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ToolManifestFlag{
					"empty": {},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "empty": must specify either boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagParamUnknownFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "unknown param",
		Parameters: []ToolParameter{
			{Name: "known", Type: validateStringType},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ToolManifestFlag{
					validateOutField: {
						Param: "unknown_param",
						Style: FlagStyleEquals,
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": param "unknown_param" does not match any declared parameter`)
}

func TestValidateV2FlagBadStyleFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "bad style",
		Parameters: []ToolParameter{
			{Name: validateOutField, Type: validateStringType},
		},
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ToolManifestFlag{
					"out": {
						Param: validateOutField,
						Style: "invalid_style",
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": style "invalid_style" must be "equals" or "separate"`)
}

func TestValidateV2ChunkingPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{"rg"},
			},
		},
		Returns: ToolManifestReturns{
			Type: "json",
			Chunking: &ToolManifestReturnsChunking{
				Mode:      toolcapabilities.ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateCompositeBackendAccepts(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ToolManifest{
		Name:        "pipe_tool",
		Family:      "shell",
		Description: "pipeline",
		Execution: ToolManifestExecution{
			Backend: ToolBackendComposite,
		},
		Composition: &ToolManifestComposition{
			Steps: []ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}
