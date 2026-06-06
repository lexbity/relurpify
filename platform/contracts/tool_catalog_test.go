package contracts

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeToolName(t *testing.T) {
	require.Equal(t, "cli_git", NormalizeToolName("  CLI-Git  "))
	require.Equal(t, "shell_query", NormalizeToolName("shell.query"))
	require.Equal(t, "tool_name", NormalizeToolName("tool name"))
}

func TestToolManifestBackendConstants(t *testing.T) {
	require.Equal(t, ToolBackendSubprocess, ToolBackend("subprocess"))
	require.Equal(t, ToolBackendGoNative, ToolBackend("go_native"))
	require.Equal(t, ToolBackendComposite, ToolBackend("composite"))
	distinct := map[ToolBackend]bool{
		ToolBackendSubprocess: true,
		ToolBackendGoNative:   true,
		ToolBackendComposite:  true,
	}
	require.Len(t, distinct, 3, "all ToolBackend constants must have distinct values")
}

func TestToolManifestFlagStyleConstants(t *testing.T) {
	require.Equal(t, "equals", FlagStyleEquals)
	require.Equal(t, "separate", FlagStyleSeparate)
}

func TestToolManifestChunkingModeConstants(t *testing.T) {
	require.Equal(t, "whole", ChunkingModeWhole)
	require.Equal(t, "per_item", ChunkingModePerItem)
	require.Equal(t, "per_field", ChunkingModePerField)
}

func TestToolParameterTypeConstants(t *testing.T) {
	distinct := map[ToolParameterType]bool{
		ToolParamString:  true,
		ToolParamInteger: true,
		ToolParamNumber:  true,
		ToolParamBoolean: true,
		ToolParamArray:   true,
		ToolParamObject:  true,
	}
	require.Len(t, distinct, 6, "all ToolParameterType constants must have distinct values")

	tests := []struct {
		val ToolParameterType
		str string
	}{
		{ToolParamString, "string"},
		{ToolParamInteger, "integer"},
		{ToolParamNumber, "number"},
		{ToolParamBoolean, "boolean"},
		{ToolParamArray, "array"},
		{ToolParamObject, "object"},
	}
	for _, tc := range tests {
		got, err := json.Marshal(tc.val)
		require.NoError(t, err)
		require.Equal(t, `"`+tc.str+`"`, string(got), "JSON serialization of %s", tc.str)

		var decoded ToolParameterType
		err = json.Unmarshal(got, &decoded)
		require.NoError(t, err)
		require.Equal(t, tc.val, decoded, "JSON round-trip of %s", tc.str)
	}
}

func TestToolManifestSandboxDefaults(t *testing.T) {
	sandbox := ToolManifestSandbox{}
	require.False(t, sandbox.AllowFlags, "AllowFlags must default to false")
	require.Equal(t, int64(0), sandbox.MemoryMB, "MemoryMB must default to 0")
	require.Equal(t, int64(0), sandbox.PidsLimit, "PidsLimit must default to 0")
	require.Equal(t, float64(0), sandbox.CPUs, "CPUs must default to 0")
}

func TestToolManifestSandboxJSONRoundTrip(t *testing.T) {
	sandbox := ToolManifestSandbox{
		AllowedRoot:    "/workspace",
		TimeoutSeconds: 30,
		NetworkAccess:  true,
		AllowFlags:     true,
		MemoryMB:       1024,
		PidsLimit:      512,
		CPUs:           2.0,
	}
	data, err := json.Marshal(sandbox)
	require.NoError(t, err)

	var decoded ToolManifestSandbox
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, sandbox.AllowedRoot, decoded.AllowedRoot)
	require.Equal(t, sandbox.TimeoutSeconds, decoded.TimeoutSeconds)
	require.Equal(t, sandbox.NetworkAccess, decoded.NetworkAccess)
	require.Equal(t, sandbox.AllowFlags, decoded.AllowFlags)
	require.Equal(t, sandbox.MemoryMB, decoded.MemoryMB)
	require.Equal(t, sandbox.PidsLimit, decoded.PidsLimit)
	require.Equal(t, sandbox.CPUs, decoded.CPUs)
}

func TestToolManifestRateLimit(t *testing.T) {
	// Verify that a ToolManifest can carry a RateLimit and it round-trips
	manifest := ToolManifest{
		Name:    "rate_limited_tool",
		Version: "1.0",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{Base: []string{"echo"}},
		},
		Capability: ToolManifestCapability{
			TrustClass: "workspace-trusted",
		},
		RateLimit: &ToolRateLimit{
			PerSecond: 5.0,
			Burst:     10,
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.RateLimit)
	require.Equal(t, 5.0, decoded.RateLimit.PerSecond)
	require.Equal(t, 10, decoded.RateLimit.Burst)
}

func FuzzNormalizeToolName(f *testing.F) {
	seeds := []string{
		"  CLI-Git  ",
		"shell.query",
		"tool name",
		"simple",
		"UPPER_CASE",
		"dots.and.dashes",
		"",
		"___",
		"a",
		"123",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		result := NormalizeToolName(name)
		if result != "" && len(result) > len(name) {
			t.Errorf("NormalizeToolName(%q) = %q (len %d), expected len <= %d", name, result, len(result), len(name))
		}
	})
}

func TestToolManifestFlagDefaults(t *testing.T) {
	flag := ToolManifestFlag{}
	require.Empty(t, flag.Param, "Param must default to empty")
	require.Empty(t, flag.Style, "Style must default to empty")
	require.Empty(t, flag.Type, "Type must default to empty")
	require.False(t, flag.Repeat, "Repeat must default to false")
}

func TestToolManifestV2TypedFlagEqualsRoundTrip(t *testing.T) {
	manifest := ToolManifest{
		Name:    "cli_rg",
		Version: "2",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base:  []string{"rg"},
				Flags: map[string]ToolManifestFlag{
					"output": {
						Param: "output_path",
						Style: FlagStyleEquals,
						Type:  "string",
					},
				},
			},
		},
		Capability: ToolManifestCapability{
			TrustClass: "builtin_trusted",
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, "cli_rg", decoded.Name)
	require.NotNil(t, decoded.Execution.Command)
	flag, ok := decoded.Execution.Command.Flags["output"]
	require.True(t, ok)
	require.Equal(t, "output_path", flag.Param)
	require.Equal(t, FlagStyleEquals, flag.Style)
	require.Equal(t, "string", flag.Type)
	require.False(t, flag.Repeat)
}

func TestToolManifestV2TypedFlagSeparateRepeatRoundTrip(t *testing.T) {
	manifest := ToolManifest{
		Name:    "cli_echo",
		Version: "2",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{
				Base: []string{"echo"},
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
			TrustClass: "builtin_trusted",
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, "cli_echo", decoded.Name)
	flag, ok := decoded.Execution.Command.Flags["glob"]
	require.True(t, ok)
	require.Equal(t, "globs", flag.Param)
	require.Equal(t, FlagStyleSeparate, flag.Style)
	require.Equal(t, "array", flag.Type)
	require.True(t, flag.Repeat)
}

func TestToolManifestV2BooleanFlagBackwardCompat(t *testing.T) {
	manifest := ToolManifest{
		Name:    "cli_colordiff",
		Version: "2",
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
			TrustClass: "builtin_trusted",
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	flag, ok := decoded.Execution.Command.Flags["hidden"]
	require.True(t, ok)
	require.Equal(t, []string{"--hidden"}, flag.WhenTrue)
	require.Equal(t, []string{"--no-hidden"}, flag.WhenFalse)
	// New fields remain zero-valued in backward-compat mode
	require.Empty(t, flag.Param)
	require.Empty(t, flag.Style)
	require.Empty(t, flag.Type)
	require.False(t, flag.Repeat)
}

func TestToolManifestV2ChunkingRoundTrip(t *testing.T) {
	manifest := ToolManifest{
		Name:    "cli_rg",
		Version: "2",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: ToolManifestCapability{
			TrustClass: "builtin_trusted",
		},
		Returns: ToolManifestReturns{
			Type: "json",
			Chunking: &ToolManifestReturnsChunking{
				Mode:      ChunkingModePerItem,
				ItemPath:  "matches[]",
				RefFields: []string{"path", "line"},
			},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, "json", decoded.Returns.Type)
	require.NotNil(t, decoded.Returns.Chunking)
	require.Equal(t, ChunkingModePerItem, decoded.Returns.Chunking.Mode)
	require.Equal(t, "matches[]", decoded.Returns.Chunking.ItemPath)
	require.Equal(t, []string{"path", "line"}, decoded.Returns.Chunking.RefFields)
}

func TestToolManifestV2TelemetryRoundTrip(t *testing.T) {
	manifest := ToolManifest{
		Name:    "cli_rg",
		Version: "2",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{Base: []string{"rg"}},
		},
		Capability: ToolManifestCapability{
			TrustClass: "builtin_trusted",
		},
		Telemetry: &ToolManifestTelemetry{
			SpanName:        "tool.cli_rg",
			ExtraAttributes: []string{"pattern"},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Telemetry)
	require.Equal(t, "tool.cli_rg", decoded.Telemetry.SpanName)
	require.Equal(t, []string{"pattern"}, decoded.Telemetry.ExtraAttributes)
}

func TestToolManifestV1BackwardCompat(t *testing.T) {
	// A v1-style manifest (no v2 fields) must decode without error and
	// leave all v2 fields at their zero values.
	manifest := ToolManifest{
		Name:    "cli_jq",
		Version: "1",
		Family:  "text",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{Base: []string{"jq"}},
		},
		Capability: ToolManifestCapability{
			TrustClass: "builtin_trusted",
			RiskClass:  []string{"execute"},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	// Round-trip through JSON to simulate decode from disk
	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, "cli_jq", decoded.Name)
	require.Equal(t, ToolBackendSubprocess, decoded.Execution.Backend)

	// v2 fields must be at zero values
	require.Nil(t, decoded.Returns.Chunking, "v1 manifest must decode with nil Chunking")
	require.Nil(t, decoded.Telemetry, "v1 manifest must decode with nil Telemetry")
	if decoded.Execution.Command != nil && decoded.Execution.Command.Flags != nil {
		for _, flag := range decoded.Execution.Command.Flags {
			require.Empty(t, flag.Param, "v1 manifest flags must have empty Param")
			require.Empty(t, flag.Style, "v1 manifest flags must have empty Style")
			require.Empty(t, flag.Type, "v1 manifest flags must have empty Type")
			require.False(t, flag.Repeat, "v1 manifest flags must have Repeat==false")
		}
	}
}

func TestToolManifestV2CompositeBackend(t *testing.T) {
	manifest := ToolManifest{
		Name:    "pipe_tool",
		Version: "2",
		Execution: ToolManifestExecution{
			Backend: ToolBackendComposite,
		},
		Capability: ToolManifestCapability{
			TrustClass: "builtin_trusted",
		},
		Composition: &ToolManifestComposition{
			Steps: []ToolManifestCompositionStep{
				{Tool: "cli_fd", Args: map[string]any{"pattern": "*.go"}, Alias: "files"},
				{Tool: "cli_rg", Args: map[string]any{"pattern": "func", "paths": "${files.stdout}"}, Alias: "matches"},
			},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	var decoded ToolManifest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, ToolBackendComposite, decoded.Execution.Backend)
	require.NotNil(t, decoded.Composition)
	require.Len(t, decoded.Composition.Steps, 2)
	require.Equal(t, "files", decoded.Composition.Steps[0].Alias)
	require.Equal(t, "${files.stdout}", decoded.Composition.Steps[1].Args["paths"])
}

func TestCommandRequestOutputCeiling(t *testing.T) {
	req := CommandRequest{
		Args:    []string{"echo", "hello"},
		Timeout: 0,
	}
	require.Equal(t, int64(0), req.OutputCeiling)

	req.OutputCeiling = 65536
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded CommandRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, int64(65536), decoded.OutputCeiling)
}

