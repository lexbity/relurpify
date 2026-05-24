package capability

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type availabilityToggleTool struct {
	name      string
	available bool
}

func (t *availabilityToggleTool) Name() string                          { return t.name }
func (t *availabilityToggleTool) Description() string                   { return t.name }
func (t *availabilityToggleTool) Category() string                      { return "test" }
func (t *availabilityToggleTool) Parameters() []contracts.ToolParameter { return nil }
func (t *availabilityToggleTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	_ = ctx
	_ = args
	return &contracts.ToolResult{Success: true, Data: map[string]any{"name": t.name}}, nil
}
func (t *availabilityToggleTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return t.available
}
func (t *availabilityToggleTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (t *availabilityToggleTool) Tags() []string { return nil }

func TestAllCapabilitySnapshots_IncludesCallable(t *testing.T) {
	reg := NewCapabilityRegistry()
	desc := core.CapabilityDescriptor{
		ID:            "cap:callable",
		Name:          "callable",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != desc.ID {
		t.Fatalf("expected descriptor %q, got %q", desc.ID, snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureInspectable {
		t.Fatalf("expected inspectable exposure, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_IncludesHidden(t *testing.T) {
	reg := NewCapabilityRegistry()
	desc := core.CapabilityDescriptor{
		ID:            "cap:hidden",
		Name:          "hidden",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{Name: desc.Name},
		Access:   core.CapabilityExposureHidden,
	}})

	if got := reg.AllCapabilities(); len(got) != 0 {
		t.Fatalf("expected hidden capability to be omitted from AllCapabilities, got %d", len(got))
	}

	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != desc.ID {
		t.Fatalf("expected descriptor %q, got %q", desc.ID, snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureHidden {
		t.Fatalf("expected hidden exposure, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_Empty(t *testing.T) {
	reg := NewCapabilityRegistry()
	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 0 {
		t.Fatalf("expected empty snapshots, got %d", len(snapshots))
	}
}

func TestAllCapabilitySnapshots_DelegateRegistry(t *testing.T) {
	reg := NewCapabilityRegistry()
	visible := core.CapabilityDescriptor{
		ID:            "cap:visible",
		Name:          "visible",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	hidden := core.CapabilityDescriptor{
		ID:            "cap:hidden",
		Name:          "delegate-hidden",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(visible); err != nil {
		t.Fatalf("register visible capability: %v", err)
	}
	if err := reg.RegisterCapability(hidden); err != nil {
		t.Fatalf("register hidden capability: %v", err)
	}
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{ID: hidden.ID},
		Access:   core.CapabilityExposureHidden,
	}})

	scoped := reg.WithAllowlist([]string{hidden.ID})
	snapshots := scoped.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 scoped snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != hidden.ID {
		t.Fatalf("expected hidden capability in scoped snapshot, got %q", snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureHidden {
		t.Fatalf("expected hidden exposure in scoped snapshot, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_ConcurrentAccess(t *testing.T) {
	reg := NewCapabilityRegistry()
	const total = 32
	errCh := make(chan error, total*2)
	done := make(chan struct{})

	for i := 0; i < total; i++ {
		go func(i int) {
			desc := core.CapabilityDescriptor{
				ID:            "cap:concurrent:" + string(rune('a'+i)),
				Name:          "concurrent",
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			}
			if err := reg.RegisterCapability(desc); err != nil {
				errCh <- err
				return
			}
			_ = reg.AllCapabilitySnapshots()
			errCh <- nil
		}(i)
	}

	go func() {
		for i := 0; i < total; i++ {
			_ = reg.AllCapabilitySnapshots()
		}
		close(done)
	}()

	for i := 0; i < total; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent register failed: %v", err)
		}
	}
	<-done
}

func TestModelCallableTools_ExcludesUnavailableToolsOnRebuild(t *testing.T) {
	reg := NewCapabilityRegistry()
	tool := &availabilityToggleTool{name: "scope_read", available: true}
	if err := reg.RegisterLegacyTool(tool); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}

	if got, want := len(reg.ModelCallableTools()), 1; got != want {
		t.Fatalf("callable tool count = %d, want %d", got, want)
	}

	tool.available = false

	if got, want := len(reg.ModelCallableTools()), 0; got != want {
		t.Fatalf("callable tool count after disable = %d, want %d", got, want)
	}
	if got, want := len(reg.CaptureExecutionCatalogSnapshot().ModelCallableTools()), 0; got != want {
		t.Fatalf("snapshot callable tool count after disable = %d, want %d", got, want)
	}
}

func TestInvokeCapability_ReturnsUnavailableWhenToolDisappears(t *testing.T) {
	reg := NewCapabilityRegistry()
	tool := &availabilityToggleTool{name: "scope_read", available: true}
	if err := reg.RegisterLegacyTool(tool); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}

	tool.available = false
	_, err := reg.InvokeCapability(context.Background(), nil, "scope_read", nil)
	if err == nil {
		t.Fatal("expected invoke error")
	}
	if !strings.Contains(err.Error(), "tool unavailable") {
		t.Fatalf("invoke error = %v, want unavailable failure", err)
	}
}

func TestWithAllowlist_PreservesFilteringAndReflectsAvailabilityChanges(t *testing.T) {
	reg := NewCapabilityRegistry()
	visible := &availabilityToggleTool{name: "scope_read", available: true}
	hidden := &availabilityToggleTool{name: "scope_write", available: true}
	if err := reg.RegisterLegacyTool(visible); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(hidden); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}

	scoped := reg.WithAllowlist([]string{"tool:scope_read"})
	if scoped == nil {
		t.Fatal("expected scoped registry")
	}

	if got, want := len(scoped.ModelCallableTools()), 1; got != want {
		t.Fatalf("scoped callable tool count = %d, want %d", got, want)
	}
	if got := scoped.ModelCallableTools()[0].Name(); got != "scope_read" {
		t.Fatalf("scoped callable tool name = %q, want scope_read", got)
	}

	visible.available = false

	if got, want := len(scoped.ModelCallableTools()), 0; got != want {
		t.Fatalf("scoped callable tool count after disable = %d, want %d", got, want)
	}
	if got, want := len(scoped.CaptureExecutionCatalogSnapshot().ModelCallableTools()), 0; got != want {
		t.Fatalf("scoped snapshot callable tool count after disable = %d, want %d", got, want)
	}
}
