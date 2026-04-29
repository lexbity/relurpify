package recipe

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	blackboardagent "codeburg.org/lexbit/relurpify/agents/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconagent "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnagent "codeburg.org/lexbit/relurpify/agents/htn"
	pipelineagent "codeburg.org/lexbit/relurpify/agents/pipeline"
	planneragent "codeburg.org/lexbit/relurpify/agents/planner"
	reactagent "codeburg.org/lexbit/relurpify/agents/react"
	reflectionagent "codeburg.org/lexbit/relurpify/agents/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// RecipeStepNode executes a compiled recipe step by delegating to the matching
// /agents constructor for the step's paradigm.
type RecipeStepNode struct {
	id      string
	env     agentenv.WorkspaceEnvironment
	step    ExecutionStep
	trigger *contextstream.Trigger
}

// NewRecipeStepNode creates a new agent-backed recipe step node.
func NewRecipeStepNode(id string, env agentenv.WorkspaceEnvironment, step ExecutionStep, trigger *contextstream.Trigger) *RecipeStepNode {
	return &RecipeStepNode{
		id:      id,
		env:     env,
		step:    step,
		trigger: trigger,
	}
}

// ID implements agentgraph.Node.
func (n *RecipeStepNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *RecipeStepNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute builds the selected paradigm agent, runs it, and writes captures back
// to the envelope.
func (n *RecipeStepNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("recipe step node is nil")
	}
	if env == nil {
		return nil, fmt.Errorf("recipe step node %q missing envelope", n.id)
	}

	task := n.buildTask(env)
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, env)
	if result == nil {
		result = &agentgraph.Result{
			NodeID:  n.id,
			Success: execErr == nil,
			Data:    map[string]any{},
		}
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	}

	captured := n.captureValues(result)
	for key, value := range captured {
		env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".result", result.Data, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".success", result.Success, contextdata.MemoryClassTask)
	if result.Error != "" {
		env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".error", result.Error, contextdata.MemoryClassTask)
	}

	if execErr != nil {
		// Preserve the failure in the result while keeping graph control flow
		// conditional instead of aborting immediately.
		return result, nil
	}
	return result, nil
}

func (n *RecipeStepNode) buildTask(env *contextdata.Envelope) *core.Task {
	data := recipeTemplateData(env, n.step)
	instruction := n.renderTemplate(n.step.Prompt, data)
	if instruction == "" {
		instruction = n.step.Prompt
	}
	task := &core.Task{
		ID:          n.id,
		Type:        n.step.Paradigm,
		Instruction: instruction,
		Data:        make(map[string]interface{}),
		Context:     data,
		Metadata: map[string]interface{}{
			"recipe_step_id":  n.step.ID,
			"recipe_paradigm": n.step.Paradigm,
			"recipe_mutation": n.step.Mutation,
			"recipe_hitl":     n.step.HITL,
		},
	}
	if len(n.step.Bindings) > 0 {
		for key, ref := range n.step.Bindings {
			if value, ok := lookupTemplateValue(data, ref); ok {
				task.Context[key] = value
			}
		}
	}
	return task
}

func (n *RecipeStepNode) buildAgent(task *core.Task) (agentgraph.WorkflowExecutor, error) {
	scopedEnv := n.env
	if scopedRegistry := n.scopedRegistry(); scopedRegistry != nil {
		scopedEnv = scopedEnv.WithRegistry(scopedRegistry)
	}

	switch strings.ToLower(strings.TrimSpace(n.step.Paradigm)) {
	case "react":
		return reactagent.New(&scopedEnv, n.streamOptions()...), nil
	case "planner":
		return planneragent.New(&scopedEnv), nil
	case "htn":
		primitive := reactagent.New(&scopedEnv, n.streamOptions()...)
		return htnagent.New(&scopedEnv, htnagent.NewMethodLibrary(), append([]htnagent.Option{
			htnagent.WithPrimitiveExec(primitive),
		}, n.streamOptionsHTN()...)...), nil
	case "reflection":
		delegate := reactagent.New(&scopedEnv, n.streamOptions()...)
		return reflectionagent.New(&scopedEnv, delegate), nil
	case "blackboard":
		return blackboardagent.New(&scopedEnv, n.streamOptionsBlackboard()...), nil
	case "chainer":
		return chaineragent.New(&scopedEnv, n.streamOptionsChainer()...), nil
	case "pipeline":
		return pipelineagent.New(&scopedEnv, n.streamOptionsPipeline()...), nil
	case "rewoo":
		agent := rewooagent.New(&scopedEnv)
		agent.Options = n.rewooOptions()
		return agent, nil
	case "goalcon":
		agent := goalconagent.New(&scopedEnv, goalconagent.DefaultOperatorRegistry(), n.streamOptionsGoalCon()...)
		if agent != nil && agent.PlanExecutor == nil {
			agent.PlanExecutor = reactagent.New(&scopedEnv, n.streamOptions()...)
		}
		return agent, nil
	default:
		return nil, fmt.Errorf("recipe step %q has unsupported paradigm %q", n.step.ID, n.step.Paradigm)
	}
}

func (n *RecipeStepNode) scopedRegistry() *capability.Registry {
	if n == nil || n.env.Registry == nil {
		return nil
	}
	allowed := extractAllowedCapabilities(n.step.Step)
	if len(allowed) == 0 {
		return nil
	}
	return n.env.Registry.WithAllowlist(allowed)
}

func (n *RecipeStepNode) streamOptions() []reactagent.Option {
	opts := make([]reactagent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, reactagent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, reactagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, reactagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, reactagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsHTN() []htnagent.Option {
	opts := make([]htnagent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, htnagent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, htnagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, htnagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, htnagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsBlackboard() []blackboardagent.Option {
	opts := make([]blackboardagent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, blackboardagent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, blackboardagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, blackboardagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, blackboardagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsChainer() []chaineragent.Option {
	opts := make([]chaineragent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, chaineragent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, chaineragent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, chaineragent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, chaineragent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsPipeline() []pipelineagent.Option {
	opts := make([]pipelineagent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, pipelineagent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, pipelineagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, pipelineagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, pipelineagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsGoalCon() []goalconagent.Option {
	opts := make([]goalconagent.Option, 0, 4)
	if n.trigger != nil {
		opts = append(opts, goalconagent.WithContextStreamTrigger(n.trigger))
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, goalconagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, goalconagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, goalconagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) rewooOptions() rewooagent.RewooOptions {
	opts := rewooagent.RewooOptions{}
	if n.trigger != nil {
		opts.StreamTrigger = n.trigger
	}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts.StreamMode = contextstream.Mode(mode)
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts.StreamQuery = query
		}
		if n.step.Stream.MaxTokens > 0 {
			opts.StreamMaxTokens = n.step.Stream.MaxTokens
		}
	}
	return opts
}

func (n *RecipeStepNode) captureValues(result *agentgraph.Result) map[string]any {
	if n == nil || result == nil {
		return nil
	}
	captures := n.step.Captures
	if len(captures) == 0 {
		return nil
	}
	out := make(map[string]any, len(captures))
	for alias, key := range captures {
		value, ok := lookupCaptureValue(result.Data, alias)
		if !ok {
			if len(result.Data) == 1 {
				for _, candidate := range result.Data {
					value = candidate
					ok = true
				}
			}
		}
		if !ok {
			value = result.Data
		}
		out[key] = value
	}
	return out
}

func recipeTemplateData(env *contextdata.Envelope, step ExecutionStep) map[string]any {
	data := map[string]any{
		"TaskID":    "",
		"SessionID": "",
		"StepID":    step.ID,
		"Paradigm":  step.Paradigm,
		"Prompt":    step.Prompt,
	}
	if env != nil {
		data["TaskID"] = env.TaskID
		data["SessionID"] = env.SessionID
		for key, value := range env.Snapshot() {
			data[key] = value
			suffix := aliasFromEnvelopeKey(key)
			if suffix != "" {
				if _, exists := data[suffix]; !exists {
					data[suffix] = value
				}
			}
			normalized := normalizeTemplateKey(key)
			if normalized != "" {
				if _, exists := data[normalized]; !exists {
					data[normalized] = value
				}
			}
		}
	}
	return data
}

func (n *RecipeStepNode) renderTemplate(src string, data map[string]any) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	src = normalizeRecipeTemplate(src)
	tpl, err := template.New("recipe-step").Option("missingkey=zero").Parse(src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return src
	}
	return buf.String()
}

func lookupTemplateValue(data map[string]any, ref string) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || data == nil {
		return nil, false
	}
	if value, ok := data[ref]; ok {
		return value, true
	}
	if value, ok := data[aliasFromEnvelopeKey(ref)]; ok {
		return value, true
	}
	return nil, false
}

func lookupCaptureValue(data map[string]any, alias string) (any, bool) {
	alias = strings.TrimSpace(alias)
	if alias == "" || data == nil {
		return nil, false
	}
	if value, ok := data[alias]; ok {
		return value, true
	}
	if value, ok := data[normalizeTemplateKey(alias)]; ok {
		return value, true
	}
	return nil, false
}

func aliasFromEnvelopeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.FieldsFunc(key, func(r rune) bool {
		switch r {
		case '.', '/', ':':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func normalizeTemplateKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	replacer := strings.NewReplacer(".", "_", "-", "_", "/", "_", ":", "_")
	return replacer.Replace(key)
}

var simpleTemplatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

func normalizeRecipeTemplate(src string) string {
	if src == "" {
		return ""
	}
	return simpleTemplatePattern.ReplaceAllString(src, "{{.$1}}")
}

func extractAllowedCapabilities(step RecipeStep) []string {
	if len(step.Config) == 0 {
		return nil
	}
	return extractAllowedCapabilitiesFromMap(step.Config)
}

func extractAllowedCapabilitiesFromMap(data map[string]any) []string {
	if len(data) == 0 {
		return nil
	}
	if nested, ok := data["capabilities"].(map[string]any); ok {
		if allowed := stringsFromAny(nested["allowed"]); len(allowed) > 0 {
			return allowed
		}
	}
	if allowed := stringsFromAny(data["capabilities.allowed"]); len(allowed) > 0 {
		return allowed
	}
	if allowed := stringsFromAny(data["allowed_capabilities"]); len(allowed) > 0 {
		return allowed
	}
	return nil
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

func sortedKeys(data map[string]any) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
