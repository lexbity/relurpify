package testfu

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func parseRequest(task *core.Task) runRequest {
	req := runRequest{
		Action:    actionRunSuite,
		Workspace: workspaceFromContext(task),
	}
	values := map[string]string{}
	if task != nil && task.Context != nil {
		for _, key := range []string{"action", "suite_path", "case_name", "agent_name", "tags", "lane", "model", "endpoint", "timeout", "workspace"} {
			if value := strings.TrimSpace(fmt.Sprint(task.Context[key])); value != "" && value != "<nil>" {
				values[key] = value
			}
		}
	}
	if task != nil {
		for _, line := range strings.Split(task.Instruction, "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key = strings.TrimSpace(strings.ToLower(key))
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}
			switch key {
			case "action", "suite", "suite_path", "case", "case_name", "agent_name", "tags", "lane", "model", "endpoint", "timeout", "workspace":
				if key == "suite" {
					key = "suite_path"
				}
				if key == "case" {
					key = "case_name"
				}
				values[key] = value
			}
		}
	}
	if req.Workspace == "" {
		req.Workspace = values["workspace"]
	}
	if req.Workspace == "" {
		req.Workspace = "."
	}
	if value := strings.TrimSpace(values["action"]); value != "" {
		req.Action = action(strings.ToLower(value))
	}
	if value := strings.TrimSpace(values["suite_path"]); value != "" {
		req.SuitePath = value
	}
	if value := strings.TrimSpace(values["case_name"]); value != "" {
		req.CaseName = value
		req.Action = actionRunCase
	}
	if value := strings.TrimSpace(values["agent_name"]); value != "" {
		req.AgentName = value
		if req.Action == actionRunSuite || req.Action == "" {
			req.Action = actionRunAgent
		}
	}
	if value := strings.TrimSpace(values["tags"]); value != "" {
		for _, tag := range strings.Split(value, ",") {
			if t := strings.TrimSpace(tag); t != "" {
				req.Tags = append(req.Tags, t)
			}
		}
	}
	req.Lane = strings.TrimSpace(values["lane"])
	req.Model = values["model"]
	req.Endpoint = values["endpoint"]
	if value := strings.TrimSpace(values["timeout"]); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			req.Timeout = parsed
		}
	}
	if req.SuitePath == "" && task != nil {
		instruction := strings.TrimSpace(task.Instruction)
		if strings.HasSuffix(instruction, ".yaml") || strings.HasSuffix(instruction, ".yml") {
			req.SuitePath = instruction
		}
	}
	if req.Action == "" {
		req.Action = actionRunSuite
	}
	if strings.Contains(strings.ToLower(taskInstruction(task)), "list suites") {
		req.Action = actionListSuites
	}
	if strings.HasPrefix(strings.ToLower(taskInstruction(task)), "list_suites") {
		req.Action = actionListSuites
	}
	return req
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func resolveSuitePath(workspace, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("testfu: suite_path required")
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	return filepath.Join(workspace, raw), nil
}
