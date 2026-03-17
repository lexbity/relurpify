package capabilities_test

import (
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/agents/chainer/capabilities"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

type mockStage struct {
	name string
}

func (m *mockStage) Name() string {
	return m.name
}

func (m *mockStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Metadata: pipeline.ContractMetadata{
			OutputKey: "output",
		},
	}
}

func (m *mockStage) BuildPrompt(ctx *core.Context) (string, error) {
	return "test prompt", nil
}

func (m *mockStage) Decode(resp *core.LLMResponse) (any, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}
	return resp.Text, nil
}

func (m *mockStage) Validate(output any) error {
	return nil
}

func (m *mockStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("output", output)
	return nil
}

func TestScopedLinkStage_AllowedToolNames_NoRestriction(t *testing.T) {
	// No allowed tools specified = all tools allowed
	link := chainer.NewLink("test", "prompt", nil, "output", nil)
	stage := &mockStage{name: "test"}

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	allowedNames := scoped.AllowedToolNames()
	if allowedNames != nil {
		t.Fatal("nil AllowedTools should return nil (all tools allowed)")
	}
}

func TestScopedLinkStage_AllowedToolNames_Whitelist(t *testing.T) {
	// With allowed tools, returns only those
	link := chainer.NewLink("test", "prompt", nil, "output", nil)
	link.AllowedTools = []string{"tool1", "tool2"}
	stage := &mockStage{name: "test"}

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	allowedNames := scoped.AllowedToolNames()
	if len(allowedNames) != 2 {
		t.Errorf("expected 2 allowed tools, got %d", len(allowedNames))
	}
	if allowedNames[0] != "tool1" || allowedNames[1] != "tool2" {
		t.Errorf("unexpected allowed tools: %v", allowedNames)
	}
}

func TestScopedLinkStage_ToolAccessPolicy_Allowed(t *testing.T) {
	link := chainer.NewLink("test", "prompt", nil, "output", nil)
	link.AllowedTools = []string{"tool1", "tool2"}
	stage := &mockStage{name: "test"}

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	action := scoped.ToolAccessPolicy("tool1")
	if action != core.InsertionActionDirect {
		t.Errorf("expected direct inclusion, got %v", action)
	}
}

func TestScopedLinkStage_ToolAccessPolicy_Denied(t *testing.T) {
	link := chainer.NewLink("test", "prompt", nil, "output", nil)
	link.AllowedTools = []string{"tool1", "tool2"}
	stage := &mockStage{name: "test"}

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	action := scoped.ToolAccessPolicy("tool3")
	if action != core.InsertionActionDenied {
		t.Errorf("expected denied, got %v", action)
	}
}

func TestScopedLinkStage_Delegation(t *testing.T) {
	// Verify delegation methods work
	link := chainer.NewLink("test", "prompt", nil, "output", nil)
	stage := &mockStage{name: "test-stage"}

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	if scoped.Name() != "test-stage" {
		t.Errorf("expected name test-stage, got %s", scoped.Name())
	}

	contract := scoped.Contract()
	if contract.Metadata.OutputKey != "output" {
		t.Errorf("expected output key output, got %s", contract.Metadata.OutputKey)
	}

	prompt, err := scoped.BuildPrompt(core.NewContext())
	if err != nil || prompt != "test prompt" {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	resp := &core.LLMResponse{Text: "result"}
	decoded, err := scoped.Decode(resp)
	if err != nil || decoded != "result" {
		t.Fatalf("Decode failed: %v", err)
	}

	err = scoped.Validate("result")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	ctx := core.NewContext()
	err = scoped.Apply(ctx, "result")
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if val, ok := ctx.Get("output"); !ok || val != "result" {
		t.Fatal("Apply did not set output")
	}
}

func TestScopedLinkStage_NilStage(t *testing.T) {
	var stage pipeline.Stage // nil stage
	link := chainer.NewLink("test", "prompt", nil, "output", nil)

	scoped := capabilities.NewScopedLinkStage(stage, &link, nil)

	if scoped.Name() != "" {
		t.Fatal("nil stage should have empty name")
	}

	_, err := scoped.BuildPrompt(core.NewContext())
	if err == nil {
		t.Fatal("nil stage should error")
	}
}

func TestScopedLinkStage_NilLink(t *testing.T) {
	stage := &mockStage{name: "test"}

	scoped := capabilities.NewScopedLinkStage(stage, nil, nil)

	// Should allow all tools (no restrictions)
	allowedNames := scoped.AllowedToolNames()
	if allowedNames != nil {
		t.Fatal("nil link should allow all tools")
	}

	action := scoped.ToolAccessPolicy("any-tool")
	if action != core.InsertionActionDirect {
		t.Errorf("nil link should allow direct, got %v", action)
	}
}
