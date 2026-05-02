package relurpicabilities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TestRunHandler implements the test runner capability as a shell tool.
// It executes test commands and parses output to determine pass/fail status.
type TestRunHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

// NewTestRunHandler creates a new test run handler.
func NewTestRunHandler(env agentenv.WorkspaceEnvironment) *TestRunHandler {
	return &TestRunHandler{env: env}
}

// Descriptor returns the capability descriptor for the test run handler.
func (h *TestRunHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.test_run",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Test Runner",
		Version:       "1.0.0",
		Description:   "Runs test suites and parses results to determine pass/fail status",
		Category:      "testing",
		Tags:          []string{"testing", "shell", "tool"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassExecute},
		EffectClasses: []core.EffectClass{core.EffectClassProcessSpawn},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
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
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
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
					Items: &core.Schema{
						Type: "string",
					},
				},
			},
		},
	}
}

// Invoke executes the test command and returns parsed results.
func (h *TestRunHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	// Extract arguments
	command, ok := stringArg(args, "command")
	if !ok {
		return failResult("command argument is required"), nil
	}

	workdir, _ := stringArg(args, "workdir")
	if workdir != "" {
		resolvedWorkdir, err := h.resolveWorkspacePath(h.env, workdir)
		if err != nil {
			return failResult(fmt.Sprintf("workdir resolution failed: %v", err)), err
		}
		workdir = resolvedWorkdir
	}
	timeoutSec, _ := intArg(args, "timeout", 300)

	// Check for CommandRunner
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	if err := h.authorizeCommand(ctx, h.env, sandbox.CommandRequest{
		Workdir: workdir,
		Args:    strings.Fields(command),
		Timeout: time.Duration(timeoutSec) * time.Second,
	}, "euclo test run"); err != nil {
		return failResult(fmt.Sprintf("test command denied: %v", err)), err
	}

	// Build command request
	req := sandbox.CommandRequest{
		Workdir: workdir,
		Args:    strings.Fields(command),
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	// Execute command
	stdout, stderr, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil {
		// Command execution failed (e.g., timeout, permission denied)
		return &contracts.CapabilityExecutionResult{
			Success: false,
			Data: map[string]interface{}{
				"success":      false,
				"passed":       false,
				"exit_code":    -1,
				"stdout":       truncate(stdout, 10000),
				"stderr":       truncate(stderr, 10000),
				"error":        err.Error(),
				"failed_tests": []string{},
			},
		}, nil
	}

	// Parse test output to determine pass/fail
	passed := true
	failedTests := parseFailedTests(stdout, stderr)

	// If there are failed tests or stderr contains failure indicators, mark as failed
	if len(failedTests) > 0 {
		passed = false
	}
	if strings.Contains(stderr, "FAIL") || strings.Contains(stdout, "FAIL") {
		passed = false
	}

	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":      true,
			"passed":       passed,
			"exit_code":    0, // We got here, so command didn't error
			"stdout":       truncate(stdout, 10000),
			"stderr":       truncate(stderr, 10000),
			"failed_tests": failedTests,
		},
	}, nil
}
