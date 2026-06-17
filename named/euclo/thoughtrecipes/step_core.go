package thoughtrecipe

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	blackboardagent "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	goalconagent "codeburg.org/lexbit/relurpify/cognitionzoo/goalcon"
	htnagent "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pipelineagent "codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	planneragent "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	reactagent "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	reflectionagent "codeburg.org/lexbit/relurpify/cognitionzoo/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

const (
	executionCapabilityIDKey = "execution_capability_id"
)

// stepCore is the shared plumbing embedded by all per-kind node types.
type stepCore struct {
	id   string
	deps *paradigm.Deps
	step ExecutionStep
}

func (c *stepCore) ID() string                    { return c.id }
func (c *stepCore) NodeType() agentgraph.NodeType { return agentgraph.NodeTypeTool }

func (c *stepCore) buildTask(ctx context.Context, env *contextdata.Envelope) (*execution.Task, error) {
	data := thoughtrecipeTemplateData(env, c.step)
	c.writeStepMetadata(env)

	var instruction string
	if c.step.PromptID != "" {
		if c.deps.PromptRegistry == nil {
			return nil, fmt.Errorf("thoughtrecipe step %q: prompt_id requires a registry", c.step.ID)
		}
		var err error
		instruction, err = c.resolveFromRegistry(ctx, env)
		if err != nil {
			return nil, err
		}
	}

	if instruction == "" {
		if strings.TrimSpace(c.step.Question) != "" {
			instruction = c.renderTemplate(c.step.Question, data)
		}
	}
	if instruction == "" {
		if strings.TrimSpace(c.step.Goal) != "" {
			instruction = c.renderTemplate(c.step.Goal, data)
		}
	}
	if instruction == "" {
		instruction = c.renderTemplate(c.step.Prompt, data)
		if instruction == "" {
			instruction = c.step.Prompt
		}
	}

	task := &execution.Task{
		ID:          c.id,
		Type:        c.step.Paradigm,
		Instruction: instruction,
		Data:        make(map[string]any),
		Context:     data,
		Metadata:    c.stepMetadata(),
	}

	if c.step.PromptID != "" {
		task.Context["prompt_id"] = c.step.PromptID
	}

	return task, nil
}

func (c *stepCore) buildAgent(task *execution.Task) (agentgraph.WorkflowExecutor, error) {
	deps := c.deps
	if scopedRegistry := c.scopedRegistry(); scopedRegistry != nil {
		deps = depsWithRegistry(deps, scopedRegistry)
	}

	switch strings.ToLower(strings.TrimSpace(c.step.Paradigm)) {
	case "react":
		return reactagent.New(deps, c.streamOptions()...), nil
	case "planner":
		return planneragent.New(deps), nil
	case "htn":
		primitive := reactagent.New(deps, c.streamOptions()...)
		return htnagent.New(deps, htnagent.NewMethodLibrary(), append([]htnagent.Option{
			htnagent.WithPrimitiveExec(primitive),
		}, c.streamOptionsHTN()...)...), nil
	case "reflection":
		delegate := reactagent.New(deps, c.streamOptions()...)
		return reflectionagent.New(deps, delegate), nil
	case "blackboard":
		return blackboardagent.New(deps, c.streamOptionsBlackboard()...), nil
	case "chainer":
		return chaineragent.New(deps, c.streamOptionsChainer()...), nil
	case "pipeline":
		return pipelineagent.New(deps, c.streamOptionsPipeline()...), nil
	case "rewoo":
		agent := rewooagent.New(deps)
		agent.Options = c.rewooOptions()
		return agent, nil
	case "goalcon":
		agent := goalconagent.New(deps, goalconagent.DefaultOperatorRegistry(), c.streamOptionsGoalCon()...)
		if agent != nil && agent.PlanExecutor == nil {
			agent.PlanExecutor = reactagent.New(deps, c.streamOptions()...)
		}
		return agent, nil
	default:
		return nil, fmt.Errorf("thoughtrecipe step %q has unsupported paradigm %q", c.step.ID, c.step.Paradigm)
	}
}

func (c *stepCore) writeCaptures(env *contextdata.Envelope, result *execution.Result) error {
	if len(c.step.CaptureBindings) > 0 {
		_, err := ApplyCaptureBindings(env, c.step.CaptureBindings, execution.ResultFields(result.Data))
		return err
	}
	return nil
}

func (c *stepCore) renderTemplate(src string, data map[string]any) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	tpl, err := template.New("thoughtrecipe-step").Option("missingkey=zero").Parse(src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return src
	}
	return buf.String()
}

func mustRouteKind(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, "euclo.dispatch.route_kind")
	return strings.TrimSpace(v)
}

func thoughtrecipeTemplateData(env *contextdata.Envelope, step ExecutionStep) map[string]any {
	data := map[string]any{
		"TaskID":    "",
		"SessionID": "",
		"StepID":    step.ID,
		"Paradigm":  step.Paradigm,
		"Prompt":    step.Prompt,
		"Goal":      step.Goal,
	}
	if len(step.Sources) > 0 {
		data["RunSources"] = append([]string(nil), step.Sources...)
	}
	if len(step.Directives) > 0 {
		data["RunDirectives"] = append([]string(nil), step.Directives...)
	}
	if env != nil {
		data["TaskID"] = env.TaskID
		data["SessionID"] = env.SessionID
		for key, value := range env.Snapshot() {
			data[key] = value
		}
	}
	return data
}

func stringsFromAny(v any) []string {
	switch typed := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func lookupTemplateValue(data map[string]any, ref string) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || data == nil {
		return nil, false
	}
	value, ok := data[ref]
	return value, ok
}

// newNodeForStep creates the appropriate node type for the step's Kind.
func newNodeForStep(id string, deps *paradigm.Deps, step ExecutionStep) agentgraph.Node {
	switch step.Kind {
	case StepKindDelegate:
		return NewDelegateNode(id, deps, step)
	case StepKindAsk:
		return NewAskNode(id, deps, step)
	case StepKindCapability:
		return NewCapabilityNode(id, deps, step)
	default:
		return NewRunNode(id, deps, step)
	}
}

// NewThoughtRecipeStepNode creates the appropriate node type for the step.
// Deprecated: use per-kind constructors (NewRunNode, NewDelegateNode, etc.).
func NewThoughtRecipeStepNode(id string, deps *paradigm.Deps, step ExecutionStep) agentgraph.Node {
	return newNodeForStep(id, deps, step)
}

func askFrameKey(stepID string) string {
	return "euclo.execution.ask." + sanitizeComponent(stepID) + ".frame"
}
