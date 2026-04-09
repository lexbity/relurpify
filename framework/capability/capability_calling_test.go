package capability

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type callingTestTool struct {
	name string
}

func (t callingTestTool) Name() string        { return t.name }
func (t callingTestTool) Description() string { return "test tool" }
func (t callingTestTool) Category() string    { return "testing" }
func (t callingTestTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: true, Description: "target path"},
		{Name: "content", Type: "string", Required: false, Description: "file contents"},
	}
}
func (t callingTestTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t callingTestTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t callingTestTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t callingTestTool) Tags() []string                                  { return nil }

func TestResolveCallingMode_NativeEnabled_CapsSupports(t *testing.T) {
	spec := &core.AgentRuntimeSpec{}
	spec.NativeToolCalling = func() *bool { v := true; return &v }()

	require.Equal(t, CapabilityCallingNative, ResolveCallingMode(spec, core.BackendCapabilities{NativeToolCalling: true}))
}

func TestResolveCallingMode_NativeEnabled_CapsLacks(t *testing.T) {
	spec := &core.AgentRuntimeSpec{}
	spec.NativeToolCalling = func() *bool { v := true; return &v }()

	require.Equal(t, CapabilityCallingFallback, ResolveCallingMode(spec, core.BackendCapabilities{}))
}

func TestResolveCallingMode_NativeDisabled(t *testing.T) {
	spec := &core.AgentRuntimeSpec{}
	spec.NativeToolCalling = func() *bool { v := false; return &v }()

	require.Equal(t, CapabilityCallingFallback, ResolveCallingMode(spec, core.BackendCapabilities{NativeToolCalling: true}))
}

func TestFallbackParity_TextMatchesNative(t *testing.T) {
	native := core.ToolCall{
		Name: "file_write",
		Args: map[string]any{
			"path":    "docs/notes.md",
			"content": "hello",
		},
	}
	fallback := ParseToolCallsFromText(`{
  "tool": "file_write",
  "arguments": {
    "path": "docs/notes.md",
    "content": "hello"
  }
}`)
	require.Equal(t, []core.ToolCall{native}, fallback)
}

func TestRenderToolsToPrompt_RoundTrip(t *testing.T) {
	tools := []core.Tool{
		callingTestTool{name: "file_write"},
		callingTestTool{name: "shell_exec"},
	}
	rendered := RenderToolsToPrompt(tools)
	require.Contains(t, rendered, "file_write")
	require.Contains(t, rendered, "shell_exec")

	resp := "{\"tool\":\"file_write\",\"arguments\":{\"path\":\"docs/a.md\",\"content\":\"alpha\"}}\n" +
		"and then:\n" +
		"{\"name\":\"shell_exec\",\"args\":{\"path\":\"scripts/run.sh\"}}\n"
	calls := ParseToolCallsFromText(resp)
	require.Len(t, calls, 2)
	require.Equal(t, "file_write", calls[0].Name)
	require.Equal(t, "shell_exec", calls[1].Name)
}

func TestParseToolCallsFromText_MalformedJSON(t *testing.T) {
	text := `{"tool":"file_write","arguments":{"path":"docs/a.md"}} junk {"tool":"broken","arguments":`
	calls := ParseToolCallsFromText(text)
	require.Len(t, calls, 1)
	require.Equal(t, "file_write", calls[0].Name)
}

func TestParseToolCallsFromText_EmptyResponse(t *testing.T) {
	calls := ParseToolCallsFromText("")
	require.Empty(t, calls)
}
