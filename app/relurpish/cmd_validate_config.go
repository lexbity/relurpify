package main

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/configcheck"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the workspace config tree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := configcheck.ValidateWorkspaceTree(cfg.Workspace)
			if !report.HasErrors() {
				fmt.Fprintln(cmd.OutOrStdout(), "config valid")
				return nil
			}
			fmt.Fprintln(cmd.ErrOrStderr(), report.Error())
			return report
		},
	}
}
