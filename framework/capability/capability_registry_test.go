package capability

import (
	"context"
	"fmt"
	"testing"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type recordingTelemetry struct {
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.events = append(r.events, event)
}

type capabilityStubTool struct {
	name string
	tags []string
}

func (t capabilityStubTool) Name() string        { return t.name }
func (t capabilityStubTool) Description() string { return "stub" }
func (t capabilityStubTool) Category() string    { return "testing" }
func (t capabilityStubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: false}}
}
func (t capabilityStubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t capabilityStubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t capabilityStubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		Executables: []core.ExecutablePermission{{Binary: "git"}},
	}}
}
func (t capabilityStubTool) Tags() []string { return t.tags }

type sessionedCapabilityTool struct {
	name    string
	source  core.CapabilitySource
	message string
}

func (t sessionedCapabilityTool) Name() string        { return t.name }
func (t sessionedCapabilityTool) Description() string { return "sessioned" }
func (t sessionedCapabilityTool) Category() string    { return "testing" }
func (t sessionedCapabilityTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "token", Type: "string"}}
}
func (t sessionedCapabilityTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"message": t.message}}, nil
}
func (t sessionedCapabilityTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t sessionedCapabilityTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t sessionedCapabilityTool) Tags() []string                          { return nil }
func (t sessionedCapabilityTool) CapabilitySource() core.CapabilitySource { return t.source }

type schemaValidatedTool struct {
	name         string
	outputSchema *core.Schema
	result       *core.ToolResult
}

type unavailableCapabilityTool struct {
	name string
}

type invocableCapabilityStub struct {
	desc   core.CapabilityDescriptor
	result *core.ToolResult
}

type promptCapabilityStub struct {
	desc   core.CapabilityDescriptor
	result *core.PromptRenderResult
	calls  int
}

type resourceCapabilityStub struct {
	desc   core.CapabilityDescriptor
	result *core.ResourceReadResult
	calls  int
}

type recordingPrecheck struct {
	calls int
	err   error
}

func (p *recordingPrecheck) Check(core.CapabilityDescriptor, map[string]any) error {
	p.calls++
	return p.err
}

type policyEngineStub struct {
	decision core.PolicyDecision
	err      error
	calls    int
}

func (s *policyEngineStub) Evaluate(context.Context, core.PolicyRequest) (core.PolicyDecision, error) {
	s.calls++
	if s.err != nil {
		return core.PolicyDecision{}, s.err
	}
	return s.decision, nil
}

var _ authorization.PolicyEngine = (*policyEngineStub)(nil)

func (t schemaValidatedTool) Name() string        { return t.name }
func (t schemaValidatedTool) Description() string { return "schema" }
func (t schemaValidatedTool) Category() string    { return "testing" }
func (t schemaValidatedTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t schemaValidatedTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return t.result, nil
}
func (t schemaValidatedTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t schemaValidatedTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t schemaValidatedTool) Tags() []string { return nil }
func (t schemaValidatedTool) CapabilityDescriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:          "tool:" + t.name,
		Kind:        core.CapabilityKindTool,
		Name:        t.name,
		Description: "schema",
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"path": {Type: "string"},
			},
			Required: []string{"path"},
		},
		OutputSchema: t.outputSchema,
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass: core.TrustClassBuiltinTrusted,
	}
}

func (t unavailableCapabilityTool) Name() string        { return t.name }
func (t unavailableCapabilityTool) Description() string { return "unavailable" }
func (t unavailableCapabilityTool) Category() string    { return "testing" }
func (t unavailableCapabilityTool) Parameters() []core.ToolParameter {
	return nil
}
func (t unavailableCapabilityTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t unavailableCapabilityTool) IsAvailable(context.Context, *core.Context) bool { return false }
func (t unavailableCapabilityTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t unavailableCapabilityTool) Tags() []string { return nil }

func (s invocableCapabilityStub) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return s.desc
}

func (s invocableCapabilityStub) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return s.result, nil
}

func (s *promptCapabilityStub) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return s.desc
}

func (s *promptCapabilityStub) RenderPrompt(context.Context, *core.Context, map[string]interface{}) (*core.PromptRenderResult, error) {
	s.calls++
	return s.result, nil
}

func (s *resourceCapabilityStub) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return s.desc
}

func (s *resourceCapabilityStub) ReadResource(context.Context, *core.Context) (*core.ResourceReadResult, error) {
	s.calls++
	return s.result, nil
}

func providerInvocableCapability(name, providerID, sessionID, message string) invocableCapabilityStub {
	return invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "provider:" + name,
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Name:          name,
			Source: core.CapabilitySource{
				ProviderID: providerID,
				Scope:      core.CapabilityScopeProvider,
				SessionID:  sessionID,
			},
			TrustClass:   core.TrustClassProviderLocalUntrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"message": message}},
	}
}

type telemetryRecordingSink struct {
	events []core.Event
}

func (s *telemetryRecordingSink) Emit(event core.Event) {
	s.events = append(s.events, event)
}

func TestAllCapabilitiesExposesDescriptors(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git", tags: []string{core.TagExecute}}))

	caps := registry.AllCapabilities()
	require.Len(t, caps, 1)
	require.Equal(t, "tool:cli_git", caps[0].ID)
	require.Contains(t, caps[0].RiskClasses, core.RiskClassExecute)
	require.Contains(t, caps[0].EffectClasses, core.EffectClassProcessSpawn)
}

func TestInvocableCapabilityRespectsCapabilityPolicy(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:review",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "review",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		CapabilityPolicies: []core.CapabilityPolicy{
			{
				Selector: core.CapabilitySelector{Name: "review"},
				Execute:  core.AgentPermissionDeny,
			},
		},
	})

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "relurpic:review", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "selector policy")
}

func TestInvocableCapabilityTelemetryIncludesRuntimeFamily(t *testing.T) {
	registry := NewCapabilityRegistry()
	sink := &telemetryRecordingSink{}
	registry.UseTelemetry(sink)
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "provider:catalog",
			Kind:          core.CapabilityKindResource,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Name:          "catalog",
			TrustClass:    core.TrustClassRemoteDeclared,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "provider:catalog", nil)
	require.NoError(t, err)
	require.Len(t, sink.events, 3)
	require.Equal(t, core.EventCapabilityCall, sink.events[1].Type)
	require.Equal(t, "provider", sink.events[1].Metadata["runtime_family"])
	require.Equal(t, "provider:catalog", sink.events[1].Metadata["capability_id"])
	require.Equal(t, core.EventCapabilityResult, sink.events[2].Type)
	require.Equal(t, "provider", sink.events[2].Metadata["runtime_family"])
	_, durationOK := sink.events[2].Metadata["duration_ms"]
	require.True(t, durationOK)
}

func TestInvocableCapabilitiesIncludesNonToolHandlers(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:review",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "review",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	invocables := registry.InvocableCapabilities()
	require.Len(t, invocables, 1)
	require.Equal(t, "relurpic:review", invocables[0].ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, invocables[0].RuntimeFamily)
}

func TestRelurpicInvocableCapabilityIsCallableByDefault(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:review",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "review",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	capability, ok := registry.GetCapability("relurpic:review")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureCallable, registry.EffectiveExposure(capability))
	require.Len(t, registry.CallableCapabilities(), 1)
}

func TestProviderInvocableCapabilityIsInspectableByDefault(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "provider:catalog",
			Kind:          core.CapabilityKindResource,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Name:          "catalog",
			Source: core.CapabilitySource{
				Scope:      core.CapabilityScopeProvider,
				ProviderID: "catalog-provider",
			},
			TrustClass:   core.TrustClassProviderLocalUntrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	capability, ok := registry.GetCapability("provider:catalog")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureInspectable, registry.EffectiveExposure(capability))
	require.Empty(t, registry.CallableCapabilities())
	require.Len(t, registry.InspectableCapabilities(), 1)
}

func TestRuntimeFamilyCapabilitySelectorMatchesDescriptor(t *testing.T) {
	desc := core.CapabilityDescriptor{
		ID:            "relurpic:review",
		Kind:          core.CapabilityKindPrompt,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "review",
	}

	require.True(t, core.SelectorMatchesDescriptor(core.CapabilitySelector{
		RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic},
	}, desc))
	require.False(t, core.SelectorMatchesDescriptor(core.CapabilitySelector{
		RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
	}, desc))
}

func TestCoordinationCapabilitySelectorMatchesDescriptor(t *testing.T) {
	longRunning := true
	directInsertion := false
	desc := core.CapabilityDescriptor{
		ID:            "prompt:review",
		Kind:          core.CapabilityKindPrompt,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "review",
		Coordination: &core.CoordinationTargetMetadata{
			Target:                 true,
			Role:                   core.CoordinationRoleReviewer,
			TaskTypes:              []string{"review", "verify"},
			ExecutionModes:         []core.CoordinationExecutionMode{core.CoordinationExecutionModeBackgroundAgent},
			LongRunning:            true,
			DirectInsertionAllowed: false,
		},
	}

	require.True(t, core.SelectorMatchesDescriptor(core.CapabilitySelector{
		CoordinationRoles:           []core.CoordinationRole{core.CoordinationRoleReviewer},
		CoordinationTaskTypes:       []string{"review"},
		CoordinationExecutionModes:  []core.CoordinationExecutionMode{core.CoordinationExecutionModeBackgroundAgent},
		CoordinationLongRunning:     &longRunning,
		CoordinationDirectInsertion: &directInsertion,
	}, desc))
	require.False(t, core.SelectorMatchesDescriptor(core.CapabilitySelector{
		CoordinationRoles: []core.CoordinationRole{core.CoordinationRolePlanner},
	}, desc))
}

func TestRegisterInvocableCapabilityRejectsInvalidCoordinationMetadata(t *testing.T) {
	registry := NewCapabilityRegistry()
	err := registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:invalid-target",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "invalid-target",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:         true,
				Role:           core.CoordinationRoleReviewer,
				TaskTypes:      []string{"review"},
				ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
				LongRunning:    true,
			},
		},
		result: &core.ToolResult{Success: true},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "long-running")
}

func TestCoordinationTargetsReturnsOnlyAdmittedNonHiddenTargets(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "planner.plan",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:         true,
				Role:           core.CoordinationRolePlanner,
				TaskTypes:      []string{"plan"},
				ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
			},
		},
		result: &core.ToolResult{Success: true},
	}))
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:reviewer.review",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "reviewer.review",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:         true,
				Role:           core.CoordinationRoleReviewer,
				TaskTypes:      []string{"review"},
				ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
			},
		},
		result: &core.ToolResult{Success: true},
	}))
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:internal.helper",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "internal.helper",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))

	registry.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{Name: "reviewer.review"},
		Access:   core.CapabilityExposureHidden,
	}})

	targets := registry.CoordinationTargets()
	require.Len(t, targets, 1)
	require.Equal(t, "planner.plan", targets[0].Name)

	planners := registry.CoordinationTargets(core.CapabilitySelector{
		CoordinationRoles: []core.CoordinationRole{core.CoordinationRolePlanner},
	})
	require.Len(t, planners, 1)
	require.Equal(t, "planner.plan", planners[0].Name)

	reviewer, ok := registry.GetCoordinationTarget("reviewer.review")
	require.False(t, ok)
	require.Equal(t, core.CapabilityDescriptor{}, reviewer)

	planner, ok := registry.GetCoordinationTarget("planner.plan")
	require.True(t, ok)
	require.Equal(t, "relurpic:planner.plan", planner.ID)
}

func TestCapabilityPoliciesUseExplicitRiskClasses(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))

	registry.UpdateClassPolicy(string(core.RiskClassExecute), AgentPermissionDeny)
	tool, ok := registry.Get("cli_git")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "capability policy")
}

func TestCapabilitySelectorPolicyMatchesDescriptorMetadata(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		CapabilityPolicies: []core.CapabilityPolicy{
			{
				Selector: core.CapabilitySelector{
					Kind:        core.CapabilityKindTool,
					RiskClasses: []core.RiskClass{core.RiskClassExecute},
				},
				Execute: core.AgentPermissionDeny,
			},
		},
	})

	tool, ok := registry.Get("cli_git")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "selector policy")
}

func TestCapturePolicySnapshotClonesRegistryPolicies(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	registry.UseAgentSpec("agent-1", &AgentRuntimeSpec{
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
		CapabilityPolicies: []core.CapabilityPolicy{
			{
				Selector: core.CapabilitySelector{Kind: core.CapabilityKindTool},
				Execute:  core.AgentPermissionAsk,
			},
		},
		InsertionPolicies: []core.CapabilityInsertionPolicy{
			{
				Selector: core.CapabilitySelector{Name: "cli_git"},
				Action:   core.InsertionActionMetadataOnly,
			},
		},
		GlobalPolicies: map[string]core.AgentPermissionLevel{
			string(core.RiskClassExecute): core.AgentPermissionDeny,
		},
		ProviderPolicies: map[string]core.ProviderPolicy{
			"remote-mcp": {Activate: core.AgentPermissionAsk},
		},
		RuntimeSafety: &core.RuntimeSafetySpec{
			MaxCallsPerCapability: 2,
		},
	})
	registry.RevokeProvider("remote-mcp", "quarantined")

	snapshot := registry.CapturePolicySnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, "agent-1", snapshot.AgentID)
	require.Equal(t, AgentPermissionAsk, snapshot.ToolPolicies["cli_git"].Execute)
	require.Len(t, snapshot.CapabilityPolicies, 1)
	require.Len(t, snapshot.InsertionPolicies, 1)
	require.Equal(t, core.AgentPermissionDeny, snapshot.GlobalPolicies[string(core.RiskClassExecute)])
	require.Equal(t, core.AgentPermissionAsk, snapshot.ProviderPolicies["remote-mcp"].Activate)
	require.NotNil(t, snapshot.RuntimeSafety)
	require.Equal(t, 2, snapshot.RuntimeSafety.MaxCallsPerCapability)
	require.Equal(t, "quarantined", snapshot.Revocations.Providers["remote-mcp"])

	snapshot.ToolPolicies["cli_git"] = ToolPolicy{Execute: AgentPermissionAllow}
	require.Equal(t, AgentPermissionAsk, registry.GetToolPolicies()["cli_git"].Execute)
}

func TestCapturePolicySnapshotReflectsLivePolicyUpdates(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	registry.UseAgentSpec("agent-1", &AgentRuntimeSpec{
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
		GlobalPolicies: map[string]core.AgentPermissionLevel{
			string(core.RiskClassExecute): core.AgentPermissionAsk,
		},
	})

	registry.UpdateToolPolicy("cli_git", ToolPolicy{Execute: AgentPermissionDeny})
	registry.UpdateClassPolicy(string(core.RiskClassExecute), core.AgentPermissionDeny)

	snapshot := registry.CapturePolicySnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, AgentPermissionDeny, snapshot.ToolPolicies["cli_git"].Execute)
	require.Equal(t, core.AgentPermissionDeny, snapshot.GlobalPolicies[string(core.RiskClassExecute)])
	require.Equal(t, AgentPermissionDeny, registry.GetToolPolicies()["cli_git"].Execute)
	require.Equal(t, core.AgentPermissionDeny, registry.GetClassPolicies()[string(core.RiskClassExecute)])
}

func TestToolWrapperRemainsStableAcrossPolicyUpdates(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))

	before, ok := registry.Get("cli_git")
	require.True(t, ok)

	registry.UseAgentSpec("agent-1", &AgentRuntimeSpec{
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
	})
	registry.UpdateToolPolicy("cli_git", ToolPolicy{Execute: AgentPermissionDeny})
	registry.UseTelemetry(&recordingTelemetry{})

	after, ok := registry.Get("cli_git")
	require.True(t, ok)
	require.Same(t, before, after)
}

func TestInstrumentedToolAttachesApprovalBindingMetadata(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "file_read"}))

	tool, ok := registry.Get("file_read")
	require.True(t, ok)

	state := core.NewContext()
	state.Set("task.id", "task-1")
	result, err := tool.Execute(context.Background(), state, map[string]interface{}{"path": "README.md"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Metadata)

	rawBinding, ok := result.Metadata["approval_binding"]
	require.True(t, ok)
	binding, ok := rawBinding.(*core.ApprovalBinding)
	require.True(t, ok)
	require.Equal(t, "README.md", binding.TargetResource)
	require.Equal(t, "task-1", binding.TaskID)

	rawDesc, ok := result.Metadata["capability_descriptor"]
	require.True(t, ok)
	desc, ok := rawDesc.(core.CapabilityDescriptor)
	require.True(t, ok)
	require.Equal(t, "tool:file_read", desc.ID)
}

func TestRegisterCapabilityStoresNonToolDescriptors(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:          "prompt:skill:1",
		Kind:        core.CapabilityKindPrompt,
		Name:        "skill.prompt.1",
		Description: "Prompt capability",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
	}))

	capability, ok := registry.GetCapability("prompt:skill:1")
	require.True(t, ok)
	require.Equal(t, core.CapabilityKindPrompt, capability.Kind)

	all := registry.AllCapabilities()
	require.Len(t, all, 1)
	require.Equal(t, "prompt:skill:1", all[0].ID)
}

func TestRegisterInvocableCapabilityInvokesByIDAndName(t *testing.T) {
	registry := NewCapabilityRegistry()
	handler := invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:          "prompt:runtime.echo",
			Kind:        core.CapabilityKindPrompt,
			Name:        "runtime.echo",
			Description: "Runtime-backed capability",
			Source: core.CapabilitySource{
				Scope: core.CapabilityScopeWorkspace,
			},
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		result: &core.ToolResult{
			Success: true,
			Data:    map[string]interface{}{"message": "ok"},
		},
	}

	require.NoError(t, registry.RegisterInvocableCapability(handler))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "prompt:runtime.echo", nil)
	require.NoError(t, err)
	require.Equal(t, "ok", result.Data["message"])

	result, err = registry.InvokeCapability(context.Background(), core.NewContext(), "runtime.echo", nil)
	require.NoError(t, err)
	require.Equal(t, "ok", result.Data["message"])
}

func TestRegisterPromptCapabilityRendersByIDAndName(t *testing.T) {
	registry := NewCapabilityRegistry()
	handler := &promptCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:          "prompt:runtime.summary",
			Kind:        core.CapabilityKindPrompt,
			Name:        "runtime.summary",
			Description: "Runtime prompt",
			Source: core.CapabilitySource{
				Scope: core.CapabilityScopeWorkspace,
			},
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		result: &core.PromptRenderResult{
			Messages: []core.PromptMessage{{
				Content: []core.ContentBlock{core.TextContentBlock{Text: "summary prompt"}},
			}},
		},
	}

	require.NoError(t, registry.RegisterPromptCapability(handler))

	rendered, err := registry.RenderPrompt(context.Background(), core.NewContext(), "prompt:runtime.summary", nil)
	require.NoError(t, err)
	require.Equal(t, "summary prompt", rendered.Messages[0].Content[0].(core.TextContentBlock).Text)

	rendered, err = registry.RenderPrompt(context.Background(), core.NewContext(), "runtime.summary", nil)
	require.NoError(t, err)
	require.Equal(t, "summary prompt", rendered.Messages[0].Content[0].(core.TextContentBlock).Text)
}

func TestRegisterResourceCapabilityReadsByIDAndName(t *testing.T) {
	registry := NewCapabilityRegistry()
	handler := &resourceCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:          "resource:workspace.docs",
			Kind:        core.CapabilityKindResource,
			Name:        "workspace.docs",
			Description: "Workspace docs",
			Source: core.CapabilitySource{
				Scope: core.CapabilityScopeWorkspace,
			},
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		result: &core.ResourceReadResult{
			Contents: []core.ContentBlock{core.TextContentBlock{Text: "guide"}},
		},
	}

	require.NoError(t, registry.RegisterResourceCapability(handler))

	resource, err := registry.ReadResource(context.Background(), core.NewContext(), "resource:workspace.docs")
	require.NoError(t, err)
	require.Equal(t, "guide", resource.Contents[0].(core.TextContentBlock).Text)

	resource, err = registry.ReadResource(context.Background(), core.NewContext(), "workspace.docs")
	require.NoError(t, err)
	require.Equal(t, "guide", resource.Contents[0].(core.TextContentBlock).Text)
}

func TestRenderPromptUsesPolicyEngineAndSkipsPrechecksWhenDenied(t *testing.T) {
	registry := NewCapabilityRegistry()
	handler := &promptCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:           "prompt:runtime.summary",
			Kind:         core.CapabilityKindPrompt,
			Name:         "runtime.summary",
			TrustClass:   core.TrustClassRemoteApproved,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.PromptRenderResult{},
	}
	engine := &policyEngineStub{decision: core.PolicyDecisionDeny("blocked")}
	precheck := &recordingPrecheck{}
	registry.SetPolicyEngine(engine)
	registry.AddPrecheck(precheck)
	require.NoError(t, registry.RegisterPromptCapability(handler))

	_, err := registry.RenderPrompt(context.Background(), core.NewContext(), "runtime.summary", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked")
	require.Equal(t, 1, engine.calls)
	require.Equal(t, 0, precheck.calls)
	require.Equal(t, 0, handler.calls)
}

func TestReadResourceRunsSharedPrechecks(t *testing.T) {
	registry := NewCapabilityRegistry()
	handler := &resourceCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:           "resource:workspace.docs",
			Kind:         core.CapabilityKindResource,
			Name:         "workspace.docs",
			TrustClass:   core.TrustClassWorkspaceTrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ResourceReadResult{},
	}
	precheck := &recordingPrecheck{err: fmt.Errorf("blocked by precheck")}
	registry.AddPrecheck(precheck)
	require.NoError(t, registry.RegisterResourceCapability(handler))

	_, err := registry.ReadResource(context.Background(), core.NewContext(), "workspace.docs")
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked by precheck")
	require.Equal(t, 1, precheck.calls)
	require.Equal(t, 0, handler.calls)
}

func TestInvokeCapabilitySupportsProviderCapabilitiesWithoutLegacyToolAdapter(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("remote_echo", "remote-mcp", "session-1", "adapted")))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "remote_echo", map[string]interface{}{"token": "secret"})
	require.NoError(t, err)
	require.Equal(t, "adapted", result.Data["message"])
	require.NotNil(t, result.Metadata)
	desc, ok := result.Metadata["capability_descriptor"].(core.CapabilityDescriptor)
	require.True(t, ok)
	require.Equal(t, "provider:remote_echo", desc.ID)
}

func TestInvokeCapabilityRejectsUnavailableLegacyTool(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(unavailableCapabilityTool{name: "offline_tool"}))

	require.False(t, registry.CapabilityAvailable(context.Background(), core.NewContext(), "offline_tool"))
	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "offline_tool", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unavailable")
}

func TestRegisterLegacyToolRejectsNonLocalRuntimeFamily(t *testing.T) {
	registry := NewCapabilityRegistry()
	err := registry.Register(sessionedCapabilityTool{
		name: "remote_echo",
		source: core.CapabilitySource{
			ProviderID: "remote-mcp",
			Scope:      core.CapabilityScopeRemote,
			SessionID:  "session-1",
		},
		message: "blocked",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "local-tool runtime family")
}

func TestModelCallableLLMToolSpecsIncludesProviderCapability(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("remote_echo", "remote-mcp", "session-1", "adapted")))
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		Mode:  core.AgentModePrimary,
		Model: core.AgentModelConfig{Provider: "test", Name: "test"},
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{Name: "remote_echo", RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider}},
			Access:   core.CapabilityExposureCallable,
		}},
	})

	// Local tool list excludes non-local capabilities.
	require.Empty(t, registry.CallableTools())
	// GetModelTool only resolves local tools; non-local capabilities are not returned.
	_, ok := registry.GetModelTool("remote_echo")
	require.False(t, ok)
	// Non-local invocable capabilities appear in the LLM tool spec list.
	specs := registry.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "remote_echo", specs[0].Name)
	// Invocation goes through InvokeCapability, not through a Tool shim.
	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "remote_echo", map[string]interface{}{"token": "secret"})
	require.NoError(t, err)
	require.Equal(t, "adapted", result.Data["message"])
}

func TestModelCallableLLMToolSpecsIncludesRelurpicCapability(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "planner.plan",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"message": "planned"}},
	}))

	require.Empty(t, registry.CallableTools())
	_, ok := registry.GetModelTool("planner.plan")
	require.False(t, ok)
	specs := registry.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "planner.plan", specs[0].Name)
	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "planner.plan", nil)
	require.NoError(t, err)
	require.Equal(t, "planned", result.Data["message"])
}

func TestExposurePolicyCanElevateProviderCapabilityToCallable(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("catalog", "remote-mcp", "session-1", "listed")))

	capability, ok := registry.GetCapability("provider:catalog")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureInspectable, registry.EffectiveExposure(capability))
	require.Empty(t, registry.CallableCapabilities())

	registry.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{
			Name:            "catalog",
			RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
		},
		Access: core.CapabilityExposureCallable,
	}})

	require.Equal(t, core.CapabilityExposureCallable, registry.EffectiveExposure(capability))
	require.Len(t, registry.CallableCapabilities(), 1)
	specs := registry.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "catalog", specs[0].Name)
}

func TestLaterExposurePolicyOverridesEarlierExposurePolicy(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("catalog", "remote-mcp", "session-1", "listed")))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		Mode:  core.AgentModePrimary,
		Model: core.AgentModelConfig{Provider: "test", Name: "test"},
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{
				Name:            "catalog",
				RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
			},
			Access: core.CapabilityExposureHidden,
		}},
	})
	registry.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{
			Name:            "catalog",
			RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
		},
		Access: core.CapabilityExposureCallable,
	}})

	capability, ok := registry.GetCapability("provider:catalog")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureCallable, registry.EffectiveExposure(capability))
	specs := registry.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "catalog", specs[0].Name)
}

func TestUseAgentSpecBrowserEnabledMakesProviderBrowserCallable(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "provider:browser",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Name:          "browser",
			Source: core.CapabilitySource{
				Scope:      core.CapabilityScopeProvider,
				ProviderID: "browser",
			},
			TrustClass:   core.TrustClassProviderLocalUntrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"message": "browser ok"}},
	}))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		Mode:    core.AgentModePrimary,
		Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
		Browser: &core.AgentBrowserSpec{Enabled: true},
	})

	capability, ok := registry.GetCapability("provider:browser")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureCallable, registry.EffectiveExposure(capability))
	require.Empty(t, registry.CallableTools())
	specs := registry.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "browser", specs[0].Name)
	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "browser", nil)
	require.NoError(t, err)
	require.Equal(t, "browser ok", result.Data["message"])
}

func TestCloneFilteredRemovesCapabilityEntriesForDroppedTools(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_rg"}))

	clone := registry.CloneFiltered(func(tool Tool) bool {
		return tool.Name() == "cli_git"
	})

	_, ok := clone.Get("cli_rg")
	require.False(t, ok)
	_, ok = clone.GetCapability("tool:cli_rg")
	require.False(t, ok)
	_, err := clone.InvokeCapability(context.Background(), core.NewContext(), "tool:cli_rg", nil)
	require.Error(t, err)

	result, err := clone.InvokeCapability(context.Background(), core.NewContext(), "tool:cli_git", nil)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestInvokeCapabilityPrecheckBlocksInvocation(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:           "relurpic:writer",
			Kind:         core.CapabilityKindTool,
			Name:         "writer",
			TrustClass:   core.TrustClassBuiltinTrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))
	registry.AddPrecheck(&recordingPrecheck{err: fmt.Errorf("blocked by precheck")})

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "relurpic:writer", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked by precheck")
}

func TestInvokeCapabilitySkipsPrecheckWhenPolicyEngineDenies(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:           "relurpic:writer",
			Kind:         core.CapabilityKindTool,
			Name:         "writer",
			TrustClass:   core.TrustClassBuiltinTrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))
	engine := &policyEngineStub{decision: core.PolicyDecisionDeny("denied by policy")}
	precheck := &recordingPrecheck{}
	registry.SetPolicyEngine(engine)
	registry.AddPrecheck(precheck)

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "relurpic:writer", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied by policy")
	require.Equal(t, 1, engine.calls)
	require.Equal(t, 0, precheck.calls)
}

func TestCloneFilteredCopiesPrechecks(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	precheck := &recordingPrecheck{err: fmt.Errorf("blocked by precheck")}
	registry.AddPrecheck(precheck)

	clone := registry.CloneFiltered(func(tool Tool) bool {
		return tool.Name() == "cli_git"
	})

	_, err := clone.InvokeCapability(context.Background(), core.NewContext(), "tool:cli_git", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked by precheck")
	require.Equal(t, 1, precheck.calls)
}

func TestProviderCapabilityRegistrarNormalizesProviderBackedCapability(t *testing.T) {
	registry := NewCapabilityRegistry()
	registrar, err := registry.ProviderCapabilityRegistrar(core.ProviderDescriptor{
		ID:            "remote-mcp",
		Kind:          core.ProviderKindMCPClient,
		TrustBaseline: core.TrustClassRemoteDeclared,
		Security: core.ProviderSecurityProfile{
			Origin:                     core.ProviderOriginRemote,
			RequiresFrameworkMediation: true,
		},
	}, core.ProviderPolicy{
		DefaultTrust: core.TrustClassRemoteApproved,
	})
	require.NoError(t, err)

	err = registrar.RegisterCapability(core.CapabilityDescriptor{
		ID:         "tool:remote.search",
		Kind:       core.CapabilityKindTool,
		Name:       "remote.search",
		TrustClass: core.TrustClassBuiltinTrusted,
	})
	require.NoError(t, err)

	capability, ok := registry.GetCapability("tool:remote.search")
	require.True(t, ok)
	require.Equal(t, core.CapabilityScopeRemote, capability.Source.Scope)
	require.Equal(t, "remote-mcp", capability.Source.ProviderID)
	require.Equal(t, core.TrustClassRemoteApproved, capability.TrustClass)
	require.Equal(t, core.CapabilityExposureInspectable, registry.EffectiveExposure(capability))
}

func TestProviderCapabilityRegistrarPreventsTrustEscalation(t *testing.T) {
	registry := NewCapabilityRegistry()
	registrar, err := registry.ProviderCapabilityRegistrar(core.ProviderDescriptor{
		ID:            "browser",
		Kind:          core.ProviderKindAgentRuntime,
		TrustBaseline: core.TrustClassProviderLocalUntrusted,
		Security: core.ProviderSecurityProfile{
			Origin:                     core.ProviderOriginLocal,
			RequiresFrameworkMediation: true,
		},
	}, core.ProviderPolicy{})
	require.NoError(t, err)

	err = registrar.RegisterCapability(core.CapabilityDescriptor{
		ID:         "resource:browser.page",
		Kind:       core.CapabilityKindResource,
		Name:       "browser.page",
		TrustClass: core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)

	capability, ok := registry.GetCapability("resource:browser.page")
	require.True(t, ok)
	require.Equal(t, core.TrustClassProviderLocalUntrusted, capability.TrustClass)
	require.Equal(t, core.CapabilityScopeProvider, capability.Source.Scope)
}

func TestRuntimeSafetyRevocationBlocksCapabilityExecution(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	registry.RevokeCapability("tool:cli_git", "manual quarantine")

	tool, ok := registry.Get("cli_git")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "revoked")
}

func TestRuntimeSafetyCallBudgetBlocksRepeatedExecution(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		RuntimeSafety: &core.RuntimeSafetySpec{MaxCallsPerCapability: 1},
	})

	tool, ok := registry.Get("cli_git")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), nil)
	require.NoError(t, err)
	_, err = tool.Execute(context.Background(), core.NewContext(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "call budget exceeded")
}

func TestRuntimeSafetySessionBudgetBlocksLargeResults(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("remote_echo", "remote-mcp", "session-1", "abcdefghijklmnopqrstuvwxyz")))
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		RuntimeSafety: &core.RuntimeSafetySpec{MaxBytesPerSession: 10},
	})

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "remote_echo", map[string]interface{}{"token": "secret"})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "byte budget exceeded")
}

func TestRuntimeSafetyTelemetryRedactsSensitiveMetadata(t *testing.T) {
	registry := NewCapabilityRegistry()
	telemetry := &recordingTelemetry{}
	registry.UseTelemetry(telemetry)
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("remote_echo", "remote-mcp", "session-1", "ok")))

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "remote_echo", map[string]interface{}{"token": "super-secret"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(telemetry.events), 2)
	args := telemetry.events[1].Metadata["args"].(map[string]interface{})
	require.Equal(t, "[REDACTED]", args["token"])
}

func TestRuntimeSafetySessionSubprocessBudgetBlocksOverage(t *testing.T) {
	registry := NewCapabilityRegistry()
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		RuntimeSafety: &core.RuntimeSafetySpec{MaxSubprocessesPerSession: 1},
	})

	require.NoError(t, registry.RecordSessionSubprocess("session-1", 1))
	err := registry.RecordSessionSubprocess("session-1", 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "subprocess budget exceeded")
}

func TestRuntimeSafetySessionNetworkBudgetBlocksOverage(t *testing.T) {
	registry := NewCapabilityRegistry()
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		RuntimeSafety: &core.RuntimeSafetySpec{MaxNetworkRequestsSession: 1},
	})

	require.NoError(t, registry.RecordSessionNetworkRequest("session-1", 1))
	err := registry.RecordSessionNetworkRequest("session-1", 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "network request budget exceeded")
}

func TestRegisterCapabilityEmitsAdmissionSecurityEvent(t *testing.T) {
	registry := NewCapabilityRegistry()
	telemetry := &recordingTelemetry{}
	registry.UseTelemetry(telemetry)

	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:   "prompt:skill:1",
		Kind: core.CapabilityKindPrompt,
		Name: "skill.prompt.1",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
	}))

	require.NotEmpty(t, telemetry.events)
	require.Equal(t, "capability_admitted", telemetry.events[0].Metadata["security_event"])
}

func TestUseAgentSpecEmitsExposureSecurityEvent(t *testing.T) {
	registry := NewCapabilityRegistry()
	telemetry := &recordingTelemetry{}
	registry.UseTelemetry(telemetry)
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{
			{
				Selector: core.CapabilitySelector{Name: "cli_git"},
				Access:   core.CapabilityExposureInspectable,
			},
		},
	})

	found := false
	for _, event := range telemetry.events {
		if event.Metadata["security_event"] == "capability_exposure_resolved" {
			found = true
			require.Equal(t, "inspectable", event.Metadata["exposure"])
		}
	}
	require.True(t, found)
}

func TestToolExecutionRejectsInputSchemaMismatch(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(schemaValidatedTool{
		name:   "schema_echo",
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"path": "ok"}},
	}))

	tool, ok := registry.Get("schema_echo")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": 42})
	require.Error(t, err)
	require.Contains(t, err.Error(), "input schema invalid")
}

func TestToolExecutionRejectsOutputSchemaMismatch(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(schemaValidatedTool{
		name: "schema_echo",
		outputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"path": {Type: "string"},
			},
			Required: []string{"path"},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"path": 42}},
	}))

	tool, ok := registry.Get("schema_echo")
	require.True(t, ok)
	result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": "ok"})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "output schema invalid")
}

func TestInspectableExposureExcludesToolFromCallableSet(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "cli_git"}))
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{
			{
				Selector: core.CapabilitySelector{Name: "cli_git"},
				Access:   core.CapabilityExposureInspectable,
			},
		},
	})

	require.Len(t, registry.All(), 0)
	require.Len(t, registry.CallableTools(), 0)
	require.Len(t, registry.InspectableTools(), 1)
	require.Len(t, registry.AllCapabilities(), 1)
	require.Equal(t, core.CapabilityExposureInspectable, registry.EffectiveExposure(registry.AllCapabilities()[0]))
}

func TestHiddenExposureRemovesCapabilityFromCatalogs(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:          "prompt:hidden:1",
		Kind:        core.CapabilityKindPrompt,
		Name:        "hidden.prompt",
		Description: "Hidden prompt",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
	}))
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{
			{
				Selector: core.CapabilitySelector{Kind: core.CapabilityKindPrompt},
				Access:   core.CapabilityExposureHidden,
			},
		},
	})

	require.Empty(t, registry.AllCapabilities())
	capability, ok := registry.GetCapability("prompt:hidden:1")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureHidden, registry.EffectiveExposure(capability))
}
