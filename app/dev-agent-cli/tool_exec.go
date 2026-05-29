package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	sandbox2 "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/services"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// buildToolExecRunner is overridable in tests. It constructs a verified,
// sandbox-backed command runner for the tool-exec CLI.
var buildToolExecRunner = buildToolExecRunnerImpl

func buildToolExecRunnerImpl(ctx context.Context, ws, backend string) (sandbox2.CommandRunner, error) {
	sandboxCfg := sandbox2.SandboxConfig{}
	sboxRuntime, err := fauthorization.SelectSandboxRuntime(backend, sandboxCfg, "", ws)
	if err != nil {
		return nil, fmt.Errorf("select sandbox runtime: %w", err)
	}
	policy := fauthorization.BuildSandboxPolicy(nil, nil)
	return sandbox2.NewVerifiedCommandRunner(ctx, sboxRuntime, policy, &contracts.CommandRunnerConfig{
		Workspace: ws,
	})
}

func newToolExecCmd() *cobra.Command {
	var argsJSON string

	cmd := &cobra.Command{
		Use:   "tool-exec <tool-name>",
		Short: "Execute a registered tool with JSON arguments and print the result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolName := args[0]

			var toolArgs map[string]any
			if argsJSON != "" {
				if err := json.Unmarshal([]byte(argsJSON), &toolArgs); err != nil {
					return fmt.Errorf("parse --args JSON: %w", err)
				}
			}

			ws := ensureWorkspace()
			sboxRunner, err := buildToolExecRunner(cmd.Context(), ws, sandboxBackend)
			if err != nil {
				return fmt.Errorf("tool-exec requires a verified sandbox (gvisor/docker); none available: %w", err)
			}

			capReg, err := services.BuildMinimalToolRegistry(ws, sboxRunner)
			if err != nil {
				return fmt.Errorf("build tool registry: %w", err)
			}

			result, err := capReg.InvokeCapability(
				cmd.Context(),
				contextdata.NewEnvelope("tool-exec", ""),
				toolName,
				toolArgs,
			)
			if err != nil {
				return fmt.Errorf("invoke %s: %w", toolName, err)
			}

			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))

			if !result.Success {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&argsJSON, "args", "", "JSON-encoded tool arguments")
	return cmd
}
