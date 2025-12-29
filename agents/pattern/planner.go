package pattern

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"strings"
)

// PlannerAgent builds a plan before executing. It is intentionally explicit:
// first ask the LLM for a structured plan, then execute tool-backed steps,
// finally verify + summarize. The separation mirrors how human operators would
// tackle unfamiliar tasks and serves as reference implementation for creating
// new multi-step agents.
type PlannerAgent struct {
	Model  core.LanguageModel
	Tools  *toolsys.ToolRegistry
	Memory memory.MemoryStore
	Config *core.Config
}

// Initialize configures the agent.
func (a *PlannerAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = toolsys.NewToolRegistry()
	}
	return nil
}

// Execute runs the planner workflow.
func (a *PlannerAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	return graph.Execute(ctx, state)
}

// Capabilities enumerates features.
func (a *PlannerAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
	}
}

// BuildGraph builds planning pipeline with explicit plan→execute→verify stages.
// Returning a Graph instead of hiding the workflow inside Execute keeps the
// system debuggable and allows other packages to analyze the structure.
func (a *PlannerAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("planner agent missing model")
	}
	planNode := &plannerPlanNode{id: "planner_plan", agent: a, task: task}
	execNode := &plannerExecuteNode{id: "planner_execute", agent: a}
	verifyNode := &plannerVerifyNode{id: "planner_verify", agent: a, task: task}
	return graph.BuildPlanExecuteVerifyGraph(planNode, execNode, verifyNode, "planner_done")
}

type plannerPlanNode struct {
	id    string
	agent *PlannerAgent
	task  *core.Task
}

// ID returns the stable graph identifier.
func (n *plannerPlanNode) ID() string { return n.id }

// Type labels the node as a system step for graph visualization.
func (n *plannerPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

// Execute prompts the LLM for a machine-readable plan. The JSON schema is small
// enough that contributors can tweak it without retraining anything.
func (n *plannerPlanNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("planning")
	extraPrompt := ""
	if n.agent != nil && n.agent.Config != nil && n.agent.Config.AgentSpec != nil {
		extraPrompt = strings.TrimSpace(n.agent.Config.AgentSpec.Prompt)
	}
	if extraPrompt != "" {
		extraPrompt = fmt.Sprintf("Additional Guidance:\n%s\n\n", extraPrompt)
	}
	prompt := fmt.Sprintf(`You are a planning agent. Break this task into steps with dependencies.
%sTask: %s
Return valid JSON Plan struct with fields goal, steps (array of {id, description, tool, params, expected, verification, files}), dependencies (map of step id -> [step id]), files.
Use string step ids (UUID-safe).
`, extraPrompt, n.task.Instruction)
	resp, err := n.agent.Model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       n.agent.Config.Model,
		Temperature: 0.2,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, err
	}
	state.AddInteraction("assistant", resp.Text, map[string]interface{}{"node": n.id})
	plan, err := parsePlan(resp.Text)
	if err != nil {
		return nil, err
	}
	state.Set("planner.plan", plan)
	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"type": "plan",
			"plan": plan,
		}, memory.MemoryScopeSession)
	}
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"plan":       plan,
		"plan_steps": plan.Steps,
		"files":      plan.Files,
	}}, nil
}

type plannerExecuteNode struct {
	id    string
	agent *PlannerAgent
}

// ID returns the identifier seen by the framework.
func (n *plannerExecuteNode) ID() string { return n.id }

// Type signals to the graph visualizer that this step consumes tools.
func (n *plannerExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

// Execute iterates the generated plan and calls the requested tool for each
// actionable step. Empty tool names are skipped, which keeps the agent tolerant
// to “reasoning only” steps the LLM might propose.
func (n *plannerExecuteNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("executing")
	value, ok := state.Get("planner.plan")
	if !ok {
		return nil, fmt.Errorf("plan not available")
	}
	plan, _ := value.(core.Plan)
	var stepResults []map[string]interface{}
	for _, step := range plan.Steps {
		if step.Tool == "" {
			continue
		}
		tool, ok := n.agent.Tools.Get(step.Tool)
		if !ok {
			return nil, fmt.Errorf("tool %s not registered", step.Tool)
		}
		result, err := tool.Execute(ctx, state, step.Params)
		if err != nil {
			return nil, err
		}
		stepResults = append(stepResults, map[string]interface{}{
			"id":     step.ID,
			"output": result.Data,
		})
		state.Set(fmt.Sprintf("planner.step.%s", step.ID), result.Data)
	}
	state.Set("planner.results", stepResults)
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{"results": stepResults}}, nil
}

type plannerVerifyNode struct {
	id    string
	agent *PlannerAgent
	task  *core.Task
}

// ID returns the verifying node identifier.
func (n *plannerVerifyNode) ID() string { return n.id }

// Type marks this node as an observation/validation phase.
func (n *plannerVerifyNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Execute packages the observed tool outputs into a short summary so downstream
// systems (CLI, LSP, tests) can display human-friendly “what just happened”
// messages without parsing the entire state map.
func (n *plannerVerifyNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("validating")
	results, _ := state.Get("planner.results")
	planVal, _ := state.Get("planner.plan")
	plan, _ := planVal.(core.Plan)
	summary := fmt.Sprintf("Executed plan for task '%s' with %d steps.", n.task.Instruction, len(plan.Steps))
	state.Set("planner.summary", summary)
	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"type":    "verification",
			"summary": summary,
			"results": results,
		}, memory.MemoryScopeSession)
	}
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"summary": summary,
		},
	}, nil
}

// parsePlan pulls the JSON payload out of the model response. The helper keeps
// PlannerAgent.Execute easy to read and doubles as a seam for unit tests.
func parsePlan(raw string) (core.Plan, error) {
	var plan core.Plan
	if err := json.Unmarshal([]byte(ExtractJSON(raw)), &plan); err != nil {
		return plan, err
	}
	if plan.Dependencies == nil {
		plan.Dependencies = make(map[string][]string)
	}
	if plan.Files == nil {
		plan.Files = make([]string, 0)
	}
	return plan, nil
}
