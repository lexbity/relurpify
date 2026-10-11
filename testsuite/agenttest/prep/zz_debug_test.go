package prep

import (
	"context"
	"testing"
	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

func TestDebugProbeRegistry(t *testing.T) {
	ws := t.TempDir()
	if err := materializeWorkspace(ws, nil); err != nil {
		t.Fatal(err)
	}
	sc := &ScriptedCapability{Name: "prep_probe", Response: map[string]any{"finding": "x"}}
	if err := writeScriptedManifests(ws, []*ScriptedCapability{sc}); err != nil {
		t.Fatal(err)
	}
	runner, err := testsupport.NewAuthorizedFakeRunner(testsupport.PermitAllPolicy())
	if err != nil {
		t.Fatal(err)
	}
	capability, err := envcomposition.BuildCapabilityRuntime(context.Background(), ws, runner, envcomposition.CapabilityRuntimeOptions{Context: context.Background(), AgentID: "prep", SkipASTIndex: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before scripted register: resolved=%v", func() bool { _, ok := capability.Registry.GetCapability("prep_probe"); return ok }())
	if err := registerScripted(context.Background(), capability.Registry, []*ScriptedCapability{sc}); err != nil {
		t.Fatal(err)
	}
	_, ok := capability.Registry.GetCapability("prep_probe")
	t.Logf("after scripted register: resolved=%v", ok)
}
