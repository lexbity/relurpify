package pipeline

import (
	"context"
	"strings"
	"testing"

	frameworktools "codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type scopedPipelineStage struct {
	allowed []string
}

func (s scopedPipelineStage) Name() string { return "scoped" }
func (s scopedPipelineStage) AllowedToolNames() []string {
	return append([]string(nil), s.allowed...)
}
func (s scopedPipelineStage) Contract() ContractDescriptor {
	return ContractDescriptor{
		Name: "scoped-stage",
		Metadata: ContractMetadata{
			AllowTools: true,
		},
	}
}
func (s scopedPipelineStage) BuildPrompt(ctx *contextdata.Envelope) (string, error) {
	_ = ctx
	return "stage prompt", nil
}
func (s scopedPipelineStage) Decode(resp *contracts.LLMResponse) (any, error) {
	_ = resp
	return map[string]any{"ok": true}, nil
}
func (s scopedPipelineStage) Validate(output any) error { return nil }
func (s scopedPipelineStage) Apply(ctx *contextdata.Envelope, output any) error {
	_ = ctx
	_ = output
	return nil
}

type scopedPipelineTool struct {
	name string
}

func (t scopedPipelineTool) Name() string                          { return t.name }
func (t scopedPipelineTool) Description() string                   { return t.name }
func (t scopedPipelineTool) Category() string                      { return "test" }
func (t scopedPipelineTool) Parameters() []contracts.ToolParameter { return nil }
func (t scopedPipelineTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	_ = ctx
	_ = args
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"name": t.name}}, nil
}
func (t scopedPipelineTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return true
}
func (t scopedPipelineTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (t scopedPipelineTool) Tags() []string { return nil }

func pipelineToolCapabilityID(name string) string {
	return "tool:" + name
}

func TestResolveStageToolsRespectsScopedRegistry(t *testing.T) {
	reg := frameworktools.NewCapabilityRegistry()
	if err := reg.RegisterLegacyTool(scopedPipelineTool{name: "scope_read"}); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(scopedPipelineTool{name: "scope_write"}); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}

	scoped := reg.WithAllowlist([]string{pipelineToolCapabilityID("scope_read")})
	stage := scopedPipelineStage{allowed: []string{"scope_read", "scope_write"}}
	tools := resolveStageTools(context.Background(), nil, stage, scoped.ModelCallableTools())
	if got, want := len(tools), 1; got != want {
		t.Fatalf("stage tools count = %d, want %d", got, want)
	}
	if got := tools[0].Name(); got != "scope_read" {
		t.Fatalf("stage tool name = %q, want scope_read", got)
	}
	rendered := frameworktools.RenderToolsToPrompt(tools)
	if !strings.Contains(rendered, "scope_read") {
		t.Fatalf("rendered tool prompt missing scoped tool: %q", rendered)
	}
	if strings.Contains(rendered, "scope_write") {
		t.Fatalf("rendered tool prompt leaked hidden tool: %q", rendered)
	}
}
