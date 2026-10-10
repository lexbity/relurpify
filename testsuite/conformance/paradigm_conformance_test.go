package conformance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// paradigmFixtureDir holds the .erpe fixtures each conformance case runs. It
// lives outside testdata/ so the phase's matrix and fixtures are committed.
const paradigmFixtureDir = "paradigm_fixtures"

// matrixPath is the generated, committed conformance matrix.
const matrixPath = "paradigm_matrix.yaml"

// TestParadigmContractConformance is the registry-driven conformance harness
// (AC-4): every ConformanceCase declared by a paradigm contract must have a
// runner here, and running it must prove the declared runtime effect. A
// declared directive without a runner fails immediately (the drift detector of
// contract completeness, parallel to each paradigm's TestContractCompleteness).
func TestParadigmContractConformance(t *testing.T) {
	for _, contract := range paradigm.Registry.All() {
		for _, cas := range contract.Conformance {
			cas := cas
			t.Run(contract.Paradigm+"/"+cas.ID, func(t *testing.T) {
				runner := conformanceRunners[cas.ID]
				if runner == nil {
					t.Fatalf("conformance case %q has no runner (contract declared, runtime drift)", cas.ID)
				}
				runner(t)
			})
		}
	}
}

// conformanceRunner executes one declared conformance case.
type conformanceRunner func(t *testing.T)

// conformanceRunners maps every declared ConformanceCase ID to its runner.
var conformanceRunners = map[string]conformanceRunner{ //nolint:gochecknoglobals // immutable case table
	"react/until_bounds_iterations":             runReactUntilBounds,
	"chainer/link_builds_chain":                 runChainerLinks,
	"chainer/link_from_registry_prompt":         runChainerFromRegistryPrompt,
	"pipeline/stages_execute_in_order":          runPipelineStages,
	"rewoo/authored_plan_skips_planner":         runRewooAuthoredPlanSkipsPlanner,
	"rewoo/authored_steps_execute_in_order":     runRewooAuthoredStepsInOrder,
	"rewoo/synthesize_guidance":                 runRewooSynthesizeGuidance,
	"planner/authored_plan_zero_planning_calls": runPlannerAuthoredPlanZeroPlanningCalls,
	"planner/authored_steps_execute_in_order":   runPlannerAuthoredStepsInOrder,
	"planner/generated_plan_bounded":            runPlannerGeneratedPlanBounded,
	"planner/verify_verdict_fields":             runPlannerVerifyVerdictFields,
	"planner/summarize_replaces_result":         runPlannerSummarizeReplacesResult,
	"htn/authored_decomposition_order":          runHTNAuthoredDecompositionOrder,
	"htn/task_capability_pin":                   runHTNTaskCapabilityPin,
	"htn/authored_resume":                       runHTNAuthoredResume,
}

// runReactUntilBounds proves the `until` directive caps the react loop budget:
// a non-completing model loop driven through the real RunNode path must stop at
// the declared iteration cap, and the envelope must observe exactly that count.
func runReactUntilBounds(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), &probeLegacyTool{}); err != nil {
		t.Fatalf("register probe tool: %v", err)
	}
	model := testhelper.NewScriptedModel("work step by step.").
		WithToolCalls(model.ToolCall{Name: "conformance_probe", Args: map[string]any{}})

	env := runFixture(t, "react_until.erpe", paradigmDeps(model, reg))
	iter, ok := contextdata.GetTyped[int](env, "react.iteration")
	if !ok {
		t.Fatal("expected react.iteration on the envelope after a react run")
	}
	if iter != 2 {
		t.Fatalf("react.iteration = %d, want 2 (the declared `until 2` cap)", iter)
	}
	if done, _ := contextdata.GetTyped[bool](env, "react.done"); !done {
		t.Fatal("expected the capped react loop to terminate")
	}
}

// runChainerLinks proves `link:` blocks build and run a real chain: every
// block becomes one executed step, each capture target holds the link output,
// and from-keys were available to each stage.
func runChainerLinks(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	model := testhelper.NewScriptedModel("chained output")
	env := contextdata.NewEnvelope("task-chainer", "session-chainer")
	env.SetWorkingValueWithClass("state.input_a", "probe-a", contextdata.MemoryClassTask)

	runFixtureInto(t, "chainer_links.erpe", paradigmDeps(model, reg), env)

	executed, ok := contextdata.GetTyped[int](env, "chainer.links_executed")
	if !ok {
		t.Fatal("expected chainer.links_executed on the envelope")
	}
	if executed != 2 {
		t.Fatalf("chainer.links_executed = %d, want 2 links", executed)
	}
	for _, key := range []string{"state.out_a", "state.out_b"} {
		value, ok := contextdata.GetTyped[any](env, key)
		if !ok {
			t.Fatalf("expected chainer capture target %q to hold the link output", key)
		}
		if got := fmt.Sprint(value); got != "chained output" {
			t.Fatalf("capture target %q = %q, want the model output", key, got)
		}
	}
}

// runChainerFromRegistryPrompt proves AC-5: when a link resolves its prompt via
// a registry PromptID, the link's `from` keys are injected into the resolved
// prompt's runtime context state (chainer/runner.go from-keys fix).
func runChainerFromRegistryPrompt(t *testing.T) {
	t.Helper()
	recorder := &recordingPromptRegistry{prompt: "resolved prompt"}
	env := contextdata.NewEnvelope("task-chain-reg", "session-chain-reg")
	env.SetWorkingValueWithClass("state.key", "from-value", contextdata.MemoryClassTask)

	link := chainer.Link{Name: "l1", PromptID: "test.prompt", InputKeys: []string{"state.key"}, OutputKey: "state.out"}
	chain := &chainer.Chain{Links: []chainer.Link{link}}
	task := &execution.Task{ID: "chain-task", Instruction: "chain it"}
	err := chainer.RunChain(context.Background(), testhelper.NewScriptedModel("out"), task, chain, env, recorder)
	if err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if got := recorder.state["state.key"]; got != "from-value" {
		t.Fatalf("registry prompt context state = %#v, want state.key=%q", recorder.state, "from-value")
	}
}

// runPipelineStages proves the pipeline shape case: both stage steps execute
// and record their capability results on the envelope.
func runPipelineStages(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	for _, id := range []string{"euclo:cap.conformance_first", "euclo:cap.conformance_second"} {
		if err := reg.RegisterInvocableCapability(context.Background(), &resultCapability{id: id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	env := runFixture(t, "pipeline_stages.erpe", paradigmDeps(nil, reg))
	results := stepResultPayloads(env)
	if len(results) < 2 {
		t.Fatalf("expected result entries for both pipeline stage steps, got %d", len(results))
	}
	joined := strings.Join(results, "\n")
	for _, want := range []string{"euclo:cap.conformance_first", "euclo:cap.conformance_second"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("pipeline stage results missing %q: %s", want, joined)
		}
	}
}

// runRewooAuthoredPlanSkipsPlanner proves the restored `plan` directive:
// authored plan+steps execute with zero planning-model calls and record
// plan_origin=authored.
func runRewooAuthoredPlanSkipsPlanner(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.conformance_layer_check", "euclo:cap.conformance_fix")
	mdl := &conformanceRecordingModel{text: "synthesized"}
	env := runFixture(t, "rewoo_authored.erpe", paradigmDeps(mdl, reg))

	if got := mdl.callCount(); got != 1 {
		t.Fatalf("model calls = %d, want 1 (synthesize only; planner skipped)", got)
	}
	for _, messages := range mdl.allMessages() {
		for _, message := range messages {
			if strings.Contains(message.Content, "ReWOO planner") {
				t.Fatalf("the planner prompt ran in authored mode: %q", message.Content)
			}
		}
	}
	if origin, _ := contextdata.GetTyped[string](env, "rewoo.plan_origin"); origin != "authored" {
		t.Fatalf("rewoo.plan_origin = %q, want authored", origin)
	}
	if len(order) != 2 {
		t.Fatalf("governed steps executed = %d, want 2", len(order))
	}
}

// runRewooAuthoredStepsInOrder proves the restored `step` directive: authored
// steps execute in declaration order through the governed executor.
func runRewooAuthoredStepsInOrder(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.conformance_layer_check", "euclo:cap.conformance_fix")
	mdl := &conformanceRecordingModel{text: "synthesized"}
	runFixture(t, "rewoo_authored.erpe", paradigmDeps(mdl, reg))

	want := []string{"euclo:cap.conformance_layer_check", "euclo:cap.conformance_fix"}
	if len(order) != len(want) {
		t.Fatalf("executed steps = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("execution order = %v, want %v", order, want)
		}
	}
}

// runRewooSynthesizeGuidance proves the restored `synthesize` directive:
// authored guidance reaches the synthesizer prompt as an authoritative system
// message.
func runRewooSynthesizeGuidance(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.conformance_layer_check")
	mdl := &conformanceRecordingModel{text: "synthesized"}
	runFixture(t, "rewoo_guidance.erpe", paradigmDeps(mdl, reg))

	found := false
	for _, messages := range mdl.allMessages() {
		for _, message := range messages {
			if strings.Contains(message.Content, "Prefer a terse bullet list.") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("synthesize guidance did not reach the synthesizer prompt")
	}
}

// runPlannerAuthoredPlanZeroPlanningCalls proves the restored `plan` directive:
// an authored plan+steps executes with zero planning-model calls and records
// plan_origin=authored.
func runPlannerAuthoredPlanZeroPlanningCalls(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe_a", "euclo:cap.planner_probe_b")
	mdl := &conformanceRecordingModel{text: "unused"}
	env := runFixture(t, "planner_authored.erpe", paradigmDeps(mdl, reg))

	if got := mdl.callCount(); got != 0 {
		t.Fatalf("model calls = %d, want 0 (authored plan; no verify/summarize)", got)
	}
	if origin, _ := contextdata.GetTyped[string](env, "planner.plan_origin"); origin != "authored" {
		t.Fatalf("planner.plan_origin = %q, want authored", origin)
	}
	if len(order) != 2 {
		t.Fatalf("executed steps = %d, want 2", len(order))
	}
}

// runPlannerAuthoredStepsInOrder proves the restored `step` directive: authored
// steps execute in declaration order.
func runPlannerAuthoredStepsInOrder(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe_a", "euclo:cap.planner_probe_b")
	mdl := &conformanceRecordingModel{text: "unused"}
	runFixture(t, "planner_authored.erpe", paradigmDeps(mdl, reg))

	want := []string{"euclo:cap.planner_probe_a", "euclo:cap.planner_probe_b"}
	if len(order) != len(want) {
		t.Fatalf("executed steps = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("execution order = %v, want %v", order, want)
		}
	}
}

// runPlannerGeneratedPlanBounded proves a plan-only directive generates a
// bounded plan with one model call and records plan_origin=generated.
func runPlannerGeneratedPlanBounded(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe_a", "euclo:cap.planner_probe_b")
	mdl := &conformanceRecordingModel{text: `{"goal":"checks","steps":[{"id":"s1","text":"first check","tool":"euclo:cap.planner_probe_a"},{"id":"s2","text":"second check","tool":"euclo:cap.planner_probe_b"}]}`}
	env := runFixture(t, "planner_generated.erpe", paradigmDeps(mdl, reg))

	if got := mdl.callCount(); got != 1 {
		t.Fatalf("model calls = %d, want 1 (generated plan only)", got)
	}
	if origin, _ := contextdata.GetTyped[string](env, "planner.plan_origin"); origin != "generated" {
		t.Fatalf("planner.plan_origin = %q, want generated", origin)
	}
	if len(order) != 2 {
		t.Fatalf("executed steps = %v, want 2", order)
	}
}

// runPlannerVerifyVerdictFields proves the restored `verify` directive writes
// the verdict fields; a fail verdict is data, not an operational failure.
func runPlannerVerifyVerdictFields(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe")
	mdl := &conformanceRecordingModel{text: `{"verdict":"fail","issues":["missing evidence"]}`}
	env := runFixture(t, "planner_verify.erpe", paradigmDeps(mdl, reg))

	if verdict, _ := contextdata.GetTyped[string](env, "planner.verification"); verdict != "fail" {
		t.Fatalf("planner.verification = %q, want fail", verdict)
	}
	issues, ok := contextdata.GetTyped[any](env, "planner.verification_issues")
	if !ok {
		t.Fatal("expected planner.verification_issues on the envelope")
	}
	if got := fmt.Sprint(issues); got != "[missing evidence]" {
		t.Fatalf("planner.verification_issues = %v", issues)
	}
	if captured, _ := contextdata.GetTyped[string](env, "state.planner_verification"); captured != "fail" {
		t.Fatalf("captured state.planner_verification = %q, want fail (capture binds the verification result field)", captured)
	}
}

// runPlannerSummarizeReplacesResult proves the restored `summarize` directive
// replaces result and preserves the pre-summary aggregate as result_raw.
func runPlannerSummarizeReplacesResult(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.planner_probe")
	mdl := &conformanceRecordingModel{text: "final report text"}
	env := runFixture(t, "planner_summarize.erpe", paradigmDeps(mdl, reg))

	if result, _ := contextdata.GetTyped[string](env, "planner.result"); result != "final report text" {
		t.Fatalf("planner.result = %q, want the synthesized summary", result)
	}
	if _, ok := contextdata.GetTyped[any](env, "planner.result_raw"); !ok {
		t.Fatal("expected planner.result_raw (pre-summary aggregate)")
	}
}

// runHTNAuthoredDecompositionOrder proves the restored `method` directive:
// authored tasks execute in declaration order with zero decomposition model
// calls.
func runHTNAuthoredDecompositionOrder(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order,
		"euclo:cap.htn_probe_a", "euclo:cap.htn_probe_b", "euclo:cap.htn_probe_c")
	mdl := &conformanceRecordingModel{text: "unused"}
	env := runFixture(t, "htn_authored.erpe", paradigmDeps(mdl, reg))

	if got := mdl.callCount(); got != 0 {
		t.Fatalf("model calls = %d, want 0 (authored decomposition)", got)
	}
	want := []string{"euclo:cap.htn_probe_a", "euclo:cap.htn_probe_b", "euclo:cap.htn_probe_c"}
	if len(order) != len(want) {
		t.Fatalf("decomposed tasks executed = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("task execution order = %v, want %v", order, want)
		}
	}
	if method, _ := contextdata.GetTyped[string](env, "htn.method"); method != "full_analysis" {
		t.Fatalf("htn.method = %q, want full_analysis", method)
	}
	if total, _ := contextdata.GetTyped[int](env, "htn.tasks_total"); total != 3 {
		t.Fatalf("htn.tasks_total = %d, want 3", total)
	}
}

// runHTNTaskCapabilityPin proves the restored `task` directive: a task's `do`
// capability is the dispatched target.
func runHTNTaskCapabilityPin(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.htn_probe_pin")
	mdl := &conformanceRecordingModel{text: "unused"}
	env := runFixture(t, "htn_pin.erpe", paradigmDeps(mdl, reg))

	if len(order) != 1 || order[0] != "euclo:cap.htn_probe_pin" {
		t.Fatalf("capability invocations = %v, want [euclo:cap.htn_probe_pin]", order)
	}
	if completed, _ := contextdata.GetTyped[int](env, "htn.tasks_completed"); completed != 1 {
		t.Fatalf("htn.tasks_completed = %d, want 1", completed)
	}
}

// runHTNAuthoredResume proves completed tasks are not re-executed when
// plan.completed_steps is pre-seeded.
func runHTNAuthoredResume(t *testing.T) {
	t.Helper()
	order := []string{}
	reg := newSequenceRegistry(t, &order, "euclo:cap.htn_probe_a", "euclo:cap.htn_probe_b")
	mdl := &conformanceRecordingModel{text: "unused"}
	env := contextdata.NewEnvelope("task-htn-resume", "session-conformance")
	env.SetWorkingValueWithClass("plan.completed_steps", []string{"t1"}, contextdata.MemoryClassTask)
	runFixtureInto(t, "htn_resume.erpe", paradigmDeps(mdl, reg), env)

	if len(order) != 1 || order[0] != "euclo:cap.htn_probe_b" {
		t.Fatalf("executed tasks = %v, want [euclo:cap.htn_probe_b] (t1 resumed)", order)
	}
}

// newSequenceRegistry registers sequence-recording invocable
// capabilities for the paradigm fixtures.
func newSequenceRegistry(t *testing.T, order *[]string, ids ...string) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, id := range ids {
		if err := reg.RegisterInvocableCapability(context.Background(), &sequenceCapability{id: id, order: order}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	return reg
}

// sequenceCapability records the order in which capabilities are invoked and
// reports its own id as output.
type sequenceCapability struct {
	id    string
	order *[]string
}

func (h *sequenceCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *sequenceCapability) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	*h.order = append(*h.order, h.id)
	return &ports.ToolResult{Success: true, Data: map[string]any{"capability_id": h.id}}, nil
}

// conformanceRecordingModel records every Chat call's messages and returns a
// fixed text, so conformance cases can distinguish planner from synthesizer
// calls and inspect prompt content.
type conformanceRecordingModel struct {
	mu    sync.Mutex
	text  string
	calls [][]model.Message
}

func (m *conformanceRecordingModel) Generate(ctx context.Context, _ string, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, nil, options)
}

func (m *conformanceRecordingModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *conformanceRecordingModel) Chat(_ context.Context, messages []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.calls = append(m.calls, append([]model.Message(nil), messages...))
	m.mu.Unlock()
	return &model.LLMResponse{Text: m.text}, nil
}

func (m *conformanceRecordingModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

func (m *conformanceRecordingModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *conformanceRecordingModel) allMessages() [][]model.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]model.Message, len(m.calls))
	copy(out, m.calls)
	return out
}

// --- fixture plumbing -------------------------------------------------------

func paradigmDeps(model model.LanguageModel, reg *registry.CapabilityRegistry) *paradigm.Deps {
	return &paradigm.Deps{
		Model:             model,
		Registry:          reg,
		Config:            &execution.Config{Name: "paradigm-conformance", Model: "scripted"},
		StreamTrigger:     contextstream.NewTrigger(noopCompiler{}),
		PermissionChecker: permissiveCapabilityChecker{},
	}
}

// permissiveCapabilityChecker allows every capability so governed paradigms
// (rewoo) run under conformance without a policy composition.
type permissiveCapabilityChecker struct{}

func (permissiveCapabilityChecker) CheckCapability(context.Context, string, string) error { return nil }

// noopCompiler is an offline compiler invoker so react's streaming trigger node
// resolves during conformance runs without a real compiler.
type noopCompiler struct{}

func (noopCompiler) Compile(context.Context, contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{}, nil
}

// runFixture parses+lowers+validates the fixture .erpe and executes its graph
// against a fresh envelope, returning the envelope after execution.
func runFixture(t *testing.T, name string, deps *paradigm.Deps) *contextdata.Envelope {
	t.Helper()
	env := contextdata.NewEnvelope("task-"+strings.TrimSuffix(name, ".erpe"), "session-conformance")
	runFixtureInto(t, name, deps, env)
	return env
}

// runFixtureInto executes a fixture through the real recipe path (parser,
// semantic/contract validation, lowering, graph build, Execute) on the given
// envelope.
func runFixtureInto(t *testing.T, name string, deps *paradigm.Deps, env *contextdata.Envelope) {
	t.Helper()
	path := filepath.Join(paradigmFixtureDir, name)
	src, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	doc, err := thoughtrecipe.ParseSource(path, string(src))
	if err != nil {
		t.Fatalf("ParseSource(%s): %v", name, err)
	}
	if errs := thoughtrecipe.ValidateAgainstContracts(doc, paradigm.Registry); len(errs) != 0 {
		t.Fatalf("fixture %s violates the paradigm contracts: %v", name, errs)
	}
	plan, err := thoughtrecipe.LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument(%s): %v", name, err)
	}
	if err := thoughtrecipe.ValidatePlanContracts(plan, paradigm.Registry); err != nil {
		t.Fatalf("fixture %s fails plan-level contract validation: %v", name, err)
	}
	graph, err := thoughtrecipe.BuildThoughtRecipeGraph(plan, deps, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph(%s): %v", name, err)
	}
	ctx := context.Background()
	if _, err := graph.Execute(ctx, env); err != nil {
		t.Fatalf("graph.Execute(%s): %v", name, err)
	}
}

func stepResultPayloads(env *contextdata.Envelope) []string {
	var out []string
	for key := range env.Snapshot() {
		if !strings.HasSuffix(key, ".result") {
			continue
		}
		if value, ok := contextdata.GetTyped[any](env, key); ok {
			out = append(out, fmt.Sprint(value))
		}
	}
	sort.Strings(out)
	return out
}

// probeLegacyTool is a no-tag tool (allowed in every react phase) whose result
// carries no read/edit/verify summary markers, so a react loop driven by it
// continues until its iteration budget and never completes early.
type probeLegacyTool struct{}

func (t *probeLegacyTool) Name() string                         { return "conformance_probe" }
func (t *probeLegacyTool) Description() string                  { return "offline conformance probe" }
func (t *probeLegacyTool) Category() string                     { return "test" }
func (t *probeLegacyTool) Parameters() []ports.ToolParameter    { return nil }
func (t *probeLegacyTool) IsAvailable(ctx context.Context) bool { return true }
func (t *probeLegacyTool) Permissions() ports.ToolPermissions   { return ports.ToolPermissions{} }
func (t *probeLegacyTool) Tags() []string                       { return nil }
func (t *probeLegacyTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}

// resultCapability is an invocable capability that records its own id.
type resultCapability struct {
	id string
}

func (h *resultCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}
func (h *resultCapability) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"capability_id": h.id}}, nil
}

// recordingPromptRegistry resolves every PromptID to a fixed prompt and
// records the runtime-context State it was asked to resolve with.
type recordingPromptRegistry struct {
	mu     sync.Mutex
	prompt string
	state  map[string]any
}

func (r *recordingPromptRegistry) Resolve(_ string, ctx any) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := ctx.(map[string]any); ok {
		if state, ok := m["State"].(map[string]any); ok {
			r.state = state
		}
	}
	return r.prompt, nil
}

// --- matrix (generated, committed) ------------------------------------------

// TestParadigmMatrixUpToDate regenerates the conformance matrix from the
// contract registry and compares it byte-for-byte with the committed file.
// Set UPDATE_MATRIX=1 to rewrite the file after a deliberate contract change.
func TestParadigmMatrixUpToDate(t *testing.T) {
	got := generateMatrixYAML(t)
	if update := os.Getenv("UPDATE_MATRIX"); update != "" {
		if err := os.WriteFile(matrixPath, got, 0o644); err != nil {
			t.Fatalf("write matrix: %v", err)
		}
		return
	}
	want, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read matrix %s: %v (set UPDATE_MATRIX=1 to generate)", matrixPath, err)
	}
	if string(got) != string(want) {
		t.Fatalf("paradigm matrix drifted from the contract registry:\ngot:\n%s\nwant:\n%s\n(set UPDATE_MATRIX=1 to regenerate)", got, want)
	}
}

// matrixRow is one declared (paradigm, directive) pair and its cases.
type matrixRow struct {
	Paradigm  string   `yaml:"paradigm"`
	Directive string   `yaml:"directive"`
	Status    string   `yaml:"status"`
	Cases     []string `yaml:"cases"`
}

// paradigmMatrix is the committed conformance matrix.
type paradigmMatrix struct {
	Declared []matrixRow `yaml:"declared"`
	Restored []matrixRow `yaml:"restored"`
	Deleted  []string    `yaml:"deleted"`
	Keywords []string    `yaml:"documented_directive_keywords"`
}

// caseStatus annotates whether a case's effect predated this phase (honored)
// or was implemented inside it (implemented).
var caseStatus = map[string]string{ //nolint:gochecknoglobals // immutable matrix annotation
	"react/until_bounds_iterations":             "implemented",
	"chainer/link_builds_chain":                 "implemented",
	"chainer/link_from_registry_prompt":         "implemented",
	"pipeline/stages_execute_in_order":          "honored",
	"rewoo/authored_plan_skips_planner":         "implemented",
	"rewoo/authored_steps_execute_in_order":     "implemented",
	"rewoo/synthesize_guidance":                 "implemented",
	"planner/authored_plan_zero_planning_calls": "implemented",
	"planner/authored_steps_execute_in_order":   "implemented",
	"planner/generated_plan_bounded":            "implemented",
	"planner/verify_verdict_fields":             "implemented",
	"planner/summarize_replaces_result":         "implemented",
	"htn/authored_decomposition_order":          "implemented",
	"htn/task_capability_pin":                   "implemented",
	"htn/authored_resume":                       "implemented",
}

// restoredDirectives is the restoration audit (Wave 3): every (paradigm,
// directive) pair whose vocabulary was retired by the Wave-2 implement-or-delete
// ruling and is restored as honored runner semantics in this wave. Each entry
// must be declared by its contract and backed by a conformance case; the
// invariant test enforces that and that no entry lingers in the deleted list.
var restoredDirectives = []string{ //nolint:gochecknoglobals // immutable audit result
	"rewoo/plan", "rewoo/step", "rewoo/synthesize",
	"planner/plan", "planner/step", "planner/verify", "planner/summarize",
	"htn/method", "htn/task",
}

// deletedDirectives is the implement-or-delete audit outcome (FR-6): every
// directive vocabulary a paradigm does not honor is retired from every
// contract, so carrying it in a recipe is a load error, never a silent no-op.
// The names here are the grammar keyword set from the parser's directive
// vocabulary minus the declared directives (until, link); entries are written
// paradigm/directive for pairs that were once declared, and bare keywords for
// vocabulary never owned by a paradigm.
var deletedDirectives = []string{ //nolint:gochecknoglobals // immutable audit result
	"reflection/review", "reflection/revise",
	"blackboard/source",
	"detect", "clarify", "retry", "decompose", "solve",
}

// documentedDirectiveKeywords is the closed set of directive keywords the
// grammar reference documents. The invariant test asserts each keyword is
// either declared by a contract or listed as deleted — no keyword may linger
// undeclared and unlisted.
var documentedDirectiveKeywords = []string{ //nolint:gochecknoglobals // immutable grammar reference vocabulary
	"until", "link", "plan", "step", "method", "task", "source",
	"detect", "clarify", "revise", "retry", "verify", "summarize",
	"review", "decompose", "solve",
}

func generateMatrixYAML(t *testing.T) []byte {
	t.Helper()
	var matrix paradigmMatrix
	for _, contract := range paradigm.Registry.All() {
		for _, spec := range contract.Directives {
			var cases []string
			for _, cas := range contract.Conformance {
				if cas.Directive == spec.Name {
					cases = append(cases, cas.ID)
				}
			}
			matrix.Declared = append(matrix.Declared, matrixRow{
				Paradigm:  contract.Paradigm,
				Directive: spec.Name,
				Status:    statusForCase(cases),
				Cases:     cases,
			})
		}
		// shape-level cases (Directive == "") surface as their own rows.
		for _, cas := range contract.Conformance {
			if cas.Directive != "" {
				continue
			}
			matrix.Declared = append(matrix.Declared, matrixRow{
				Paradigm: contract.Paradigm,
				Status:   statusForCase([]string{cas.ID}),
				Cases:    []string{cas.ID},
			})
		}
	}
	matrix.Deleted = append([]string(nil), deletedDirectives...)
	matrix.Keywords = append([]string(nil), documentedDirectiveKeywords...)
	for _, entry := range restoredDirectives {
		paradigmName, directive, ok := strings.Cut(entry, "/")
		if !ok {
			continue
		}
		contract, ok := paradigm.Registry.Lookup(paradigmName)
		if !ok {
			continue
		}
		var cases []string
		for _, cas := range contract.Conformance {
			if cas.Directive == directive {
				cases = append(cases, cas.ID)
			}
		}
		matrix.Restored = append(matrix.Restored, matrixRow{
			Paradigm:  paradigmName,
			Directive: directive,
			Status:    statusForCase(cases),
			Cases:     cases,
		})
	}
	sort.Slice(matrix.Declared, func(i, j int) bool {
		if matrix.Declared[i].Paradigm != matrix.Declared[j].Paradigm {
			return matrix.Declared[i].Paradigm < matrix.Declared[j].Paradigm
		}
		return matrix.Declared[i].Directive < matrix.Declared[j].Directive
	})
	sort.Slice(matrix.Restored, func(i, j int) bool {
		if matrix.Restored[i].Paradigm != matrix.Restored[j].Paradigm {
			return matrix.Restored[i].Paradigm < matrix.Restored[j].Paradigm
		}
		return matrix.Restored[i].Directive < matrix.Restored[j].Directive
	})
	sort.Strings(matrix.Deleted)
	sort.Strings(matrix.Keywords)

	data, err := yaml.Marshal(matrix)
	if err != nil {
		t.Fatalf("marshal matrix: %v", err)
	}
	return data
}

// statusForCase resolves the honored|implemented annotation for a case set.
// Every declared case must carry a status, or the matrix generation fails.
func statusForCase(cases []string) string {
	for _, cas := range cases {
		if status := caseStatus[cas]; status != "" {
			return status
		}
	}
	return ""
}

// TestParadigmMatrixInvariants asserts the matrix's honest invariants:
// (1) every documented directive keyword is declared or listed as deleted,
// (2) the deleted list has no entry that is also declared, and
// (3) every declared directive has at least one conformance case and a case
// status annotation (honored|implemented).
func TestParadigmMatrixInvariants(t *testing.T) {
	declaredNames := map[string]bool{}
	declaredPairs := map[string]bool{}
	for _, contract := range paradigm.Registry.All() {
		for _, spec := range contract.Directives {
			declaredNames[spec.Name] = true
			declaredPairs[contract.Paradigm+"/"+spec.Name] = true
			caseCount := 0
			for _, cas := range contract.Conformance {
				if cas.Directive == spec.Name {
					caseCount++
					if caseStatus[cas.ID] == "" {
						t.Errorf("declared case %s/%s has no status annotation", contract.Paradigm, cas.ID)
					}
				}
			}
			if caseCount == 0 {
				t.Errorf("declared directive %s/%s has no conformance case", contract.Paradigm, spec.Name)
			}
		}
	}
	deletedKeywords := map[string]bool{}
	deletedPairs := map[string]bool{}
	for _, entry := range deletedDirectives {
		if _, directive, ok := strings.Cut(entry, "/"); ok {
			if declaredPairs[entry] {
				t.Errorf("deleted entry %q is still declared by a contract", entry)
			}
			deletedPairs[entry] = true
			deletedKeywords[directive] = true
			continue
		}
		if declaredNames[entry] {
			t.Errorf("deleted entry %q is still declared by a contract", entry)
		}
		deletedKeywords[entry] = true
	}
	for _, keyword := range documentedDirectiveKeywords {
		if !declaredNames[keyword] && !deletedKeywords[keyword] {
			t.Errorf("documented directive keyword %q is neither declared nor listed as deleted", keyword)
		}
	}
	// (4) every restored pair is declared and backed by a status-annotated case.
	for _, entry := range restoredDirectives {
		paradigmName, directive, ok := strings.Cut(entry, "/")
		if !ok {
			t.Errorf("restored entry %q is not paradigm/directive", entry)
			continue
		}
		if !declaredPairs[entry] {
			t.Errorf("restored pair %q is not declared by any contract", entry)
			continue
		}
		if deletedPairs[entry] {
			t.Errorf("restored pair %q also appears in the deleted list", entry)
		}
		contract, ok := paradigm.Registry.Lookup(paradigmName)
		if !ok {
			t.Errorf("restored pair %q names an unregistered paradigm", entry)
			continue
		}
		caseCount := 0
		for _, cas := range contract.Conformance {
			if cas.Directive == directive {
				caseCount++
				if caseStatus[cas.ID] == "" {
					t.Errorf("restored case %s has no status annotation", cas.ID)
				}
			}
		}
		if caseCount == 0 {
			t.Errorf("restored pair %q has no conformance case", entry)
		}
	}
	_ = declaredPairs
}
