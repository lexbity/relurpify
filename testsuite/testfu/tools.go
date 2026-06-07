package testfu

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func (a *Agent) registerTools() {
	if a == nil || a.Tools == nil {
		return
	}
	for _, tool := range []ports.Tool{
		&agentTool{agent: a, name: "testfu:run_suite", description: "Run an agenttest suite."},
		&agentTool{agent: a, name: "testfu:run_case", description: "Run a single case from an agenttest suite."},
		&agentTool{agent: a, name: "testfu:list_suites", description: "List available agenttest suites."},
		&agentTool{agent: a, name: "testfu:run_agent_suites", description: "Run all testsuite YAML files matching an agent name."},
		&assertPassedTool{},
	} {
		if _, ok := a.Tools.Get(tool.Name()); ok {
			continue
		}
		_ = a.Tools.Register(tool)
	}
}

type agentTool struct {
	agent       *Agent
	name        string
	description string
}

func (t *agentTool) Name() string        { return t.name }
func (t *agentTool) Description() string { return t.description }
func (t *agentTool) Category() string    { return "test" }
func (t *agentTool) Parameters() []ports.ToolParameter {
	if t.name == "testfu:run_agent_suites" {
		return []ports.ToolParameter{
			{Name: "agent_name", Type: "string", Description: "Agent name to match suite files (e.g. 'react').", Required: true},
			{Name: "tags", Type: "string", Description: "Comma-separated tag filter applied before running.", Required: false},
			{Name: "lane", Type: "string", Description: "Optional lane name passed to RunOptions.", Required: false},
			{Name: "timeout", Type: "string", Description: "Optional per-suite timeout duration.", Required: false},
		}
	}
	return []ports.ToolParameter{
		{Name: "suite_path", Type: "string", Description: "Path to the testsuite YAML.", Required: t.name != "testfu:list_suites"},
		{Name: "case_name", Type: "string", Description: "Specific case name to run.", Required: false},
		{Name: "model", Type: "string", Description: "Optional model override.", Required: false},
		{Name: "endpoint", Type: "string", Description: "Optional endpoint override.", Required: false},
		{Name: "timeout", Type: "string", Description: "Optional timeout duration.", Required: false},
	}
}
func (t *agentTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	if t == nil || t.agent == nil {
		return nil, fmt.Errorf("testfu tool unavailable")
	}
	task := &execution.Task{Instruction: "", Context: map[string]any{}}
	switch t.name {
	case "testfu:list_suites":
		task.Instruction = "list_suites"
	case "testfu:run_agent_suites":
		task.Context["agent_name"] = args["agent_name"]
		task.Context["tags"] = args["tags"]
		task.Context["lane"] = args["lane"]
		task.Context["timeout"] = args["timeout"]
		task.Context["workspace"] = t.agent.Workspace
		task.Context["action"] = string(actionRunAgent)
	default:
		task.Context["suite_path"] = args["suite_path"]
		task.Context["case_name"] = args["case_name"]
		task.Context["model"] = args["model"]
		task.Context["endpoint"] = args["endpoint"]
		task.Context["timeout"] = args["timeout"]
		task.Context["workspace"] = t.agent.Workspace
		if t.name == "testfu:run_case" {
			task.Context["action"] = string(actionRunCase)
		}
		if t.name == "testfu:run_suite" {
			task.Context["action"] = string(actionRunSuite)
		}
	}
	env := contextdata.NewEnvelope(task.ID, "")
	result, err := t.agent.Execute(ctx, task, env)
	if err != nil {
		return nil, err
	}
	return &ports.ToolResult{Success: result.Success, Data: execution.ResultFields(result.Data)}, nil
}
func (t *agentTool) IsAvailable(context.Context) bool { return true }
func (t *agentTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *agentTool) Tags() []string { return []string{toolcapabilities.TagExecute} }

type assertPassedTool struct{}

func (t *assertPassedTool) Name() string        { return "testfu:assert_passed" }
func (t *assertPassedTool) Description() string { return "Assert that a testfu report or case passed." }
func (t *assertPassedTool) Category() string    { return "test" }
func (t *assertPassedTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{{Name: "passed", Type: "boolean", Description: "Passed flag from a prior testfu result.", Required: true}}
}
func (t *assertPassedTool) Execute(_ context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	passed, _ := args["passed"].(bool)
	if passed {
		return &ports.ToolResult{Success: true, Data: map[string]interface{}{"passed": true}}, nil
	}
	return &ports.ToolResult{Success: false, Error: "report indicates failure", Data: map[string]interface{}{"passed": false}}, nil
}
func (t *assertPassedTool) IsAvailable(context.Context) bool { return true }
func (t *assertPassedTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *assertPassedTool) Tags() []string { return []string{toolcapabilities.TagReadOnly} }
