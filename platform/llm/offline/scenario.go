package offline

import (
	"fmt"
	"strconv"
	"strings"
)

type ScenarioKind string

const (
	ScenarioGreeting    ScenarioKind = "greeting"
	ScenarioEcho        ScenarioKind = "echo"
	ScenarioFileRead    ScenarioKind = "tool:file_read"
	ScenarioExecRunCode ScenarioKind = "tool:exec_run_code"
	ScenarioCliGit      ScenarioKind = "tool:cli_git"
	ScenarioHITL        ScenarioKind = "hitl"
	ScenarioMulti       ScenarioKind = "multi"
	ScenarioError       ScenarioKind = "error"
)

type Scenario struct {
	Kind      ScenarioKind
	ToolArg   string
	MultiStep int
}

func ParseScenario(raw any) (Scenario, error) {
	switch v := raw.(type) {
	case nil:
		return Scenario{Kind: ScenarioGreeting}, nil
	case string:
		return parseScenarioString(v)
	default:
		return Scenario{}, fmt.Errorf("offline_scenario must be a string, got %T", raw)
	}
}

func parseScenarioString(raw string) (Scenario, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Scenario{Kind: ScenarioGreeting}, nil
	}
	switch {
	case raw == "echo":
		return Scenario{Kind: ScenarioEcho}, nil
	case raw == "hitl":
		return Scenario{Kind: ScenarioHITL}, nil
	case raw == "error":
		return Scenario{Kind: ScenarioError}, nil
	case raw == "tool:file_read":
		return Scenario{}, fmt.Errorf("offline_scenario %q missing path", raw)
	case strings.HasPrefix(raw, "tool:file_read:"):
		return Scenario{Kind: ScenarioFileRead, ToolArg: strings.TrimPrefix(raw, "tool:file_read:")}, nil
	case strings.HasPrefix(raw, "tool:exec_run_code:"):
		return Scenario{Kind: ScenarioExecRunCode, ToolArg: strings.TrimPrefix(raw, "tool:exec_run_code:")}, nil
	case strings.HasPrefix(raw, "tool:cli_git:"):
		return Scenario{Kind: ScenarioCliGit, ToolArg: strings.TrimPrefix(raw, "tool:cli_git:")}, nil
	case strings.HasPrefix(raw, "multi:"):
		count, err := strconv.Atoi(strings.TrimPrefix(raw, "multi:"))
		if err != nil {
			return Scenario{}, fmt.Errorf("offline_scenario %q invalid multi count: %w", raw, err)
		}
		if count < 0 {
			return Scenario{}, fmt.Errorf("offline_scenario %q multi count must be >= 0", raw)
		}
		return Scenario{Kind: ScenarioMulti, MultiStep: count}, nil
	default:
		return Scenario{}, fmt.Errorf("offline_scenario %q unsupported", raw)
	}
}
