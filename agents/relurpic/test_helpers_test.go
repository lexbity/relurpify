package relurpic

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
)

type architectStubTool struct{}

func (architectStubTool) Name() string        { return "echo" }
func (architectStubTool) Description() string { return "echoes input" }
func (architectStubTool) Category() string    { return "test" }
func (architectStubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "value", Type: "string", Required: false}}
}
func (architectStubTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{"echo": args["value"]}}, nil
}
func (architectStubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (architectStubTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (architectStubTool) Tags() []string                                  { return nil }
