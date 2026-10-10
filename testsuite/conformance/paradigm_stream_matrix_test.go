package conformance

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

// TestParadigmStreamedContextMatrix is the Phase-2 gate (D-2/D-6): every
// DSL-reachable paradigm renders the envelope's compiled slice into its model
// messages through the substrate renderer. A sentinel-bodied slice is
// pre-applied to the envelope; the paradigm run must surface the sentinel
// verbatim in at least one recorded model call. The runner table is checked
// against paradigm.Registry dynamically: a paradigm added to the registry
// without a stream integration fails this gate in CI instead of silently
// dropping the slice in a live run.
func TestParadigmStreamedContextMatrix(t *testing.T) {
	runners := map[string]func(t *testing.T) [][]model.Message{
		"react":      streamMatrixReact,
		"reflection": streamMatrixReflection, // inherits via its react primitive
		"htn":        streamMatrixHTN,        // inherits via its react primitive
		"chainer":    streamMatrixChainer,
		"planner":    streamMatrixPlanner,
		"rewoo":      streamMatrixRewoo,
		"pipeline":   streamMatrixPipeline,
		"blackboard": streamMatrixBlackboard,
	}

	for name, run := range runners {
		t.Run(name+"/renders_streamed_slice", func(t *testing.T) {
			messages := run(t)
			found := false
			for _, call := range messages {
				for _, message := range call {
					if strings.Contains(message.Content, streamSentinel(name)) {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("paradigm %q dropped the streamed slice: sentinel %q absent from %d recorded model call(s)", name, streamSentinel(name), len(messages))
			}
		})
	}

	// Drift detector: every registered paradigm must appear in the table. A
	// new paradigm without a stream integration fails here, not in production.
	for _, contract := range paradigm.Registry.All() {
		if _, ok := runners[contract.Paradigm]; !ok {
			t.Errorf("paradigm %q is registered but has no streamed-context matrix runner (paradigm_not_integrated)", contract.Paradigm)
		}
	}
}

// streamSentinel is the per-paradigm sentinel body seeded into the slice.
func streamSentinel(paradigmName string) string {
	return "BKC-SENTINEL-" + paradigmName
}

// seedStreamSlice pre-applies a one-chunk slice whose body carries the
// paradigm's sentinel. The epoch is far above any in-fixture stream apply so
// a noop compile cannot wipe the sentinel slice (D-4 guard).
func seedStreamSlice(t *testing.T, env *contextdata.Envelope, paradigmName string) {
	t.Helper()
	sentinel := streamSentinel(paradigmName)
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID:   "stream-matrix-" + paradigmName,
		Epoch:       99,
		FinalTokens: 16,
		Chunks: []contextdata.StreamedChunk{{
			ChunkID:       contextdata.ChunkID("chunk-sentinel-" + paradigmName),
			ContentHash:   "hash-sentinel-" + paradigmName,
			Body:          sentinel,
			TokenEstimate: 16,
			TrustClass:    "workspace",
		}},
	})
}

func streamMatrixReact(t *testing.T) [][]model.Message {
	t.Helper()
	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), &probeLegacyTool{}); err != nil {
		t.Fatalf("register probe tool: %v", err)
	}
	mdl := &conformanceRecordingModel{text: "work step by step."}
	mdl = mdl.WithToolCalls(model.ToolCall{Name: "conformance_probe", Args: map[string]any{}})
	env := contextdata.NewEnvelope("task-stream-react", "session-stream")
	seedStreamSlice(t, env, "react")
	runFixtureInto(t, "react_until.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func streamMatrixReflection(t *testing.T) [][]model.Message {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text:      "acknowledged",
		chatQueue: []string{`{"verdict":"pass","issues":[]}`},
	}
	env := contextdata.NewEnvelope("task-stream-reflection", "session-stream")
	seedStreamSlice(t, env, "reflection")
	runFixtureInto(t, "reflection_review.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func streamMatrixHTN(t *testing.T) [][]model.Message {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{text: "thought through"}
	env := contextdata.NewEnvelope("task-stream-htn", "session-stream")
	seedStreamSlice(t, env, "htn")
	runFixtureInto(t, "htn_react_fallback.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func streamMatrixChainer(t *testing.T) [][]model.Message {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{text: "chained output"}
	env := contextdata.NewEnvelope("task-stream-chainer", "session-stream")
	env.SetWorkingValueWithClass("state.input_a", "probe-a", contextdata.MemoryClassTask)
	seedStreamSlice(t, env, "chainer")
	runFixtureInto(t, "chainer_links.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func streamMatrixPlanner(t *testing.T) [][]model.Message {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe_a")
	mdl := &conformanceRecordingModel{text: `{"goal":"checks","steps":[{"id":"s1","text":"first check","tool":"euclo:cap.planner_probe_a"}]}`}
	env := contextdata.NewEnvelope("task-stream-planner", "session-stream")
	seedStreamSlice(t, env, "planner")
	runFixtureInto(t, "planner_generated.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func streamMatrixRewoo(t *testing.T) [][]model.Message {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.conformance_layer_check")
	mdl := &conformanceRecordingModel{text: "synthesized"}
	env := contextdata.NewEnvelope("task-stream-rewoo", "session-stream")
	seedStreamSlice(t, env, "rewoo")
	runFixtureInto(t, "rewoo_authored.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

// streamMatrixPipeline drives the pipeline Runner with a stub stage whose
// prompt assembly reads the envelope — the paradigm's single integration
// point — under the recording model.
func streamMatrixPipeline(t *testing.T) [][]model.Message {
	t.Helper()
	mdl := &conformanceRecordingModel{text: "stage output"}
	runner := &pipeline.Runner{Options: pipeline.RunnerOptions{Model: mdl}}
	env := contextdata.NewEnvelope("task-stream-pipeline", "session-stream")
	seedStreamSlice(t, env, "pipeline")
	task := &execution.Task{ID: "stream-matrix", Instruction: "probe"}
	_, err := runner.Execute(context.Background(), task, env, []pipeline.Stage{&streamMatrixStage{}})
	if err != nil {
		t.Fatalf("pipeline runner: %v", err)
	}
	return mdl.allMessages()
}

// streamMatrixBlackboard drives an authored specialist source (no capability
// pin) so the run goes through the agent's real model prompt assembly.
func streamMatrixBlackboard(t *testing.T) [][]model.Message {
	t.Helper()
	mdl := &conformanceRecordingModel{text: "specialist output"}
	agent := blackboard.New(paradigmDeps(mdl, registry.NewRegistry()), blackboard.WithAuthoredSources([]blackboard.AuthoredSource{
		{Name: "specialist", Read: []string{"state.seed"}, Write: "state.out"},
	}))
	env := contextdata.NewEnvelope("task-stream-blackboard", "session-stream")
	env.SetWorkingValueWithClass("state.seed", "seed-value", contextdata.MemoryClassTask)
	seedStreamSlice(t, env, "blackboard")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "stream-matrix", Instruction: "probe"}, env); err != nil {
		t.Fatalf("blackboard execute: %v", err)
	}
	return mdl.allMessages()
}

// streamMatrixStage is the minimal typed stage: its prompt is empty so the
// only content in the model call is what the substrate renderer added.
type streamMatrixStage struct{}

func (s *streamMatrixStage) Name() string { return "sentinel_stage" }

func (s *streamMatrixStage) Contract() pipeline.ContractDescriptor {
	return pipeline.ContractDescriptor{
		Name: "sentinel_stage",
		Metadata: pipeline.ContractMetadata{
			InputKey:      "state.seed",
			OutputKey:     "state.out",
			SchemaVersion: "v1",
		},
	}
}

func (s *streamMatrixStage) BuildPrompt(env *contextdata.Envelope) (string, error) {
	return "", nil
}

func (s *streamMatrixStage) Decode(resp *model.LLMResponse) (any, error) {
	return resp.Text, nil
}

func (s *streamMatrixStage) Validate(output any) error { return nil }

func (s *streamMatrixStage) Apply(env *contextdata.Envelope, output any) error { return nil }
