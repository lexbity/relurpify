package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var workspace string //nolint:gochecknoglobals // persistent flag target bound via StringVar at the CLI entrypoint

// Execute is the entry point for the CLI.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewRootCmd wires the cobra tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "dev-agent",
		Short:         "Development CLI for Relurpify",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	// A non-empty Version makes cobra register --version; the template renders
	// the link-time build metadata. The name is taken from the command itself
	// so renaming Use renames the version output.
	root.SetVersionTemplate(fmt.Sprintf("%s %s (commit %s, built %s)\n", root.Name(), version, commit, date))
	root.PersistentFlags().StringVar(&workspace, "workspace", "", "Workspace directory")
	root.AddCommand(newAgentTestCmd())
	return root
}
