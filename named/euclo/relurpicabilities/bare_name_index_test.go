package relurpicabilities

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
)

// collisionTool is a minimal manifest-tool stand-in holding a name that a
// relurpic capability also claims as its bare name.
type collisionTool struct {
	name string
}

func (t *collisionTool) Name() string                      { return t.name }
func (t *collisionTool) Description() string               { return "collision fixture" }
func (t *collisionTool) Category() string                  { return "test" }
func (t *collisionTool) Parameters() []ports.ToolParameter { return nil }
func (t *collisionTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}
func (t *collisionTool) IsAvailable(ctx context.Context) bool { return true }
func (t *collisionTool) Permissions() ports.ToolPermissions   { return ports.ToolPermissions{} }
func (t *collisionTool) Tags() []string                       { return nil }

// TestBareCapabilityNameResolves pins D3: after RegisterAll, every
// invocable capability resolves by its bare name ("test_run") wherever the
// full ID ("euclo:cap.test_run") resolves — declared recipe vocabulary uses
// the bare form.
func TestBareCapabilityNameResolves(t *testing.T) {
	reg := registry.NewRegistry()
	ctx := context.Background()
	for _, tool := range []ports.Tool{
		&fs.ReadFileTool{BasePath: "."},
		&fs.WriteFileTool{BasePath: "."},
	} {
		if err := reg.Register(ctx, tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	if err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: []string{
			"euclo:cap.test_run", "euclo:cap.ast_query", "euclo:cap.diff_summary",
			"euclo:cap.code_review", "euclo:cap.targeted_refactor",
		},
	}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	for _, bare := range []string{"test_run", "ast_query", "diff_summary", "code_review", "targeted_refactor"} {
		bareDesc, ok := reg.GetCapability(bare)
		if !ok {
			t.Fatalf("bare name %q must resolve after registration", bare)
		}
		fullDesc, ok := reg.GetCapability("euclo:cap." + bare)
		if !ok || fullDesc.ID != bareDesc.ID {
			t.Fatalf("bare %q must resolve to the same capability as the full ID (got %q vs %q)", bare, bareDesc.ID, fullDesc.ID)
		}
	}
}

// TestBareCapabilityNameCollisionFailsRegistration pins the D3 collision
// rule: a manifest tool already holding the bare name makes the capability
// registration fail naming both holders — the namespace-pollution detector.
func TestBareCapabilityNameCollisionFailsRegistration(t *testing.T) {
	reg := registry.NewRegistry()
	ctx := context.Background()
	for _, tool := range []ports.Tool{
		&fs.ReadFileTool{BasePath: "."},
		&collisionTool{name: "test_run"},
	} {
		if err := reg.Register(ctx, tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}
	err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: []string{"euclo:cap.test_run"},
	})
	if err == nil {
		t.Fatal("registration must fail when the bare name is already held by a manifest tool")
	}
	if !strings.Contains(err.Error(), `"test_run"`) || !strings.Contains(err.Error(), "already maps to") {
		t.Fatalf("collision error must name the bare name and both holders: %v", err)
	}
}
