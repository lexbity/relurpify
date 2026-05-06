package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage workspace services",
	}
	cmd.AddCommand(newServiceListCmd(), newServiceRestartCmd())
	return cmd
}

func newServiceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered service IDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := openWorkspaceForInspection(cmd.Context(), ensureWorkspace())
			if err != nil {
				return err
			}
			defer func() { _ = ws.Close() }()
			if err := startPreparedRunServices(cmd.Context(), ws); err != nil {
				return err
			}
			ids := ws.ListServices()
			sort.Strings(ids)
			if len(ids) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No services registered.")
				return nil
			}
			for _, id := range ids {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", id)
			}
			return nil
		},
	}
}

func newServiceRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [service-id]",
		Short: "Restart a specific service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := openWorkspaceForInspection(cmd.Context(), ensureWorkspace())
			if err != nil {
				return err
			}
			defer func() { _ = ws.Close() }()
			if err := startPreparedRunServices(cmd.Context(), ws); err != nil {
				return err
			}
			if err := restartPreparedRunService(cmd.Context(), ws, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Restarted service %s\n", args[0])
			return nil
		},
	}
}
