package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var workspace string

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
	}
	root.PersistentFlags().StringVar(&workspace, "workspace", "", "Workspace directory")
	root.AddCommand(newAgentTestCmd())
	return root
}
