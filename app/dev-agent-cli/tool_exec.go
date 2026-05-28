package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	sandbox2 "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformshell "codeburg.org/lexbit/relurpify/platform/shell"
)

// sandboxRunnerAdapter adapts a sandbox.CommandRunner to contracts.CommandRunner.
type sandboxRunnerAdapter struct {
	inner sandbox2.CommandRunner
}

func (a sandboxRunnerAdapter) Run(ctx context.Context, req contracts.CommandRequest) (string, string, error) {
	if a.inner == nil {
		return "", "", nil
	}
	return a.inner.Run(ctx, sandbox2.CommandRequest{
		Workdir:        req.Workdir,
		Args:           req.Args,
		Env:            req.Env,
		Input:          req.Input,
		Timeout:        req.Timeout,
		MaxOutputBytes: req.MaxOutputBytes,
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
			sboxRunner := sandbox2.NewLocalCommandRunner(ws, nil, nil)
			runner := sandboxRunnerAdapter{inner: sboxRunner}
			tools := platformshell.CommandLineTools(ws, runner, nil)
			capReg := capability.NewRegistry()

			for _, t := range tools {
				if err := capReg.Register(t); err != nil {
					return fmt.Errorf("register tool %s: %w", t.Name(), err)
				}
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
