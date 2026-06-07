package agenttest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
	"codeburg.org/lexbit/relurpify/userconfig/config"
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

func buildVerifyToolIndex(workspace string, runner sandbox.CommandRunner) map[string]ports.Tool {
	manifestDir := config.DefaultToolManifestDir(workspace)
	manifests, err := config.LoadToolManifests(manifestDir)
	if err != nil {
		return nil
	}
	tools := toolcapabilities.Build(workspace, commandRunnerAdapter{runner: runner}, manifests)
	index := make(map[string]ports.Tool, len(tools))
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

	spec := subprocess.RunSpec{
		Command: []string{"bash", absScript},
		Workdir: workspace,
	}
	runResult, err := subprocess.Run(ctx, commandRunnerAdapter{runner: runner}, spec)
	passed := err == nil && runResult != nil && runResult.Success

	var msg string
	if runResult != nil {
		toolResult := &ports.ToolResult{
			Success: runResult.Success,
			Error:   runResult.Error,
			Data: map[string]interface{}{
				"stdout": runResult.Stdout,
				"stderr": runResult.Stderr,
			},
		}
		msg = extractVerifyMessage(toolResult, err)
	} else {
		msg = extractVerifyMessage(nil, err)
	}

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

func extractVerifyMessage(result *ports.ToolResult, err error) string {
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
