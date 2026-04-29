package agenttest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/shell"
	"codeburg.org/lexbit/relurpify/platform/shell/command"
)

// VerifyStepResult captures the outcome of one verification step.
type VerifyStepResult struct {
	StepIndex int
	ToolName  string
	Success   bool
	Stdout    string
	Stderr    string
	Summary   string
}

func buildVerifyToolIndex(workspace string, runner sandbox.CommandRunner) map[string]core.Tool {
	tools := shell.CommandLineTools(workspace, commandRunnerAdapter{runner: runner})
	index := make(map[string]core.Tool, len(tools))
	for _, tool := range tools {
		index[tool.Name()] = tool
	}
	return index
}

func runVerificationSteps(ctx context.Context, spec VerifySpec, workspace string, runner sandbox.CommandRunner) []AssertionResult {
	index := buildVerifyToolIndex(workspace, runner)
	var results []AssertionResult

	for i, step := range spec.Steps {
		tool, ok := index[step.Tool]
		if !ok {
			results = append(results, AssertionResult{
				AssertionID: fmt.Sprintf("verify.step[%d].%s", i, step.Tool),
				Tier:        "outcome",
				Passed:      false,
				Message:     fmt.Sprintf("verification tool %q not found in registry", step.Tool),
			})
			break
		}

		toolResult, err := tool.Execute(ctx, normalizeVerifyArgs(step.Args))
		passed := err == nil && toolResult != nil && toolResult.Success
		msg := extractVerifyMessage(toolResult, err)

		results = append(results, AssertionResult{
			AssertionID: fmt.Sprintf("verify.step[%d].%s", i, step.Tool),
			Tier:        "outcome",
			Passed:      passed,
			Message:     msg,
		})

		if !passed && !step.ContinueOnFailure {
			break
		}
	}

	if spec.Script != "" {
		results = append(results, runVerifyScript(ctx, spec.Script, workspace, runner))
	}

	return results
}

func runVerifyScript(ctx context.Context, scriptPath, workspace string, runner sandbox.CommandRunner) AssertionResult {
	absScript := scriptPath
	if !filepath.IsAbs(scriptPath) {
		absScript = filepath.Join(workspace, scriptPath)
	}

	scriptTool := command.NewCommandTool(workspace, command.CommandToolConfig{
		Name:     "verify_script",
		Command:  "bash",
		Category: "verify",
		Timeout:  120 * time.Second,
	})
	scriptTool.SetCommandRunner(commandRunnerAdapter{runner: runner})
	result, err := scriptTool.Execute(ctx, map[string]interface{}{
		"args":              []interface{}{absScript},
		"working_directory": workspace,
	})
	passed := err == nil && result != nil && result.Success
	msg := extractVerifyMessage(result, err)
	return AssertionResult{
		AssertionID: fmt.Sprintf("verify.script[%s]", filepath.Base(scriptPath)),
		Tier:        "outcome",
		Passed:      passed,
		Message:     msg,
	}
}

func normalizeVerifyArgs(args map[string]any) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}

func extractVerifyMessage(result *core.ToolResult, err error) string {
	var parts []string
	if err != nil {
		parts = append(parts, err.Error())
	}
	if result == nil {
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	for _, key := range []string{"summary", "first_failure", "stdout", "stderr"} {
		if value := strings.TrimSpace(fmt.Sprint(result.Data[key])); value != "" && value != "<nil>" {
			parts = append(parts, value)
		}
	}
	if strings.TrimSpace(result.Error) != "" {
		parts = append(parts, strings.TrimSpace(result.Error))
	}
	return strings.TrimSpace(strings.Join(dedupeNonEmptyStrings(parts), "\n"))
}

func dedupeNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
