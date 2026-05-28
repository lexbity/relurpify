package contracts

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzValidateToolArguments(f *testing.F) {
	manifest := ToolManifest{
		Parameters: []ToolParameter{
			{Name: "path", Type: ToolParamString, Required: true},
			{Name: "count", Type: ToolParamInteger},
			{Name: "verbose", Type: ToolParamBoolean},
		},
	}
	seeds := []string{
		`{"path": "src", "count": 42, "verbose": true}`,
		`{"path": "/tmp/test"}`,
		`{"path": "", "count": 0, "verbose": false}`,
		`{}`,
		`{"path": "x", "extra": 1}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var args map[string]any
		if err := json.Unmarshal(data, &args); err != nil {
			return // invalid JSON is not a crash
		}
		_ = ValidateToolArguments(manifest, args)
	})
}

func TestBuildToolExecutionPlan(t *testing.T) {
	manifest := ToolManifest{
		Name:   "demo",
		Family: "cli",
		Execution: ToolManifestExecution{
			Backend:         ToolBackendSubprocess,
			Command:         &ToolManifestCommand{Base: []string{"demo"}, Args: []string{"--flag"}},
			DefaultArgs:     []string{"--default"},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Parameters: []ToolParameter{{Name: "path", Type: "string", Required: true}},
		Capability: ToolManifestCapability{TrustClass: "local", RiskClass: []string{"read-only"}, EffectClass: []string{"inspect"}},
	}

	resolution, request, err := BuildToolExecutionPlan(manifest, map[string]any{
		"path":              "src",
		"args":              []any{"--extra"},
		"working_directory": "workspace",
		"stdin":             "hello",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"demo", "--default", "--extra"}, resolution.Command)
	require.Equal(t, request.Args, resolution.Command)
	require.Equal(t, "workspace", request.Workdir)
	require.Equal(t, "hello", request.Input)
	require.Equal(t, "workspace", resolution.Workdir)
}

func TestBuildToolExecutionPlanRejectsUnknownParameters(t *testing.T) {
	manifest := ToolManifest{
		Name:   "demo",
		Family: "cli",
		Execution: ToolManifestExecution{
			Backend:        ToolBackendGoNative,
			Implementation: "demo",
		},
		Parameters: []ToolParameter{{Name: "path", Type: "string", Required: true}},
		Capability: ToolManifestCapability{TrustClass: "local", RiskClass: []string{"read-only"}, EffectClass: []string{"inspect"}},
	}

	_, _, err := BuildToolExecutionPlan(manifest, map[string]any{"unknown": "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown parameter")
}

func TestToolParameterSummaryIncludesExecutorFields(t *testing.T) {
	manifest := ToolManifest{
		Name:   "demo",
		Family: "cli",
		Execution: ToolManifestExecution{
			Backend:         ToolBackendGoNative,
			Implementation:  "demo",
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Parameters: []ToolParameter{{Name: "path", Type: "string"}},
	}

	require.Equal(t, []string{"path", "args", "working_directory", "stdin"}, ToolParameterSummary(manifest))
	require.Equal(t, "workspace", ToolWorkdirMode(manifest))
	require.Equal(t, "demo", ToolCommand(manifest))
}
