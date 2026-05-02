package relurpicabilities

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type availabilityTool struct {
	name      string
	available bool
}

func (t availabilityTool) Name() string        { return t.name }
func (t availabilityTool) Description() string { return t.name }
func (t availabilityTool) Category() string    { return "test" }
func (t availabilityTool) Parameters() []contracts.ToolParameter {
	return nil
}
func (t availabilityTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return &contracts.ToolResult{Success: true}, nil
}
func (t availabilityTool) IsAvailable(ctx context.Context) bool { return t.available }
func (t availabilityTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}
func (t availabilityTool) Tags() []string { return []string{"test"} }

func TestRegisterAllNilRegistryErrors(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Registry: nil,
	}

	err := RegisterAll(env)
	if err == nil {
		t.Fatal("expected error when registry is nil, got nil")
	}

	if err.Error() != "capability registry is nil" {
		t.Fatalf("expected 'capability registry is nil' error, got: %v", err)
	}
}

func TestRegisterAllValidRegistryNoError(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Registry: capability.NewCapabilityRegistry(),
	}

	err := RegisterAll(env)
	if err != nil {
		t.Fatalf("expected no error with valid registry, got: %v", err)
	}
}

func TestRegisterAllEmptyToolRegistryMarksAllUnavailable(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	env := agentenv.WorkspaceEnvironment{Registry: reg}
	if err := RegisterAll(env); err != nil {
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
	reg := capability.NewCapabilityRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))

	env := agentenv.WorkspaceEnvironment{Registry: reg}
	if err := RegisterAll(env); err != nil {
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
	reg := capability.NewCapabilityRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))

	env := agentenv.WorkspaceEnvironment{Registry: reg}
	if err := RegisterAll(env); err != nil {
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
	if got := computeAvailability(capability.NewCapabilityRegistry(), nil); !got.Available {
		t.Fatalf("expected empty requirements to be available, got %#v", got)
	}
}

func TestComputeAvailability_NonCallableToolCounts(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{Name: "file_write"},
		Access:   core.CapabilityExposureHidden,
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
