package prep

import (
	"context"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

const canonicalAnswer = "prep canonical completion"

// canonicalScripted shadows the `do relurpic:*` route-step capabilities the
// canonical recipes lower to (executor ID form `euclo:cap.*`), so the whole
// table runs hermetically against scripted capabilities instead of real
// command-running tools.
func canonicalScripted() []*ScriptedCapability {
	return []*ScriptedCapability{
		{Name: "euclo:cap.code_review", Response: map[string]any{"findings": "none"}},
		{Name: "euclo:cap.ast_query", Response: map[string]any{"symbols": []string{"main"}}},
		{Name: "euclo:cap.test_run", Response: map[string]any{"passed": true}},
	}
}

// TestDryRunCanonicalRecipeTable drives the canonical 7 recipes (extracted
// from the embedded template) through the real loader, real euclo agent, real
// RootGraph, and scripted capabilities with a scripted model (FR-8/FR-9).
func TestDryRunCanonicalRecipeTable(t *testing.T) {
	cases := []struct {
		name        string
		instruction string
		wantRoute   string
		// wantModel is false only for extract_func, whose shipped route shape
		// executes pure capability branches when state.summary is absent.
		wantModel bool
	}{
		{name: "code_review", instruction: "review main.go", wantRoute: "euclo.thoughtrecipe.code_review", wantModel: true},
		{name: "debug_tdd_repair", instruction: "fix failure.log", wantRoute: "euclo.thoughtrecipe.debug_tdd_repair", wantModel: true},
		{name: "dep_upgrade", instruction: "fix upgrade dep.md", wantRoute: "euclo.thoughtrecipe.dep_upgrade", wantModel: true},
		{name: "extract_func", instruction: "refactor extract.md", wantRoute: "euclo.thoughtrecipe.extract_func"},
		{name: "test_synthesis", instruction: "create main_test.go", wantRoute: "euclo.thoughtrecipe.test_synthesis", wantModel: true},
		{name: "investigation", instruction: "investigate notes.md", wantRoute: "euclo.thoughtrecipe.investigation", wantModel: true},
		// No family-classifiable token: no recipe scores family affinity, so
		// route resolution falls back to the built-in default execution recipe.
		{name: "default_fallback", instruction: "explain summary.md", wantRoute: "euclo.thoughtrecipe.default", wantModel: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Run(context.Background(), DryRunConfig{
				Name:           tc.name,
				WorkspaceFiles: map[string]string{canonicalMarker: "1"},
				Instruction:    tc.instruction,
				Turns:          []testhelper.ModelTurn{{Text: canonicalAnswer}},
				Scripted:          canonicalScripted(),
			})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !report.Success {
				t.Fatalf("run errors: %v", report.Errors)
			}
			if report.Dispatched != tc.wantRoute {
				t.Errorf("dispatched %q, want %q (decidedBy=%q fallback=%v)", report.Dispatched, tc.wantRoute, report.DecidedBy, report.FallbackTaken)
			}
			if len(report.ParadigmRuns) == 0 {
				t.Errorf("no paradigm runs recorded")
			}
			if tc.wantModel && len(report.ModelMessages) == 0 {
				t.Errorf("no model messages recorded")
			}
			if report.Dispatched != "euclo.thoughtrecipe.default" && report.FallbackTaken {
				t.Errorf("unexpected fallback for %q", report.Dispatched)
			}
		})
	}
}

// TestDryRunScriptedCapabilityScoping proves the DSL `may invoke` scoping path:
// the react loop can invoke the declared scripted, and the scripted records the
// invocation with its arguments (FR-8 capability invocation, not mocked away).
func TestDryRunScriptedCapabilityScoping(t *testing.T) {
	probe := &ScriptedCapability{Name: "prep_probe", Response: map[string]any{"finding": "platypus-quartz 42"}}
	report, err := Run(context.Background(), DryRunConfig{
		Name:           "scripted-scoping",
		WorkspaceFiles: map[string]string{"relurpify_cfg/euclo/capture_ground.erpe": readFixture(t, "capture_ground.erpe")},
		Instruction:    "investigate platypus-quartz.md",
		Turns: []testhelper.ModelTurn{
			// The first turn is consumed by the route-disambiguation model call
			// before any paradigm runs; deterministic lattice scoring already
			// selected the route, so its text is irrelevant.
			{Text: `{}`},
			// react runs with native tool calling disabled: decisions arrive as
			// JSON text, exactly the shape the production prompt demands.
			{Text: `{"thought":"call the probe","action":"tool","tool":"prep_probe","arguments":{"target":"platypus-quartz.md"}}`},
			{Text: `{"thought":"collected","action":"complete","summary":"platypus-quartz 42 observed"}`},
			{Text: `{"thought":"summarized","action":"complete","summary":"platypus-quartz 42 confirmed"}`},
		},
		Scripted: []*ScriptedCapability{probe},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Success {
		t.Fatalf("run errors: %v", report.Errors)
	}
	if report.Dispatched != "prep_capture_ground" {
		t.Fatalf("dispatched %q, want prep_capture_ground", report.Dispatched)
	}
	calls := probe.Calls()
	if len(calls) == 0 {
		t.Fatalf("prep_probe was never invoked; capability calls: %+v", report.CapabilityCalls)
	}
	if calls[0].Args["target"] != "platypus-quartz.md" {
		t.Errorf("scripted invoked with args %+v", calls[0].Args)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile("testdata/relurpify_cfg/euclo/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(content)
}
