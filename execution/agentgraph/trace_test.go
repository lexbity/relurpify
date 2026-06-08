package agentgraph

import (
	"context"
	"testing"

	capresult "codeburg.org/lexbit/relurpify/capability/result"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/telemetry"
	"github.com/stretchr/testify/require"
)

func TestToolNodeSetsTraceContext(t *testing.T) {
	reg := &traceCaptureRegistry{}
	node := NewToolNode("test_node", &traceTestTool{name: "test_tool"}, nil, reg)
	node.SetTraceID("root_trace_123")

	env := contextdata.NewEnvelope("test_task", "test_session")
	ctx := context.Background()
	_, err := node.Execute(ctx, env)
	require.NoError(t, err)

	// Verify that trace context was set on the context passed to InvokeCapability
	tc, ok := telemetry.TraceContextFromContext(reg.lastCtx)
	require.True(t, ok, "InvokeCapability should receive context with trace context")
	require.Equal(t, "root_trace_123", tc.TraceID)
	require.NotEmpty(t, tc.SpanID)
}

func TestToolNodeGeneratesChildSpanID(t *testing.T) {
	reg := &traceCaptureRegistry{}
	node := NewToolNode("test_node", &traceTestTool{name: "test_tool"}, nil, reg)
	node.SetTraceID("trace_for_spans")

	id1 := node.nextSpanID()
	id2 := node.nextSpanID()
	require.NotEqual(t, id1, id2, "consecutive span IDs must differ")
	require.NotEmpty(t, id1)
	require.NotEmpty(t, id2)
}

func TestNextTraceContextPreservesTraceID(t *testing.T) {
	reg := &traceCaptureRegistry{}
	node := NewToolNode("test_node", &traceTestTool{name: "test_tool"}, nil, reg)
	node.SetTraceID("my_trace")

	tc1 := node.nextTraceContext()
	tc2 := node.nextTraceContext()

	require.Equal(t, "my_trace", tc1.TraceID)
	require.Equal(t, "my_trace", tc2.TraceID)
	require.NotEqual(t, tc1.SpanID, tc2.SpanID, "each call must get a unique SpanID")
}

// traceTestTool is a minimal tool that captures the context.
type traceTestTool struct {
	name string
}

func (t *traceTestTool) Name() string                      { return t.name }
func (t *traceTestTool) Description() string               { return "trace test" }
func (t *traceTestTool) Category() string                  { return "test" }
func (t *traceTestTool) Parameters() []ports.ToolParameter { return nil }
func (t *traceTestTool) Tags() []string                    { return nil }
func (t *traceTestTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *traceTestTool) IsAvailable(ctx context.Context) bool { return true }
func (t *traceTestTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}

// traceCaptureRegistry captures the context from InvokeCapability.
type traceCaptureRegistry struct {
	lastCtx context.Context
}

func (r *traceCaptureRegistry) InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]interface{}) (*ports.ToolResult, error) {
	r.lastCtx = ctx
	return &ports.ToolResult{Success: true, Data: map[string]interface{}{"stdout": "ok"}}, nil
}

func (r *traceCaptureRegistry) CapturePolicySnapshot() *capresult.PolicySnapshot { return nil }
func (r *traceCaptureRegistry) GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool) {
	return descriptor.CapabilityDescriptor{}, false
}
