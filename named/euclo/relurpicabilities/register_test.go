package relurpicabilities

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

var declaredRelurpicIDs = []string{
	"euclo:cap.test_run",
	"euclo:cap.ast_query",
	"euclo:cap.symbol_trace",
	"euclo:cap.call_graph",
	"euclo:cap.blame_trace",
	"euclo:cap.bisect",
	"euclo:cap.code_review",
	"euclo:cap.diff_summary",
	"euclo:cap.layer_check",
	"euclo:cap.targeted_refactor",
	"euclo:cap.rename_symbol",
	"euclo:cap.api_compat",
	"euclo:cap.boundary_report",
	"euclo:cap.coverage_check",
}

type availabilityTool struct {
	name      string
	available bool
}

func (t availabilityTool) Name() string        { return t.name }
func (t availabilityTool) Description() string { return t.name }
func (t availabilityTool) Category() string    { return "test" }
func (t availabilityTool) Parameters() []ports.ToolParameter {
	return nil
}
func (t availabilityTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}
func (t availabilityTool) IsAvailable(ctx context.Context) bool { return t.available }
func (t availabilityTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t availabilityTool) Tags() []string { return []string{"test"} }

func TestRegisterAllNilRegistryErrors(t *testing.T) {
	err := RegisterAll(RegistrationDeps{
		Registry: nil,
		Declared: declaredRelurpicIDs,
	})
	if err == nil {
		t.Fatal("expected error when registry is nil, got nil")
	}
	if err.Error() != "capability registry is nil" {
		t.Fatalf("expected 'capability registry is nil' error, got: %v", err)
	}
}

func TestRegisterAllEmptyDeclaredErrors(t *testing.T) {
	err := RegisterAll(RegistrationDeps{
		Registry: registry.NewRegistry(),
		Declared: nil,
	})
	if err == nil {
		t.Fatal("expected error when declared is nil, got nil")
	}
	if !strings.Contains(err.Error(), "capabilities.relurpic required") {
		t.Fatalf("expected 'capabilities.relurpic required' error, got: %v", err)
	}
}

func TestRegisterAllEmptyDeclaredSliceErrors(t *testing.T) {
	err := RegisterAll(RegistrationDeps{
		Registry: registry.NewRegistry(),
		Declared: []string{},
	})
	if err == nil {
		t.Fatal("expected error when declared is empty")
	}
}

func TestRegisterAllRejectsUnknownDeclaration(t *testing.T) {
	err := RegisterAll(RegistrationDeps{
		Registry: registry.NewRegistry(),
		Declared: []string{"euclo:cap.test_run", "euclo:cap.does_not_exist"},
	})
	if err == nil {
		t.Fatal("expected unknown declaration to fail")
	}
	if !strings.Contains(err.Error(), "unknown relurpic capability declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterAllSkipsDuplicateDeclared(t *testing.T) {
	reg := registry.NewRegistry()
	err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: append(declaredRelurpicIDs, declaredRelurpicIDs...), // duplicate declared
	})
	if err != nil {
		t.Fatalf("expected duplicate declared to be idempotent, got: %v", err)
	}
	// Verify all capabilities are registered once
	for _, id := range declaredRelurpicIDs {
		if !reg.HasCapability(id) {
			t.Fatalf("expected capability %s to be registered", id)
		}
	}
}

func TestRegisterAllAlreadyRegisteredIsIdempotent(t *testing.T) {
	reg := registry.NewRegistry()
	deps := RegistrationDeps{
		Registry: reg,
		Declared: declaredRelurpicIDs,
	}
	if err := RegisterAll(deps); err != nil {
		t.Fatalf("first register: %v", err)
	}
	count := len(reg.AllCapabilitySnapshots())
	// Register again - should be idempotent
	if err := RegisterAll(deps); err != nil {
		t.Fatalf("second register: %v", err)
	}
	if got := len(reg.AllCapabilitySnapshots()); got != count {
		t.Fatalf("expected %d capabilities after re-registration, got %d", count, got)
	}
}

func TestRegisterAllValidRegistryNoError(t *testing.T) {
	err := RegisterAll(RegistrationDeps{
		Registry: registry.NewRegistry(),
		Declared: declaredRelurpicIDs,
	})
	if err != nil {
		t.Fatalf("expected no error with valid registry, got: %v", err)
	}
}

func TestRegisterAllEmptyToolRegistryMarksAllUnavailable(t *testing.T) {
	reg := registry.NewRegistry()
	if err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: declaredRelurpicIDs,
	}); err != nil {
		t.Fatalf("register all: %v", err)
	}

	for _, id := range []string{
		"euclo:cap.test_run",
		"euclo:cap.ast_query",
		"euclo:cap.symbol_trace",
		"euclo:cap.call_graph",
		"euclo:cap.blame_trace",
		"euclo:cap.bisect",
		"euclo:cap.code_review",
		"euclo:cap.diff_summary",
		"euclo:cap.layer_check",
		"euclo:cap.targeted_refactor",
		"euclo:cap.rename_symbol",
		"euclo:cap.api_compat",
		"euclo:cap.boundary_report",
		"euclo:cap.coverage_check",
	} {
		desc, ok := reg.GetCapability(id)
		if !ok {
			t.Fatalf("expected capability %s to be registered", id)
		}
		if desc.Availability.Available {
			t.Fatalf("expected capability %s to be unavailable in an empty tool registry", id)
		}
		if desc.Availability.Reason == "" {
			t.Fatalf("expected availability reason for %s", id)
		}
	}
}

func TestRegisterAllAvailabilityDependsOnRequiredTools(t *testing.T) {
	reg := registry.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))

	if err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: declaredRelurpicIDs,
	}); err != nil {
		t.Fatalf("register all: %v", err)
	}

	for _, id := range []string{
		"euclo:cap.test_run",
		"euclo:cap.ast_query",
		"euclo:cap.symbol_trace",
		"euclo:cap.call_graph",
		"euclo:cap.blame_trace",
		"euclo:cap.bisect",
		"euclo:cap.code_review",
		"euclo:cap.diff_summary",
		"euclo:cap.layer_check",
		"euclo:cap.targeted_refactor",
		"euclo:cap.rename_symbol",
		"euclo:cap.api_compat",
		"euclo:cap.boundary_report",
		"euclo:cap.coverage_check",
	} {
		desc, ok := reg.GetCapability(id)
		if !ok {
			t.Fatalf("expected capability %s to be registered", id)
		}
		if !desc.Availability.Available {
			t.Fatalf("expected capability %s to be available, got reason %q", id, desc.Availability.Reason)
		}
	}
}

func TestRegisterAllUnavailableWhenRequiredToolMissing(t *testing.T) {
	reg := registry.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))

	if err := RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: declaredRelurpicIDs,
	}); err != nil {
		t.Fatalf("register all: %v", err)
	}

	targeted, ok := reg.GetCapability("euclo:cap.targeted_refactor")
	if !ok {
		t.Fatal("expected targeted_refactor capability to be registered")
	}
	if targeted.Availability.Available {
		t.Fatal("expected targeted_refactor to be unavailable when file_write is missing")
	}
	if targeted.Availability.Reason == "" || !strings.Contains(targeted.Availability.Reason, "file_write") {
		t.Fatalf("expected availability reason to mention missing file_write dependency, got %q", targeted.Availability.Reason)
	}
	if _, ok := reg.GetCapability("euclo:cap.ast_query"); !ok {
		t.Fatal("expected ast_query capability to be registered")
	}
	astQuery, _ := reg.GetCapability("euclo:cap.ast_query")
	if !astQuery.Availability.Available {
		t.Fatalf("expected ast_query to remain available with file_read present, got %q", astQuery.Availability.Reason)
	}
}

func TestComputeAvailability_EmptyRequirements(t *testing.T) {
	if got := computeAvailability(registry.NewRegistry(), nil); !got.Available {
		t.Fatalf("expected empty requirements to be available, got %#v", got)
	}
}

func TestComputeAvailability_NonCallableToolCounts(t *testing.T) {
	reg := registry.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))
	reg.AddExposurePolicies([]agentspec.CapabilityExposurePolicy{{
		Selector: agentspec.CapabilitySelector{Name: "file_write"},
		Access:   agentspec.CapabilityExposureHidden,
	}})

	got := computeAvailability(reg, []string{"file_write"})
	if got.Available {
		t.Fatal("expected hidden dependency to be treated as unavailable")
	}
	if got.Reason == "" || got.Reason != "tool dependency missing: file_write (not callable)" {
		t.Fatalf("unexpected reason: %q", got.Reason)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
