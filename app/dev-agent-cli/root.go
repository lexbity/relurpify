package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/runtimeenv"
)

var (
	cfgFile        string
	workspace      string
	sandboxBackend string

	workspaceCfg *cfgload.WorkspaceConfig
	envSnapshot  []string
	envOverrides cfgload.EnvOverrides
	secrets      cfgload.Secrets
	sharedRoot   string
)

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
		Short:         "Development and integration CLI for Relurpify",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				if wd, err := os.Getwd(); err == nil {
					workspace = wd
				} else {
					return err
				}
			}
			if cfgFile == "" {
				cfgFile = cfgload.DefaultWorkspaceConfigPath(workspace)
			}
			cfg, err := cfgload.LoadWorkspaceConfig(cfgFile, workspace, cfgload.WorkspaceLoadOptions{})
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			workspaceCfg = cfg
			envSnapshot = runtimeenv.Capture()
			var ovErr error
			envOverrides, ovErr = cfgload.LoadEnvOverrides(envSnapshot)
			if ovErr != nil {
				return ovErr
			}
			secrets = cfgload.LoadSecrets(envSnapshot)
			sharedRoot = cfgload.ResolveSharedRoot(envOverrides.XDGDataHome)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&workspace, "workspace", "", "Workspace directory")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to development CLI config file")
	root.PersistentFlags().StringVar(&sandboxBackend, "sandbox-backend", "", "Sandbox backend to use (gvisor or docker)")

	root.AddCommand(
		newStartCmd(),
		newWorkspaceCmd(),
		newServiceCmd(),
		newAgentsCmd(),
		newSkillCmd(),
		newConfigCmd(),
		newSessionCmd(),
		newAgentTestCmd(),
		newToolExecCmd(),
	)
	return root
}
