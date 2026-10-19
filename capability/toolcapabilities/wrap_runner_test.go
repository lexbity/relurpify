package toolcapabilities

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// runnerAwareTool models a go_native tool that needs the sandbox command runner
// injected after construction — the shipped test runners (python_pytest,
// python_unittest, go_test, node_npm_test, rust_cargo_test) all do.
type runnerAwareTool struct{ runner ports.CommandRunner }

func (t *runnerAwareTool) Name() string                       { return "runner_aware" }
func (t *runnerAwareTool) Description() string                { return "runner-aware test tool" }
func (t *runnerAwareTool) Category() string                   { return "test" }
func (t *runnerAwareTool) Parameters() []ports.ToolParameter  { return nil }
func (t *runnerAwareTool) Permissions() ports.ToolPermissions { return ports.ToolPermissions{} }
func (t *runnerAwareTool) Tags() []string                     { return nil }
func (t *runnerAwareTool) IsAvailable(context.Context) bool   { return t.runner != nil }
func (t *runnerAwareTool) SetCommandRunner(r ports.CommandRunner) {
	t.runner = r
}
func (t *runnerAwareTool) Execute(context.Context, map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}

type noopRunner struct{}

func (noopRunner) Run(context.Context, ports.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

// The composition root injects the sandbox runner through the tool it receives
// from Build — which is the capability wrapper (app/envcomposition does
// `tool.(interface{ SetCommandRunner(ports.CommandRunner) })`). A wrapper that
// does not forward SetCommandRunner silently leaves every runner-backed
// go_native tool unavailable: declared in a recipe's `may invoke`, absent from
// the model's tool list, warned about at boot as
// recipe.vocabulary_not_model_callable.
func TestWrapWithCapabilityForwardsCommandRunner(t *testing.T) {
	wrapped := wrapWithCapability(&runnerAwareTool{}, ports.ToolManifest{
		Name:       "runner_aware",
		Capability: ports.ToolManifestCapability{TrustClass: "builtin_trusted"},
	})
	if wrapped == nil {
		t.Fatal("wrapWithCapability returned nil")
	}
	setter, ok := wrapped.(interface{ SetCommandRunner(ports.CommandRunner) })
	if !ok {
		t.Fatal("wrapped tool must forward SetCommandRunner: the composition root injects the runner through the wrapper")
	}
	if wrapped.IsAvailable(context.Background()) {
		t.Fatal("tool must be unavailable before the runner is injected")
	}
	setter.SetCommandRunner(noopRunner{})
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("tool must become available once the runner is injected through the wrapper")
	}
}
