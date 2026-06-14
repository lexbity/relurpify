package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	ports "codeburg.org/lexbit/relurpify/platform/configmanifest"
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
		Name:        validateToolName,
		Family:      "search",
		Description: "ok",
		Parameters: []ports.ToolParameter{
			{Name: "query", Type: validateStringType, Required: true},
		},
		Execution: ports.ToolManifestExecution{
			Backend:        ports.ToolBackendGoNative,
			Implementation: "ok_tool",
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{"read_only"},
			EffectClass: []string{validateToolEffect},
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
			{Name: "output_path", Type: validateStringType},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"rg"},
				Flags: map[string]ports.ToolManifestFlag{
					validateOutputField: {
						Param: "output_path",
						Style: ports.FlagStyleEquals,
						Type:  testStringType,
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagSeparateRepeatPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_echo",
		Family:      validateToolFamily,
		Description: validateEchoBinary,
		Parameters: []ports.ToolParameter{
			{Name: "globs", Type: "array"},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateEchoBinary},
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
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2TypedFlagEmptyStyleDefaults(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_jq",
		Family:      validateToolFamily,
		Description: "jq",
		Parameters: []ports.ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{"jq"},
				Flags: map[string]ports.ToolManifestFlag{
					validateOutputField: {
						Param: validateOutputField,
						// Style intentionally empty — defaults to "equals"
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err, "empty style should be accepted (defaults to equals)")
}

func TestValidateV2BooleanFlagPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "cli_colordiff",
		Family:      validateToolFamily,
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
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2NoFlagsPasses(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        "simple",
		Family:      validateToolFamily,
		Description: "no flags",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateEchoBinary},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}

func TestValidateV2FlagMixedBooleanAndParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "mixed forms",
		Parameters: []ports.ToolParameter{
			{Name: validateOutputField, Type: validateStringType},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ports.ToolManifestFlag{
					validateOutField: {
						WhenTrue: []string{"--out"},
						Param:    validateOutputField,
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": must use exactly one of boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagNeitherBooleanNorParamFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "empty flag",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ports.ToolManifestFlag{
					"empty": {},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "empty": must specify either boolean (when_true/when_false) or typed (param) form`)
}

func TestValidateV2FlagParamUnknownFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "unknown param",
		Parameters: []ports.ToolParameter{
			{Name: "known", Type: validateStringType},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ports.ToolManifestFlag{
					validateOutField: {
						Param: "unknown_param",
						Style: ports.FlagStyleEquals,
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `flag "out": param "unknown_param" does not match any declared parameter`)
}

func TestValidateV2FlagBadStyleFails(t *testing.T) {
	err := validateToolManifest("tool.tool.yaml", &ports.ToolManifest{
		Name:        validateBadName,
		Family:      validateToolFamily,
		Description: "bad style",
		Parameters: []ports.ToolParameter{
			{Name: validateOutField, Type: validateStringType},
		},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{
				Base: []string{validateToolKind},
				Flags: map[string]ports.ToolManifestFlag{
					"out": {
						Param: validateOutField,
						Style: "invalid_style",
					},
				},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
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
				Mode:      ports.ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
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
			TrustClass:  validateToolTrustClass,
			RiskClass:   []string{validateToolRiskClass},
			EffectClass: []string{validateToolEffect},
		},
	})
	require.NoError(t, err)
}
