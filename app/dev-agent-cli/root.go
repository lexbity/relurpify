package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
)

var (
	cfgFile        string
	workspace      string
	sandboxBackend string

	globalCfg *frameworkmanifest.GlobalConfig
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
				cfgFile = frameworkmanifest.DefaultConfigPath(workspace)
				altCfg := filepath.Join(frameworkmanifest.New(workspace).ConfigRoot(), "relurpify.yaml")
				if _, err := os.Stat(cfgFile); errors.Is(err, os.ErrNotExist) {
					if _, altErr := os.Stat(altCfg); altErr == nil {
						cfgFile = altCfg
					}
				}
			}
			cfg, err := frameworkmanifest.LoadGlobalConfig(cfgFile, workspace)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			globalCfg = cfg
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
	)
	return root
}
