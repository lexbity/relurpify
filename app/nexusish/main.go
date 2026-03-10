package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	nexusruntime "github.com/lexcodex/relurpify/app/nexusish/runtime"
	"github.com/lexcodex/relurpify/app/nexusish/tui"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var workspace string
	var configPath string
	cmd := &cobra.Command{
		Use:   "nexusish",
		Short: "Terminal dashboard for the Nexus gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return runDashboard(ctx, workspace, configPath)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace directory")
	cmd.Flags().StringVar(&configPath, "config", "", "path to nexus config")
	return cmd
}

func runDashboard(ctx context.Context, workspace, configPath string) error {
	return tui.Run(ctx, nexusruntime.New(workspace, configPath))
}
