package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/userconfig/tools/manifest"
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
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name: "bad_tool",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "family required")
	require.Contains(t, err.Error(), "description required")
	require.Contains(t, err.Error(), "execution.command.base required")
}

func TestValidateToolManifestAcceptsGoNativeManifest(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        validateToolName,
		Family:      "search",
		Description: "ok",
		Parameters: []manifest.ToolParameter{
			{Name: "query", Type: validateStringType, Required: true},
		},
		Execution: manifest.ToolManifestExecution{
			Backend:        manifest.ToolBackendGoNative,
			Implementation: "ok_tool",
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{"read_only"},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

// --- v2 flag validation ---

func TestValidateV2TypedFlagEqualsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Parameters: []manifest.ToolParameter{
			{Name: "output_path", Type: validateStringType},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]manifest.ToolManifestFlag{
					validateOutputField: {
						Param: "output_path",
						Style: manifest.FlagStyleEquals,
						Type:  testStringType,
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagSeparateRepeatPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "cli_echo",
		Family:      validateToolFamily,
		Description: validateEchoBinary,
		Parameters: []manifest.ToolParameter{
			{Name: "globs", Type: "array"},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateEchoBinary},
				Flags: map[string]manifest.ToolManifestFlag{
					"glob": {
						Param:  "globs",
						Style:  manifest.FlagStyleSeparate,
						Type:   "array",
						Repeat: true,
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagEmptyStyleDefaults(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "cli_jq",
		Family:      validateToolFamily,
		Description: "jq",
		Parameters: []manifest.ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{"jq"},
				Flags: map[string]manifest.ToolManifestFlag{
					validateOutputField: {
						Param: validateOutputField,
						// Style intentionally empty — defaults to "equals"
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err, "empty style should be accepted (defaults to equals)")
}

func TestValidateV2BooleanFlagPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "cli_colordiff",
		Family:      validateToolFamily,
		Description: "colorized diff",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{"colordiff"},
				Flags: map[string]manifest.ToolManifestFlag{
					"hidden": {
						WhenTrue:  []string{"--hidden"},
						WhenFalse: []string{"--no-hidden"},
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2NoFlagsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "simple",
		Family:      validateToolFamily,
		Description: "no flags",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateEchoBinary},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2FlagMixedBooleanAndParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "mixed forms",
		Parameters: []manifest.ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]manifest.ToolManifestFlag{
					validateOutField: {
						WhenTrue: []string{"--out"},
						Param:    validateOutputField,
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": must use exactly one of boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagNeitherBooleanNorParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "empty flag",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]manifest.ToolManifestFlag{
					"empty": {},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "empty": must specify either boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagParamUnknownFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "unknown param",
		Parameters: []manifest.ToolParameter{
			{Name: "known", Type: validateStringType},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]manifest.ToolManifestFlag{
					validateOutField: {
						Param: "unknown_param",
						Style: manifest.FlagStyleEquals,
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": param "unknown_param" does not match any declared parameter`)
}

func TestValidateV2FlagBadStyleFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "bad style",
		Parameters: []manifest.ToolParameter{
			{Name: validateOutField, Type: validateStringType},
		},
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]manifest.ToolManifestFlag{
					"out": {
						Param: validateOutField,
						Style: "invalid_style",
					},
				},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": style "invalid_style" must be "equals" or "separate"`)
}

func TestValidateV2ChunkingPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "cli_rg",
		Family:      "fileops",
		Description: "ripgrep",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendSubprocess,
			Command: &manifest.ToolManifestCommand{
				Base: []string{"rg"},
			},
		},
		Returns: manifest.ToolManifestReturns{
			Type: "json",
			Chunking: &manifest.ToolManifestReturnsChunking{
				Mode:      manifest.ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateCompositeBackendAccepts(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &manifest.ToolManifest{
		Name:        "pipe_tool",
		Family:      "shell",
		Description: "pipeline",
		Execution: manifest.ToolManifestExecution{
			Backend: manifest.ToolBackendComposite,
		},
		Composition: &manifest.ToolManifestComposition{
			Steps: []manifest.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
		Capability: manifest.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}
