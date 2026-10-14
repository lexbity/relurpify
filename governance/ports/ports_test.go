package ports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/sandbox"
)

func TestPrincipalContextRoundTrip(t *testing.T) {
	principal := Principal{
		AgentID: "agent-1",
		Delegation: []DelegationLink{
			{FromAgentID: "root", ToAgentID: "agent-1"},
			{FromAgentID: "agent-1", ToAgentID: "agent-2"},
		},
	}

	ctx := ContextWithPrincipal(context.Background(), principal)

	recovered := PrincipalFromContext(ctx)
	assert.Equal(t, "agent-1", recovered.AgentID)
	require.Len(t, recovered.Delegation, 2)
	assert.Equal(t, "root", recovered.Delegation[0].FromAgentID)
	assert.Equal(t, "agent-2", recovered.Delegation[1].ToAgentID)
}

func TestPrincipalFromContextWithoutPrincipal(t *testing.T) {
	assert.Equal(t, Principal{}, PrincipalFromContext(context.Background()),
		"a context without a principal yields the zero value")

	assert.Equal(t, Principal{}, PrincipalFromContext(ContextWithPrincipal(context.Background(), Principal{})),
		"an explicitly empty principal yields the zero value")
}

func TestActionVocabularyValues(t *testing.T) {
	expected := map[Action]string{
		ActionToolInvoke:  "tool.invoke",
		ActionFileRead:    "file.read",
		ActionFileWrite:   "file.write",
		ActionFileEdit:    "file.edit",
		ActionFileExecute: "file.execute",
		ActionExecBinary:  "exec.binary",
		ActionNetEgress:   "net.egress",
		ActionCapability:  "capability.invoke",
		ActionIPC:         "ipc.communicate",
	}
	for action, want := range expected {
		assert.Equalf(t, want, string(action), "action constant vocabulary changed: %s", want)
	}
}

// recordingRuntime is a minimal SandboxRuntime fake proving the port is
// implementable outside the platform package and that policy round-trips.
type recordingRuntime struct {
	policy   sandbox.SandboxPolicy
	config   sandbox.SandboxConfig
	verified bool
	applied  []sandbox.SandboxPolicy
}

func (r *recordingRuntime) Verify(context.Context) error               { r.verified = true; return nil }
func (r *recordingRuntime) ValidatePolicy(sandbox.SandboxPolicy) error { return nil }
func (r *recordingRuntime) ApplyPolicy(_ context.Context, p sandbox.SandboxPolicy) error {
	r.applied = append(r.applied, p)
	r.policy = p
	return nil
}
func (r *recordingRuntime) Policy() sandbox.SandboxPolicy    { return r.policy }
func (r *recordingRuntime) RunConfig() sandbox.SandboxConfig { return r.config }
func (r *recordingRuntime) Name() string                     { return "recording" }

var _ SandboxRuntime = (*recordingRuntime)(nil)

func TestSandboxRuntimePortRoundTripsPolicy(t *testing.T) {
	runtime := &recordingRuntime{config: sandbox.SandboxConfig{ContainerRuntime: "docker"}}
	policy := sandbox.SandboxPolicy{ProtectedPaths: []string{"/etc"}, ReadOnlyRoot: true}

	require.NoError(t, runtime.Verify(context.Background()))
	require.NoError(t, runtime.ApplyPolicy(context.Background(), policy))

	assert.True(t, runtime.verified)
	assert.Equal(t, policy, runtime.Policy())
	assert.Equal(t, "recording", runtime.Name())
}

// staticSelector is a SandboxSelector fake proving backend selection metadata
// flows through the governance-owned port unchanged.
type staticSelector struct {
	backends []string
	chosen   string
}

func (s *staticSelector) SelectBackend(_ context.Context, backend string, _ sandbox.SandboxConfig, _, _ string) (SandboxRuntime, error) {
	s.chosen = backend
	return &recordingRuntime{}, nil
}
func (s *staticSelector) SupportedBackends() []string { return s.backends }

var _ SandboxSelector = (*staticSelector)(nil)

func TestSandboxSelectorPortPassesBackendName(t *testing.T) {
	selector := &staticSelector{backends: []string{"docker", "gvisor"}}

	runtime, err := selector.SelectBackend(context.Background(), "gvisor", sandbox.SandboxConfig{}, "img", "/ws")

	require.NoError(t, err)
	assert.Equal(t, "gvisor", selector.chosen)
	assert.Equal(t, []string{"docker", "gvisor"}, selector.SupportedBackends())
	assert.Equal(t, "recording", runtime.Name())
}

// staticTask and staticResult prove TaskView/ResultView are structural views
// over execution types.
type staticTask struct{ id, typ, instruction string }

func (s staticTask) ID() string          { return s.id }
func (s staticTask) Type() string        { return s.typ }
func (s staticTask) Instruction() string { return s.instruction }

var _ TaskView = staticTask{}

type staticResult struct {
	success bool
	data    map[string]any
}

func (s staticResult) Success() bool        { return s.success }
func (s staticResult) Data() map[string]any { return s.data }

var _ ResultView = staticResult{}

func TestTaskAndResultViewsExposeTheirFields(t *testing.T) {
	task := staticTask{id: "t-1", typ: "capability", instruction: "run bisect"}
	assert.Equal(t, "t-1", task.ID())
	assert.Equal(t, "capability", task.Type())
	assert.Equal(t, "run bisect", task.Instruction())

	result := staticResult{success: true, data: map[string]any{"out": "ok"}}
	assert.True(t, result.Success())
	assert.Equal(t, map[string]any{"out": "ok"}, result.Data())
}

// staticState and staticSearch prove the read-only context ports.
type staticState struct{ taskID, sessionID string }

func (s staticState) TaskID() string    { return s.taskID }
func (s staticState) SessionID() string { return s.sessionID }

var _ StateView = staticState{}

type globSearch struct{}

func (globSearch) MatchGlob(pattern, target string) bool { return pattern == target }

var _ SearchScope = globSearch{}

func TestStateViewAndSearchScope(t *testing.T) {
	state := staticState{taskID: "task-9", sessionID: "sess-3"}
	assert.Equal(t, "task-9", state.TaskID())
	assert.Equal(t, "sess-3", state.SessionID())

	scope := globSearch{}
	assert.True(t, scope.MatchGlob("exact", "exact"))
	assert.False(t, scope.MatchGlob("*.go", "main.go"))
}

func TestDelegationEntryAndTransitionCarryLifecycleFields(t *testing.T) {
	entry := DelegationEntry{
		DelegationID:   "d-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		TaskID:         "task-1",
		State:          "running",
		TrustClass:     "builtin_trusted",
		Recoverability: "resumable",
		Background:     true,
		Metadata:       map[string]any{"attempt": 2},
	}
	assert.Equal(t, "running", entry.State)
	assert.True(t, entry.Background)

	transition := DelegationTransitionEntry{
		TransitionID: "tr-1",
		DelegationID: "d-1",
		ToState:      "completed",
		CreatedAt:    entry.UpdatedAt,
	}
	assert.Equal(t, "d-1", transition.DelegationID)
	assert.Equal(t, "completed", transition.ToState)

	artifact := WorkflowArtifactRecord{
		ArtifactID:        "a-1",
		StorageKind:       ArtifactStorageInline,
		RawSizeBytes:      42,
		CompressionMethod: "none",
	}
	assert.Equal(t, ArtifactStorageInline, artifact.StorageKind)
	assert.Equal(t, int64(42), artifact.RawSizeBytes)
	assert.Equal(t, "inline", string(ArtifactStorageInline))
	assert.Equal(t, "external", string(ArtifactStorageExternal))
}

func TestManifestAndContractViewsCarryPolicyInputs(t *testing.T) {
	manifest := ManifestView{
		ID:                  "agent-1",
		AllowedCapabilities: []CapabilitySelectorView{{ID: "cap.alpha", Name: "Alpha"}},
		GlobalPolicies:      map[string]string{"net": "deny"},
	}
	assert.Equal(t, "agent-1", manifest.ID)
	assert.Equal(t, "Alpha", manifest.AllowedCapabilities[0].Name)

	snapshot := SandboxPolicyView{ProtectedPaths: []string{"/etc/passwd"}}
	bundle := SecurityBundleView{Sandbox: snapshot}
	assert.Equal(t, []string{"/etc/passwd"}, bundle.Sandbox.ProtectedPaths)

	contract := AgentContractView{
		AgentID: "agent-1",
		Rules:   []PolicyRuleView{{ID: "rule-1", Priority: 10, Effect: "allow"}},
	}
	assert.Equal(t, "allow", contract.Rules[0].Effect)

	coordination := CoordinationSpecView{
		MaxDelegationDepth:        3,
		AllowRemoteDelegation:     true,
		DelegationTargetSelectors: []CapabilitySelectorView{{ID: "cap.target"}},
	}
	assert.Equal(t, 3, coordination.MaxDelegationDepth)
	assert.True(t, coordination.AllowRemoteDelegation)
}

// staticSpec is a SpecView fake: the structural interface capability/agentspec
// satisfies for policy compilation.
type staticSpec struct{}

func (staticSpec) GetToolExecutionPolicy() map[string]ToolPolicyView {
	return map[string]ToolPolicyView{"bash": {Execute: "ask"}}
}
func (staticSpec) GetCapabilityPolicies() []CapabilityPolicyView {
	return []CapabilityPolicyView{{Selector: CapabilitySelectorView{ID: "cap.alpha"}, Execute: "allow"}}
}
func (staticSpec) GetProviderPolicies() map[string]ProviderPolicyView {
	return map[string]ProviderPolicyView{"ollama": {Activate: "on", DefaultTrust: "local"}}
}
func (staticSpec) GetSessionPolicies() []SessionPolicyView {
	return []SessionPolicyView{{ID: "sp-1", Priority: 5, Enabled: true, Effect: "allow"}}
}
func (staticSpec) GetGlobalPolicies() map[string]string { return map[string]string{"mode": "strict"} }
func (staticSpec) GetAllowedCapabilities() []CapabilitySelectorView {
	return []CapabilitySelectorView{{ID: "cap.alpha"}}
}
func (staticSpec) GetOrchestration() OrchestrationConfigView {
	return OrchestrationConfigView{PhaseCapabilities: map[string][]string{"edit": {"cap.alpha"}}}
}

var _ SpecView = staticSpec{}

var _ ManifestSnapshotView = staticSnapshot{}

type staticSnapshot struct{}

func (staticSnapshot) Manifest() ManifestView { return ManifestView{ID: "agent-1"} }
func (staticSnapshot) AgentSpec() SpecView    { return staticSpec{} }

func TestSpecViewExposesAllPolicyInputs(t *testing.T) {
	var spec SpecView = staticSpec{}

	assert.Equal(t, "ask", spec.GetToolExecutionPolicy()["bash"].Execute)
	require.Len(t, spec.GetCapabilityPolicies(), 1)
	assert.Equal(t, "cap.alpha", spec.GetCapabilityPolicies()[0].Selector.ID)
	assert.Equal(t, "local", spec.GetProviderPolicies()["ollama"].DefaultTrust)
	require.Len(t, spec.GetSessionPolicies(), 1)
	assert.Equal(t, 5, spec.GetSessionPolicies()[0].Priority)
	assert.Equal(t, "strict", spec.GetGlobalPolicies()["mode"])
	assert.Equal(t, []string{"cap.alpha"}, spec.GetOrchestration().PhaseCapabilities["edit"])
	require.Len(t, spec.GetAllowedCapabilities(), 1)
	assert.Equal(t, "cap.alpha", spec.GetAllowedCapabilities()[0].ID)

	snapshot := staticSnapshot{}
	assert.Equal(t, "agent-1", snapshot.Manifest().ID)
}
