package main

import (
	"fmt"
	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
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
			path := filepath.Join(agents.ConfigDir(ws), "agents")
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			file := filepath.Join(path, fmt.Sprintf("%s.yaml", sanitizeName(name)))
			if _, err := os.Stat(file); err == nil {
				return fmt.Errorf("manifest %s already exists", file)
			}
			wsGlob := filepath.ToSlash(filepath.Join(ws, "**"))
			defaultToolCalling := true
			manifest := manifest.AgentManifest{
				APIVersion: "relurpify/v1alpha1",
				Kind:       "AgentManifest",
				Metadata: manifest.ManifestMetadata{
					Name:        name,
					Version:     "1.0.0",
					Description: description,
				},
				Spec: manifest.ManifestSpec{
					Image:   "ghcr.io/relurpify/runtime:latest",
					Runtime: "gvisor",
					Defaults: &manifest.ManifestDefaults{
						Permissions: &core.PermissionSet{
							FileSystem: []core.FileSystemPermission{
								{Action: core.FileSystemRead, Path: wsGlob, Justification: "Read workspace"},
								{Action: core.FileSystemList, Path: wsGlob, Justification: "List workspace"},
								{Action: core.FileSystemWrite, Path: wsGlob, Justification: "Modify workspace"},
								{Action: core.FileSystemExecute, Path: wsGlob, Justification: "Execute tooling inside workspace"},
							},
							Executables: []core.ExecutablePermission{
								{Binary: "bash", Args: []string{"-c", "*"}},
								{Binary: "go", Args: []string{"*"}},
							},
							Network: []core.NetworkPermission{
								{Direction: "egress", Protocol: "tcp", Host: "localhost", Port: 11434, Description: "Ollama"},
							},
						},
						Resources: &manifest.ResourceSpec{
							Limits: manifest.ResourceLimit{
								CPU:    "2",
								Memory: "4Gi",
								DiskIO: "500MBps",
							},
						},
					},
					Security: manifest.SecuritySpec{
						RunAsUser:       1000,
						ReadOnlyRoot:    false,
						NoNewPrivileges: true,
					},
					Audit: manifest.AuditSpec{
						Level:         "verbose",
						RetentionDays: 7,
					},
					Agent: &core.AgentRuntimeSpec{
						Mode:              core.AgentMode(kind),
						Version:           "1.0.0",
						Prompt:            defaultManifestPrompt(name),
						OllamaToolCalling: &defaultToolCalling,
						Model: core.AgentModelConfig{
							Provider:    "ollama",
							Name:        model,
							Temperature: 0.2,
							MaxTokens:   4096,
						},
						AllowedCapabilities: []core.CapabilitySelector{
							{Name: "file_read", Kind: core.CapabilityKindTool},
							{Name: "file_write", Kind: core.CapabilityKindTool},
							{Name: "file_edit", Kind: core.CapabilityKindTool},
							{Name: "file_list", Kind: core.CapabilityKindTool},
							{Name: "file_search", Kind: core.CapabilityKindTool},
							{Name: "file_create", Kind: core.CapabilityKindTool},
							{Name: "search_find_similar", Kind: core.CapabilityKindTool},
							{Name: "search_semantic", Kind: core.CapabilityKindTool},
							{Name: "query_ast", Kind: core.CapabilityKindTool},
						},
						Bash: core.AgentBashPermissions{
							Default:       core.AgentPermissionAsk,
							AllowPatterns: []string{"git diff*", "git status"},
							DenyPatterns:  []string{"rm -rf*", "sudo*"},
						},
						Files: core.AgentFileMatrix{
							Write: core.AgentFilePermissionSet{AllowPatterns: []string{"**/*.go", "docs/**/*.md"}, Default: core.AgentPermissionAsk},
							Edit:  core.AgentFilePermissionSet{Default: core.AgentPermissionAsk, RequireApproval: true},
						},
						Invocation: core.AgentInvocationSpec{
							CanInvokeSubagents: true,
							MaxDepth:           2,
						},
						Context: core.AgentContextSpec{
							MaxFiles:            20,
							MaxTokens:           20000,
							IncludeDependencies: true,
						},
						Metadata: core.AgentMetadata{
							Author:   os.Getenv("USER"),
							Tags:     []string{"generated"},
							Priority: 5,
						},
					},
				},
			}
			if err := manifest.Validate(); err != nil {
				return err
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
	cmd.Flags().StringVar(&kind, "kind", string(core.AgentModePrimary), "Agent kind (primary|subagent|system)")
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
			if err := manifest.Validate(); err != nil {
				return err
			}
			modelName := ""
			if manifest.Spec.Agent != nil {
				modelName = manifest.Spec.Agent.Model.Name
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Manifest %s valid (model=%s)\n", manifest.Metadata.Name, modelName)
			return nil
		},
	}
}

// defaultManifestPrompt returns a short instruction block for generated agents.
func defaultManifestPrompt(name string) string {
	return fmt.Sprintf(`You are %s. Follow project rules, ask before destructive actions, and summarize each change.`, strings.Title(name))
}
