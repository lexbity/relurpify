package main

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/configcheck"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate the workspace config tree and full load pipeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Validating config for workspace: %s\n", cfg.Workspace)
			report := configcheck.ValidateWorkspaceTree(cfg.Workspace)
			if report.HasErrors() {
				fmt.Fprintln(cmd.ErrOrStderr(), report.Error())
				return report
			}
			loaded, _, err := cfgload.Load(cfgload.LoadOptions{
				WorkspaceRoot: cfg.Workspace,
				EnvOverrides:  append([]string(nil), cfg.EnvOverrides...),
			})
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ workspace.yaml\n")
			fmt.Fprintf(cmd.OutOrStdout(), "✓ security policies\n")
			fmt.Fprintf(cmd.OutOrStdout(), "✓ model/providers (%d)\n", len(loaded.Model.Providers))
			fmt.Fprintf(cmd.OutOrStdout(), "✓ model/profiles (%d)\n", len(loaded.Model.Profiles))
			fmt.Fprintf(cmd.OutOrStdout(), "✓ tools/ (%d loaded)\n", len(loaded.Tools.ListTools()))
			fmt.Fprintf(cmd.OutOrStdout(), "✓ agents (%d loaded)\n", len(loaded.Agents.Names()))
			fmt.Fprintf(cmd.OutOrStdout(), "config fingerprint: %s\n", loaded.Fingerprint)
			fmt.Fprintln(cmd.OutOrStdout(), "Config valid.")
			return nil
		},
	}
}
