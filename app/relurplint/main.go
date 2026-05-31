package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	checkFlag     string
	workspaceFlag string
	formatFlag    string
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(2)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "relurplint",
		Short: "Validate relurpify workspace configuration, tools, recipes, and prompts",
		RunE:  runLint,
	}
	root.Flags().StringVar(&checkFlag, "check", "all", "Checks to run: all, config, tools, recipes, prompts, or comma-separated")
	root.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace directory (default: current directory)")
	root.Flags().StringVar(&formatFlag, "format", "text", "Output format: text or json")
	return root
}

func runLint(cmd *cobra.Command, args []string) error {
	diags, err := collectDiagnostics(checkFlag, workspaceFlag)
	if err != nil {
		return err
	}
	Render(diags, formatFlag, cmd.OutOrStdout())
	code := ExitCode(diags)
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

func collectDiagnostics(check, workspace string) ([]Diagnostic, error) {
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve workspace: %w", err)
		}
	}
	ws, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	checks, err := selected(check)
	if err != nil {
		return nil, err
	}
	var all []Diagnostic
	for _, c := range checks {
		all = append(all, c.Run(ws)...)
	}
	return all, nil
}
