package promptprovider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

func newEnvelope(t *testing.T, pairs ...any) *contextdata.Envelope {
	t.Helper()
	env := contextdata.NewEnvelope("task-id", "session-id")
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		require.True(t, ok, "odd element in pairs at %d must be a string key", i)
		contextdata.SetTyped[any](env, key, pairs[i+1])
	}
	return env
}

func TestReactToolsProviderListsSortedToolsWithDescriptions(t *testing.T) {
	provider := reactToolsProvider{}
	out := provider.Provide(prompt.RuntimeContext{
		Tools: []ports.Tool{
			&stubTool{name: "write", desc: "Write a file"},
			&stubTool{name: "edit", desc: "Edit a file"},
			&stubTool{name: "bare", desc: ""},
		},
	})
	assert.Equal(t, "- bare\n- edit: Edit a file\n- write: Write a file", out.Content)
}

func TestReactToolsProviderTruncatesLongDescriptions(t *testing.T) {
	long := strings.Repeat("a", 200)

	provider := reactToolsProvider{}
	out := provider.Provide(prompt.RuntimeContext{
		Tools: []ports.Tool{&stubTool{name: "chatty", desc: long}},
	})
	assert.Equal(t, "- chatty: "+strings.Repeat("a", 120)+"…", out.Content)
}

func TestReactToolsProviderEmptyToolsYieldsEmptyChunk(t *testing.T) {
	provider := reactToolsProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
}

func TestReactCapabilityCatalogSkipsToolsAndCapsAtSix(t *testing.T) {
	provider := reactCapabilityCatalogProvider{}
	caps := make([]descriptor.CapabilityDescriptor, 0, 9)
	for i := 0; i < 8; i++ {
		caps = append(caps, descriptor.CapabilityDescriptor{
			ID:          "cap-" + string(rune('a'+i)),
			Name:        "Capability " + string(rune('a'+i)),
			Kind:        agentspec.CapabilityKindPrompt,
			Description: "does things",
		})
	}
	caps = append(caps, descriptor.CapabilityDescriptor{
		ID:   "tool-ignored",
		Kind: agentspec.CapabilityKindTool,
	})

	out := provider.Provide(prompt.RuntimeContext{Capabilities: caps})
	lines := splitLines(out.Content)
	require.Len(t, lines, 6, "non-tool capabilities must be capped at 6")
	assert.Contains(t, out.Content, "Capability a [prompt]: does things")
	assert.NotContains(t, out.Content, "tool-ignored")
}

func TestReactCapabilityCatalogFallsBackToIDAndKind(t *testing.T) {
	provider := reactCapabilityCatalogProvider{}
	out := provider.Provide(prompt.RuntimeContext{
		Capabilities: []descriptor.CapabilityDescriptor{
			{ID: "bare-id", Kind: agentspec.CapabilityKindSession},
		},
	})
	assert.Equal(t, "- bare-id [session]: session", out.Content)
}

func TestReactCapabilityCatalogEmptyInputsYieldEmptyChunk(t *testing.T) {
	provider := reactCapabilityCatalogProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Capabilities: []descriptor.CapabilityDescriptor{{Kind: agentspec.CapabilityKindTool}},
	}))
}

func TestReactPhaseProviderReportsPhaseFromEnvelope(t *testing.T) {
	provider := reactPhaseProvider{}
	out := provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t, "react.phase", "verify")})
	assert.Equal(t, "Execution phase: verify", out.Content)

	out = provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)})
	assert.Equal(t, prompt.ContextChunk{}, out, "missing phase must yield an empty chunk")
}

func TestReactPlanGoalProviderPrefersGoalOverFiles(t *testing.T) {
	provider := reactPlanGoalProvider{}
	env := newEnvelope(t, "architect.plan", pl.Plan{
		Goal:  "Ship the feature",
		Files: []string{"a.go", "b.go"},
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "Ship the feature", out.Content)
}

func TestReactPlanGoalProviderFallsBackToFileScope(t *testing.T) {
	provider := reactPlanGoalProvider{}
	env := newEnvelope(t, "architect.plan", &pl.Plan{Files: []string{"x.go", "y.go"}})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "Files in scope: x.go, y.go", out.Content)
}

func TestReactPlanGoalProviderReadsPlannerPlanWhenArchitectPlanMissing(t *testing.T) {
	provider := reactPlanGoalProvider{}
	env := newEnvelope(t, "planner.plan", pl.Plan{Goal: "planner goal"})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "planner goal", out.Content)
}

func TestReactPlanGoalProviderIgnoresNilPlanAndNilEnvelope(t *testing.T) {
	provider := reactPlanGoalProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: nil}))
}

func TestReactPlanGoalProviderPlanWithoutGoalOrFilesYieldsEmptyChunk(t *testing.T) {
	provider := reactPlanGoalProvider{}
	env := newEnvelope(t, "architect.plan", pl.Plan{})
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))
}

func TestReactCurrentStepProviderRendersPlanStepAsJSON(t *testing.T) {
	provider := reactCurrentStepProvider{}
	task := &execution.Task{Context: map[string]any{
		"current_step": pl.PlanStep{ID: "s1", Description: "do the thing", Expected: "done"},
	}}
	out := provider.Provide(prompt.RuntimeContext{Task: task})
	assert.Contains(t, out.Content, `"ID": "s1"`)
	assert.Contains(t, out.Content, `"Description": "do the thing"`)
	assert.Contains(t, out.Content, `"Expected": "done"`)
}

func TestReactCurrentStepProviderMarshalsRawValues(t *testing.T) {
	provider := reactCurrentStepProvider{}
	task := &execution.Task{Context: map[string]any{"current_step": map[string]any{"k": "v"}}}
	out := provider.Provide(prompt.RuntimeContext{Task: task})
	assert.Contains(t, out.Content, `"k": "v"`)
}

func TestReactCurrentStepProviderMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := reactCurrentStepProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Task: &execution.Task{}}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Task: &execution.Task{Context: map[string]any{"current_step": nil}},
	}))
}

func TestReactPriorStepProviderServesSummaryFromEnvelope(t *testing.T) {
	provider := reactPriorStepProvider{}
	out := provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t, "architect.last_step_summary", "  finished step one  ")})
	assert.Equal(t, "finished step one", out.Content)

	out = provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)})
	assert.Equal(t, prompt.ContextChunk{}, out)
}

func TestReactExternalStateProviderServesTaskContextSlice(t *testing.T) {
	provider := reactExternalStateProvider{}

	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"external_state_slice": "  k=v\n"},
	}})
	assert.Equal(t, "k=v", out.Content)

	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"external_state_slice": 7},
	}})
	assert.Equal(t, "7", out.Content, "non-string values render through fmt.Sprint")

	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"external_state_slice": "   "},
	}})
	assert.Equal(t, prompt.ContextChunk{}, out, "whitespace-only slice must yield an empty chunk")
}

func TestReactExternalStateProviderMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := reactExternalStateProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Task: &execution.Task{}}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Task: &execution.Task{Context: map[string]any{"external_state_slice": nil}},
	}))
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
