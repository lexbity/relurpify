package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newAgentsCmd wires the `agents` command group.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage agent manifests",
	}
	cmd.AddCommand(newAgentsListCmd(), newAgentsCreateCmd(), newAgentsTestCmd())
	return cmd
}

// newAgentsListCmd lists manifests in the configured registry.
func newAgentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := buildRegistry(ensureWorkspace())
			if err != nil {
				return err
			}
			summaries := reg.List()
			if len(summaries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No agents found.")
				return nil
			}
			for _, summary := range summaries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s) · model=%s · %s\n", summary.Name, summary.Mode, summary.Model, summary.Source)
				if summary.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", summary.Description)
				}
			}
			if errs := reg.Errors(); len(errs) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Manifest load errors:")
				for _, err := range errs {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", err.Path, err.Error)
				}
			}
			return nil
		},
	}
}

// newAgentsCreateCmd scaffolds a manifest using the CLI flags.
func newAgentsCreateCmd() *cobra.Command {
	var name string
	var kind string
	var model string
	var description string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new agent manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			if name == "" {
				return fmt.Errorf("--name required")
			}
			if model == "" {
				model = defaultModelName()
			}
			path := filepath.Join(cfgload.New(ws).ConfigRoot(), "agents")
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			file := filepath.Join(path, fmt.Sprintf("%s.yaml", sanitizeName(name)))
			if _, err := os.Stat(file); err == nil {
				return fmt.Errorf("manifest %s already exists", file)
			}
			manifest := cfgload.AgentManifest{
				APIVersion: "relurpify/v1alpha1",
				Kind:       "AgentManifest",
				Metadata: cfgload.ManifestMetadata{
					Name:        name,
					Version:     "1.0.0",
					Description: description,
				},
				Spec: struct{}{},
			}
			data, err := yaml.Marshal(manifest)
			if err != nil {
				return err
			}
			if err := os.WriteFile(file, data, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", file)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Agent name")
	cmd.Flags().StringVar(&kind, "kind", string(agentspec.AgentModePrimary), "Agent kind (primary|subagent|system)")
	cmd.Flags().StringVar(&model, "model", "", "Model name")
	cmd.Flags().StringVar(&description, "description", "Custom agent", "Description")
	return cmd
}

// newAgentsTestCmd validates a manifest by name and prints the result.
func newAgentsTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [name]",
		Short: "Validate an agent manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			reg, err := buildRegistry(ws)
			if err != nil {
				return err
			}
			name := args[0]
			manifest, ok := reg.Get(name)
			if !ok {
				return fmt.Errorf("agent %s not found", name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Manifest %s loaded\n", manifest.Metadata.Name)
			return nil
		},
	}
}

// defaultManifestPrompt returns a short instruction block for generated agents.
func defaultManifestPrompt(name string) string {
	return fmt.Sprintf(`You are %s. Follow project rules, ask before destructive actions, and summarize each change.`, strings.Title(name))
}

func defaultRelurpicCapabilities(name string) []string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "euclo":
		return []string{
			"euclo:cap.test_run",
			"euclo:cap.ast_query",
			"euclo:cap.symbol_trace",
			"euclo:cap.call_graph",
			"euclo:cap.blame_trace",
			"euclo:cap.bisect",
			"euclo:cap.code_review",
			"euclo:cap.diff_summary",
			"euclo:cap.layer_check",
			"euclo:cap.targeted_refactor",
			"euclo:cap.rename_symbol",
			"euclo:cap.api_compat",
			"euclo:cap.boundary_report",
			"euclo:cap.coverage_check",
		}
	default:
		return nil
	}
}
