package capability

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type recordingPostcheck struct {
	calls int
	desc  core.CapabilityDescriptor
	last  *core.ToolResult
	err   error
}

func (p *recordingPostcheck) Record(desc core.CapabilityDescriptor, result *core.ToolResult) error {
	p.calls++
	p.desc = desc
	p.last = result
	return p.err
}

type stubRecoveryBroker struct {
	requests chan RecoveryGuidanceRequest
	choice   string
}

func (s *stubRecoveryBroker) RequestRecovery(_ context.Context, req RecoveryGuidanceRequest) (*RecoveryGuidanceDecision, error) {
	if s.requests != nil {
		s.requests <- req
	}
	return &RecoveryGuidanceDecision{ChoiceID: s.choice}, nil
}

func TestDoomLoopDetectorIdenticalCalls(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	desc := core.CapabilityDescriptor{ID: "tool:write"}
	args := map[string]any{"path": "a.go"}

	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))
	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))

	err := detector.Check(desc, args)
	var doomErr *DoomLoopError
	require.ErrorAs(t, err, &doomErr)
	require.Equal(t, DoomLoopIdenticalCall, doomErr.Kind)
}

func TestDoomLoopDetectorOscillation(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	a := core.CapabilityDescriptor{ID: "tool:a"}
	b := core.CapabilityDescriptor{ID: "tool:b"}

	sequence := []struct {
		desc core.CapabilityDescriptor
		args map[string]any
	}{
		{a, map[string]any{"step": 1}},
		{b, map[string]any{"step": 2}},
		{a, map[string]any{"step": 1}},
		{b, map[string]any{"step": 2}},
		{a, map[string]any{"step": 1}},
	}
	for _, item := range sequence {
		require.NoError(t, detector.Check(item.desc, item.args))
		require.NoError(t, detector.RecordResult(item.desc, &core.ToolResult{Success: true}))
	}

	err := detector.Check(b, map[string]any{"step": 2})
	var doomErr *DoomLoopError
	require.ErrorAs(t, err, &doomErr)
	require.Equal(t, DoomLoopOscillating, doomErr.Kind)
}

func TestDoomLoopDetectorErrorFixation(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	desc := core.CapabilityDescriptor{ID: "tool:write"}

	for i := 0; i < 4; i++ {
		require.NoError(t, detector.Check(desc, map[string]any{"try": i}))
		require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: false, Error: "same failure"}))
	}

	err := detector.Check(desc, map[string]any{"try": 5})
	var doomErr *DoomLoopError
	require.ErrorAs(t, err, &doomErr)
	require.Equal(t, DoomLoopErrorFixation, doomErr.Kind)
}

func TestDoomLoopDetectorProgressStall(t *testing.T) {
	cfg := DefaultDoomLoopConfig()
	cfg.ProgressStallThreshold = 3
	detector := NewDoomLoopDetector(cfg)
	desc := core.CapabilityDescriptor{ID: "tool:write"}

	require.NoError(t, detector.Check(desc, map[string]any{"step": 1}))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true, Data: map[string]interface{}{"path": "a.go"}}))
	require.NoError(t, detector.Check(desc, map[string]any{"step": 2}))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))
	require.NoError(t, detector.Check(desc, map[string]any{"step": 3}))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))
	require.NoError(t, detector.Check(desc, map[string]any{"step": 4}))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))

	err := detector.Check(desc, map[string]any{"step": 5})
	var doomErr *DoomLoopError
	require.ErrorAs(t, err, &doomErr)
	require.Equal(t, DoomLoopProgressStall, doomErr.Kind)
}

func TestDoomLoopDetectorReset(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	desc := core.CapabilityDescriptor{ID: "tool:write"}
	args := map[string]any{"path": "a.go"}

	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))
	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.RecordResult(desc, &core.ToolResult{Success: true}))
	detector.Reset()
	require.NoError(t, detector.Check(desc, args))
}

func TestCapabilityRegistryAddPostcheck(t *testing.T) {
	registry := NewCapabilityRegistry()
	postcheck := &recordingPostcheck{}
	registry.AddPostcheck(postcheck)

	stub := invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "runtime.echo",
			Name:          "runtime.echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
			TrustClass:    core.TrustClassBuiltinTrusted,
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"message": "ok"}},
	}
	require.NoError(t, registry.RegisterInvocableCapability(stub))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "runtime.echo", nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, postcheck.calls)
	require.Equal(t, stub.desc.ID, postcheck.desc.ID)
}

func TestCapabilityRegistryDoomLoopGuidanceContinue(t *testing.T) {
	registry := NewCapabilityRegistry()
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	registry.AddPrecheck(detector)
	registry.AddPostcheck(detector)

	requests := make(chan RecoveryGuidanceRequest, 1)
	broker := &stubRecoveryBroker{requests: requests, choice: "continue"}
	registry.SetGuidanceBroker(broker)

	stub := invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "runtime.echo",
			Name:          "runtime.echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
			TrustClass:    core.TrustClassBuiltinTrusted,
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"message": "ok"}},
	}
	require.NoError(t, registry.RegisterInvocableCapability(stub))

	for i := 0; i < 2; i++ {
		result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "runtime.echo", map[string]interface{}{"value": 1})
		require.NoError(t, err)
		require.True(t, result.Success)
	}

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "runtime.echo", map[string]interface{}{"value": 1})
	require.NoError(t, err)
	require.True(t, result.Success)
	select {
	case req := <-requests:
		require.Equal(t, "Execution loop detected", req.Title)
	case <-time.After(time.Second):
		t.Fatal("guidance request was not observed")
	}
}
