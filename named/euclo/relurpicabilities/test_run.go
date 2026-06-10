package relurpicabilities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/capability/classification"
)

// TestRunHandler implements the test runner capability as a shell tool.
// It executes test commands and parses output to determine pass/fail status.
type TestRunHandler struct {
	cmd CommandDeps
}

// NewTestRunHandler creates a new test run handler.
func NewTestRunHandler(cmd CommandDeps) *TestRunHandler {
	return &TestRunHandler{cmd: cmd}
}

// Descriptor returns the capability descriptor for the test run handler.
func (h *TestRunHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.test_run",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Test Runner",
		Version:       "1.0.0",
		Description:   "Runs test suites and parses results to determine pass/fail status",
		Category:      "testing",
		Tags:          []string{"testing", "shell", "tool"},
		Source: descriptor.CapabilitySource{
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"command": {
					Type:        "string",
					Description: "Test command to execute (e.g., 'go test ./...')",
				},
				"workdir": {
					Type:        "string",
					Description: "Working directory for command execution",
				},
				"timeout": {
					Type:        "integer",
					Description: "Timeout in seconds (default: 300)",
				},
			},
			Required: []string{"command"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if command executed successfully",
				},
				"passed": {
					Type:        "boolean",
					Description: "True if all tests passed",
				},
				"exit_code": {
					Type:        "integer",
					Description: "Process exit code",
				},
				"stdout": {
					Type:        "string",
					Description: "Command stdout output",
				},
				"stderr": {
					Type:        "string",
					Description: "Command stderr output",
				},
				"failed_tests": {
					Type:        "array",
					Description: "List of failed test names",
					Items: &schemacoerce.Schema{
						Type: "string",
					},
				},
			},
		},
	}
}

// Invoke executes the test command and returns parsed results.
func (h *TestRunHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	// Extract arguments
	command, ok := stringArg(args, "command")
	if !ok {
		return failResult("command argument is required"), nil
	}

	workdir, _ := stringArg(args, "workdir")
	if workdir != "" {
		resolved := resolveCandidatePath(workdir, h.cmd.Workspace)
		if resolved == "" {
			return failResult(fmt.Sprintf("workdir resolution failed: %s", workdir)), nil
		}
		workdir = resolved
	}
	timeoutSec, _ := intArg(args, "timeout", 300)

	req := sandbox.CommandRequest{
		Workdir: workdir,
		Args:    strings.Fields(command),
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	if h.cmd.Policy != nil {
		if err := h.cmd.Policy.AllowCommand(ctx, req); err != nil {
			return failResult(fmt.Sprintf("test command denied: %v", err)), err
		}
	}

	if h.cmd.Runner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	res, err := h.cmd.Runner.Run(ctx, req)
	if err != nil {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]interface{}{
				"success":      false,
				"passed":       false,
				"exit_code":    -1,
				"stdout":       "",
				"stderr":       "",
				"error":        err.Error(),
				"failed_tests": []string{},
			},
		}, nil
	}

	// Parse test output to determine pass/fail
	passed := true
	failedTests := parseFailedTests(res.Stdout, res.Stderr)

	// If there are failed tests or stderr contains failure indicators, mark as failed
	if len(failedTests) > 0 {
		passed = false
	}
	if strings.Contains(res.Stderr, "FAIL") || strings.Contains(res.Stdout, "FAIL") {
		passed = false
	}

	return &ports.ToolResult{
		Success: res.ExitCode == 0,
		Data: map[string]interface{}{
			"success":      res.ExitCode == 0,
			"passed":       passed,
			"exit_code":    res.ExitCode,
			"stdout":       truncate(res.Stdout, 10000),
			"stderr":       truncate(res.Stderr, 10000),
			"failed_tests": failedTests,
		},
	}, nil
}
