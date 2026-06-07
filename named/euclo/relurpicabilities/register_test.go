package relurpicabilities

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
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
	env := agentenv.WorkspaceEnvironment{
		Registry: nil,
	}

	err := RegisterAll(env, declaredRelurpicIDs)
	if err == nil {
		t.Fatal("expected error when registry is nil, got nil")
	}

	if err.Error() != "capability registry is nil" {
		t.Fatalf("expected 'capability registry is nil' error, got: %v", err)
	}
}

func TestRegisterAllRejectsUnknownDeclaration(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: []string{
					"euclo:cap.test_run",
					"euclo:cap.does_not_exist",
				}},
			},
		},
		Registry: capability.NewRegistry(),
	}

	err := RegisterAll(env, []string{"euclo:cap.test_run", "euclo:cap.does_not_exist"})
	if err == nil {
		t.Fatal("expected unknown declaration to fail")
	}
	if !strings.Contains(err.Error(), "unknown relurpic capability declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterAllValidRegistryNoError(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, declaredRelurpicIDs...)},
			},
		},
		Registry: capability.NewRegistry(),
	}

	err := RegisterAll(env, declaredRelurpicIDs)
	if err != nil {
		t.Fatalf("expected no error with valid registry, got: %v", err)
	}
}

func TestRegisterAllEmptyToolRegistryMarksAllUnavailable(t *testing.T) {
	reg := capability.NewRegistry()
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, declaredRelurpicIDs...)},
			},
		},
		Registry: reg,
	}
	if err := RegisterAll(env, declaredRelurpicIDs); err != nil {
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
	reg := capability.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))

	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, declaredRelurpicIDs...)},
			},
		},
		Registry: reg,
	}
	if err := RegisterAll(env, declaredRelurpicIDs); err != nil {
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
	reg := capability.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_read", available: true}))

	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, declaredRelurpicIDs...)},
			},
		},
		Registry: reg,
	}
	if err := RegisterAll(env, declaredRelurpicIDs); err != nil {
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
	if got := computeAvailability(capability.NewRegistry(), nil); !got.Available {
		t.Fatalf("expected empty requirements to be available, got %#v", got)
	}
}

func TestComputeAvailability_NonCallableToolCounts(t *testing.T) {
	reg := capability.NewRegistry()
	requireNoError(t, reg.RegisterLegacyTool(availabilityTool{name: "file_write", available: true}))
	reg.AddExposurePolicies([]capability.CapabilityExposurePolicy{{
		Selector: agentspec.CapabilitySelector{Name: "file_write"},
		Access:   capability.CapabilityExposureHidden,
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
