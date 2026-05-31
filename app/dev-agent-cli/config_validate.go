package main

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"github.com/spf13/cobra"
)

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the workspace config tree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := validateConfigTree(ensureWorkspace())
			if !report.HasErrors() {
				fmt.Fprintln(cmd.OutOrStdout(), "config valid")
				return nil
			}
			fmt.Fprintln(cmd.ErrOrStderr(), report.Error())
			return report
		},
	}
}

func validateConfigTree(workspace string) *cfgload.ValidationReport {
	return cfgload.ValidateWorkspaceTree(workspace)
}
