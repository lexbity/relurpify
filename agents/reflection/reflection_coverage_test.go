package reflection

import (
	"context"
	"errors"
	"strings"
	"testing"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type stubTelemetry struct {
	events []core.Event
}

func (s *stubTelemetry) Emit(event core.Event) {
	s.events = append(s.events, event)
}

type envAwareDelegate struct {
	initEnvCalled bool
	initEnv       agentenv.AgentEnvironment
}

func (d *envAwareDelegate) Initialize(_ *core.Config) error { return nil }
func (d *envAwareDelegate) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{NodeID: "delegate", Success: true, Data: map[string]any{"summary": "passed"}}, nil
}
func (d *envAwareDelegate) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityExecute}
}
func (d *envAwareDelegate) BuildGraph(*core.Task) (*graph.Graph, error) { return nil, nil }
func (d *envAwareDelegate) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	d.initEnvCalled = true
	d.initEnv = env
	return nil
}

type registryDelegate struct {
	registry *capability.Registry
}

func (d *registryDelegate) Initialize(*core.Config) error { return nil }
func (d *registryDelegate) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}
func (d *registryDelegate) Capabilities() []core.Capability             { return nil }
func (d *registryDelegate) BuildGraph(*core.Task) (*graph.Graph, error) { return nil, nil }
func (d *registryDelegate) CapabilityRegistry() *capability.Registry {
	return d.registry
}

type reviewDelegate struct {
	executions int
}

func (d *reviewDelegate) Initialize(*core.Config) error { return nil }
func (d *reviewDelegate) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	d.executions++
	return &core.Result{NodeID: "delegate", Success: true, Data: map[string]any{"summary": "passed"}}, nil
}
func (d *reviewDelegate) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityExecute}
}
func (d *reviewDelegate) BuildGraph(*core.Task) (*graph.Graph, error) { return nil, nil }

func TestReflectionConstructionAndEnvironment(t *testing.T) {
	constructed := New(agentenv.AgentEnvironment{Config: &core.Config{Model: "default-model"}}, nil)
	if constructed == nil || constructed.Delegate == nil {
		t.Fatal("expected default delegate to be constructed")
	}

	env := agentenv.AgentEnvironment{
		Model: &recordingLLM{},
		Config: &core.Config{
			Model: "test-model",
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{},
			},
		},
	}
	delegate := &envAwareDelegate{}
	agent := &ReflectionAgent{Delegate: delegate}
	if err := agent.InitializeEnvironment(env); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if !delegate.initEnvCalled {
		t.Fatal("expected delegate environment initialization")
	}
	if agent.Reviewer == nil || agent.Config == nil {
		t.Fatal("expected environment wiring")
	}
	if agent.maxIterations != 3 {
		t.Fatalf("expected default max iterations, got %d", agent.maxIterations)
	}

	if err := agent.Initialize(&core.Config{MaxIterations: 5}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if agent.maxIterations != 5 {
		t.Fatalf("expected max iterations override, got %d", agent.maxIterations)
	}
}

func TestReflectionCapabilityRegistry(t *testing.T) {
	var nilAgent *ReflectionAgent
	if nilAgent.CapabilityRegistry() != nil {
		t.Fatal("expected nil registry on nil agent")
	}

	plain := &ReflectionAgent{Delegate: &reviewDelegate{}}
	if plain.CapabilityRegistry() != nil {
		t.Fatal("expected nil registry without provider")
	}

	provider := &registryDelegate{registry: capability.NewRegistry()}
	withProvider := &ReflectionAgent{Delegate: provider}
	if got := withProvider.CapabilityRegistry(); got == nil {
		t.Fatal("expected registry provider result")
	}
}

func TestReflectionExecuteAndHelpers(t *testing.T) {
	reviewer := &recordingLLM{
		response: &core.LLMResponse{Text: `{"issues":[],"approve":true}`},
	}
	delegate := &reviewDelegate{}
	agent := &ReflectionAgent{
		Reviewer: reviewer,
		Delegate: delegate,
		Config: &core.Config{
			Model:     "test-model",
			Telemetry: &stubTelemetry{},
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						ApprovalRules: core.AgentReviewApprovalRules{
							RequireVerificationEvidence: true,
						},
						SeverityWeights: map[string]float64{
							"high":   1.0,
							"medium": 0.5,
							"low":    0.2,
						},
					},
				},
			},
		},
	}
	cfg := &core.Config{
		Model:     "test-model",
		Telemetry: &stubTelemetry{},
		AgentSpec: &core.AgentRuntimeSpec{
			SkillConfig: core.AgentSkillConfig{
				Review: core.AgentReviewPolicy{
					ApprovalRules: core.AgentReviewApprovalRules{
						RequireVerificationEvidence: true,
					},
				},
			},
		},
	}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{ID: "task-1", Instruction: "Review the answer"}
	state := core.NewContext()
	state.Set("react.tool_observations", []reactpkg.ToolObservation{
		{Tool: "go_test", Success: true},
	})
	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if delegate.executions != 1 {
		t.Fatalf("expected one delegate execution, got %d", delegate.executions)
	}
	if got := reviewer.lastPrompt; !strings.Contains(got, "Review the following result for task \"Review the answer\"") {
		t.Fatalf("unexpected review prompt: %s", got)
	}
	if review, ok := state.Get("reflection.review"); !ok || review == nil {
		t.Fatal("expected review state to be populated")
	}
	if assessment, ok := state.Get("reflection.assessment"); !ok || assessment == nil {
		t.Fatal("expected assessment state to be populated")
	}
	if revise, ok := state.Get("reflection.revise"); !ok || revise != false {
		t.Fatalf("unexpected revise value: %v", revise)
	}

	node := &reflectionDecisionNode{id: "reflection_decide", agent: agent}
	state.Set("reflection.review", reviewPayload{Approve: false})
	state.Set("reflection.iteration", 0)
	res, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("decision node execute: %v", err)
	}
	if res == nil || res.Data["revise"] != true {
		t.Fatalf("expected revise=true, got %+v", res)
	}

	resolveState := core.NewContext()
	resolveState.SetHandle("reflection.last_result", &core.Result{Success: true})
	if resolved := resolveResultHandle(resolveState, "reflection.last_result"); resolved == nil {
		t.Fatal("expected resolved handle")
	}
	resolveState = core.NewContext()
	resolveState.Set("reflection.last_result", &core.Result{Success: true})
	if resolved := resolveResultHandle(resolveState, "reflection.last_result"); resolved == nil {
		t.Fatal("expected resolved state value")
	}
	if resolved := resolveResultHandle(nil, "reflection.last_result"); resolved != nil {
		t.Fatal("expected nil result for nil state")
	}

	if got := taskScope(&core.Task{ID: "task-2"}, nil); got != "task-2" {
		t.Fatalf("unexpected task scope %q", got)
	}
	scopeState := core.NewContext()
	scopeState.Set("task.id", "scope-from-state")
	if got := taskScope(nil, scopeState); got != "scope-from-state" {
		t.Fatalf("unexpected task scope fallback %q", got)
	}

	if review, err := parseReview("```json\n{\"issues\":[{\"severity\":\"low\",\"description\":\"d\",\"suggestion\":\"s\"}],\"approve\":false}\n```"); err != nil || review.Approve {
		t.Fatalf("unexpected parsed review: %+v err=%v", review, err)
	}

	if ok := reflectionApprovalPasses(nil, state, reviewPayload{Approve: true}); !ok {
		t.Fatal("expected approval to pass with nil agent")
	}
	if ok := reflectionApprovalPasses(&ReflectionAgent{}, state, reviewPayload{Approve: true}); !ok {
		t.Fatal("expected approval to pass without configured policy")
	}

	assessment := reflectionAssessmentForReview(&ReflectionAgent{}, state, reviewPayload{Approve: true, Issues: []struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	}{{Severity: "low"}}})
	if !assessment.Allowed || assessment.UnresolvedIssueCount != 1 {
		t.Fatalf("unexpected default assessment: %+v", assessment)
	}

	if got := reflectionSeverityWeight(map[string]float64{"medium": 0.4}, "unknown"); got != 0.4 {
		t.Fatalf("unexpected severity weight %v", got)
	}
	if got := reflectionApprovalThreshold(map[string]float64{}); got != 0.2 {
		t.Fatalf("unexpected threshold %v", got)
	}
	if got := reflectionMinInt(2, 5); got != 2 {
		t.Fatalf("unexpected min int %d", got)
	}
	if got := uniqueStrings([]string{" a ", "", "a", "b", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected unique strings: %+v", got)
	}
	if got := truncateReflectionString(strings.Repeat("x", reflectionMaxStringLen+20)); !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("expected truncated string, got %q", got)
	}
	if got := compactResultForReview(nil); got["present"] != false {
		t.Fatalf("unexpected compact nil result: %+v", got)
	}
	longResult := &core.Result{
		NodeID:  "node-1",
		Success: true,
		Error:   errors.New(strings.Repeat("error ", 100)),
		Data: map[string]any{
			"items": []string{"one", "two", "three", "four", "five", "six", "seven"},
		},
	}
	compact := compactResultForReview(longResult)
	if compact["present"] != true || compact["node_id"] != "node-1" {
		t.Fatalf("unexpected compact result: %+v", compact)
	}
	if _, ok := compact["error"]; !ok {
		t.Fatal("expected compact error")
	}
	if _, ok := compact["data"]; !ok {
		t.Fatal("expected compact data")
	}

	if got := compactReflectionValue("hello", 0); got != "hello" {
		t.Fatalf("unexpected compact string %v", got)
	}
	if got := compactReflectionValue([]string{"a", "b", "c", "d", "e", "f", "g"}, 0); len(got.([]any)) != 7 {
		t.Fatalf("unexpected compact string slice: %+v", got)
	}
	if got := compactReflectionValue([]any{"a", map[string]any{"nested": "value"}}, 0); len(got.([]any)) != 2 {
		t.Fatalf("unexpected compact any slice: %+v", got)
	}
	wideMap := map[string]any{}
	for i := 0; i < reflectionMaxMapItems+2; i++ {
		wideMap[string(rune('a'+i))] = i
	}
	if got := compactReflectionMap(wideMap, 0); got["_truncated_keys"] != 2 {
		t.Fatalf("unexpected truncated map: %+v", got)
	}

	if got := hasVerificationEvidence(core.NewContext()); got {
		t.Fatal("expected no verification evidence")
	}
	evidenceState := core.NewContext()
	evidenceState.Set("react.tool_observations", []reactpkg.ToolObservation{
		{Tool: "go_test", Success: true},
	})
	if !hasVerificationEvidence(evidenceState) {
		t.Fatal("expected verification evidence from tool observations")
	}
	resultState := core.NewContext()
	resultState.Set("reflection.last_result", &core.Result{Success: true, Data: map[string]any{"summary": "passed"}})
	if !hasVerificationEvidence(resultState) {
		t.Fatal("expected verification evidence from result data")
	}
}

func TestReflectionSmallHelpers(t *testing.T) {
	if got := (&ReflectionAgent{}).Capabilities(); len(got) != 1 || got[0] != core.CapabilityReview {
		t.Fatalf("unexpected capabilities: %+v", got)
	}

	guidance := reflectionSeverityGuidance(map[string]float64{"high": 1.0, "low": 0.2})
	if !strings.Contains(guidance, "high") || !strings.Contains(guidance, "low") {
		t.Fatalf("unexpected severity guidance: %s", guidance)
	}
}
