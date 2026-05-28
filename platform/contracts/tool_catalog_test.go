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
	require.Equal(t, ToolBackendMCP, ToolBackend("mcp"))
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
	// Zero-value ToolManifestSandbox should have AllowFlags=false and MaxOutputBytes=0
	sandbox := ToolManifestSandbox{}
	require.False(t, sandbox.AllowFlags, "AllowFlags must default to false")
	require.Equal(t, int64(0), sandbox.MaxOutputBytes, "MaxOutputBytes must default to 0")
}

func TestToolManifestSandboxJSONRoundTrip(t *testing.T) {
	sandbox := ToolManifestSandbox{
		AllowedRoot:    "/workspace",
		TimeoutSeconds: 30,
		NetworkAccess:  true,
		AllowFlags:     true,
		MaxOutputBytes: 512 * 1024,
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
	require.Equal(t, sandbox.MaxOutputBytes, decoded.MaxOutputBytes)
}

func TestToolResultTruncationFields(t *testing.T) {
	result := ToolResult{
		Success:     true,
		Data:        map[string]interface{}{"stdout": "hello"},
		Truncated:   true,
		TruncatedAt: 16000,
	}
	require.True(t, result.Truncated)
	require.Equal(t, int64(16000), result.TruncatedAt)

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded ToolResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.True(t, decoded.Truncated)
	require.Equal(t, int64(16000), decoded.TruncatedAt)
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

func TestCommandRequestMaxOutputBytes(t *testing.T) {
	req := CommandRequest{
		Args:           []string{"echo", "hello"},
		Timeout:        0,
		MaxOutputBytes: 0,
	}
	require.Equal(t, int64(0), req.MaxOutputBytes)

	req.MaxOutputBytes = 65536
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded CommandRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, int64(65536), decoded.MaxOutputBytes)
}

