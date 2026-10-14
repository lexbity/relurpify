package ports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTool is a minimal Tool used by the ports tests. It implements only the
// core Tool interface — none of the optional extension interfaces — so tests
// can observe the plain-tool fallback paths.
type fakeTool struct {
	name        string
	description string
	category    string
	params      []ToolParameter
	available   bool
}

func (t *fakeTool) Name() string                       { return t.name }
func (t *fakeTool) Description() string                { return t.description }
func (t *fakeTool) Category() string                   { return t.category }
func (t *fakeTool) Parameters() []ToolParameter        { return t.params }
func (t *fakeTool) IsAvailable(_ context.Context) bool { return t.available }
func (t *fakeTool) Permissions() ToolPermissions       { return ToolPermissions{} }
func (t *fakeTool) Tags() []string                     { return nil }

func (t *fakeTool) Execute(_ context.Context, _ map[string]any) (*ToolResult, error) {
	return &ToolResult{Success: true}, nil
}

func TestLLMToolSpecFromTool_BuildsSchemaFromParameters(t *testing.T) {
	tool := &fakeTool{
		name:        "file_read",
		description: "reads a file",
		params: []ToolParameter{
			{Name: "path", Type: ToolParamString, Description: "target path", Required: true},
			{Name: "limit", Type: ToolParamInteger, Default: 10},
		},
	}

	spec := LLMToolSpecFromTool(tool)

	require.Equal(t, "file_read", spec.Name)
	require.Equal(t, "reads a file", spec.Description)
	require.NotNil(t, spec.InputSchema)
	require.Equal(t, "object", spec.InputSchema.Type)
	require.Len(t, spec.InputSchema.Properties, 2)

	path := spec.InputSchema.Properties["path"]
	require.NotNil(t, path)
	assert.Equal(t, "string", path.Type)
	assert.Equal(t, "target path", path.Description)

	limit := spec.InputSchema.Properties["limit"]
	require.NotNil(t, limit)
	assert.Equal(t, "integer", limit.Type)
	require.NotNil(t, limit.Default)
	assert.Equal(t, 10, limit.Default)

	// Only the required parameter is listed, in declaration order.
	assert.Equal(t, []string{"path"}, spec.InputSchema.Required)
}

func TestLLMToolSpecFromTool_NilDefaultStaysNil(t *testing.T) {
	tool := &fakeTool{
		name:   "no_defaults",
		params: []ToolParameter{{Name: "path", Type: ToolParamString}},
	}
	spec := LLMToolSpecFromTool(tool)
	require.NotNil(t, spec.InputSchema)
	prop := spec.InputSchema.Properties["path"]
	require.NotNil(t, prop)
	assert.Nil(t, prop.Default, "parameter without a default must not synthesize one")
}

func TestLLMToolSpecFromTool_NoParametersOmitsSchema(t *testing.T) {
	spec := LLMToolSpecFromTool(&fakeTool{name: "bare", description: "no params"})
	assert.Equal(t, "bare", spec.Name)
	assert.Nil(t, spec.InputSchema, "a tool without parameters must not emit an input schema")
}

func TestLLMToolSpecsFromTools_ConvertsInOrder(t *testing.T) {
	specs := LLMToolSpecsFromTools([]Tool{
		&fakeTool{name: "alpha"},
		&fakeTool{name: "beta", params: []ToolParameter{{Name: "p", Type: ToolParamString, Required: true}}},
	})
	require.Len(t, specs, 2)
	assert.Equal(t, "alpha", specs[0].Name)
	assert.Nil(t, specs[0].InputSchema)
	assert.Equal(t, "beta", specs[1].Name)
	assert.Equal(t, []string{"p"}, specs[1].InputSchema.Required)
}

func TestLLMToolSpecsFromTools_EmptyReturnsNil(t *testing.T) {
	assert.Nil(t, LLMToolSpecsFromTools(nil))
	assert.Nil(t, LLMToolSpecsFromTools([]Tool{}))
}
