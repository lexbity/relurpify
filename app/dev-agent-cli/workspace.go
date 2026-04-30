package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
	"gopkg.in/yaml.v3"
)

var probeWorkspaceFn = func(cfg ayenitd.WorkspaceConfig) []ayenitd.ProbeResult {
	return ayenitd.ProbeWorkspace(cfg, nil)
}

type inspectionTarget struct {
	workspace    string
	agentName    string
	manifestPath string
	spec         *core.AgentRuntimeSpec
	cfg          ayenitd.WorkspaceConfig
}

func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Inspect workspace configuration and services",
	}
	cmd.AddCommand(newWorkspaceProbeCmd(), newWorkspaceStatusCmd(), newWorkspaceServicesCmd(), newWorkspaceInitCmd())
	return cmd
}

func newWorkspaceProbeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "probe",
		Short: "Run workspace platform checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			cfg := buildProbeWorkspaceConfig(ws)
			results := probeWorkspaceFn(cfg)
			failedRequired := false
			for _, result := range results {
				status := "FAIL"
				if result.OK {
					status = "OK"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-4s %s\n", result.Name, status, result.Message)
				if result.Required && !result.OK {
					failedRequired = true
				}
			}
			if failedRequired {
				return fmt.Errorf("one or more required workspace checks failed")
			}
			return nil
		},
	}
}

func newWorkspaceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show resolved workspace configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := buildInspectionTarget(ensureWorkspace())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "workspace: %s\n", target.workspace)
			fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", target.agentName)
			fmt.Fprintf(cmd.OutOrStdout(), "manifest: %s\n", target.manifestPath)
			fmt.Fprintf(cmd.OutOrStdout(), "model: %s\n", target.cfg.InferenceModel)
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\n", target.cfg.ConfigPath)
			fmt.Fprintf(cmd.OutOrStdout(), "log: %s\n", target.cfg.LogPath)
			fmt.Fprintf(cmd.OutOrStdout(), "events: %s\n", target.cfg.EventsPath)
			fmt.Fprintf(cmd.OutOrStdout(), "telemetry: %s\n", target.cfg.TelemetryPath)
			fmt.Fprintf(cmd.OutOrStdout(), "skip_ast_index: %v\n", target.cfg.SkipASTIndex)
			return nil
		},
	}
}

func newWorkspaceServicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "services",
		Short: "List workspace services",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := openWorkspaceForInspection(cmd.Context(), ensureWorkspace())
			if err != nil {
				return err
			}
			defer func() { _ = ws.Close() }()
			if ws.ServiceManager != nil {
				if err := ws.ServiceManager.StartAll(cmd.Context()); err != nil {
					return err
				}
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

func newWorkspaceInitCmd() *cobra.Command {
	var modelName string
	var agentName string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default relurpify.yaml workspace config",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			if strings.TrimSpace(modelName) == "" {
				modelName = defaultModelName()
			}
			if strings.TrimSpace(agentName) == "" {
				agentName = "coding"
			}
			path := filepath.Join(frameworkmanifest.New(ws).ConfigRoot(), "relurpify.yaml")
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Workspace config already exists at %s\n", path)
				return nil
			}
			cfg := workspaceInitConfig{
				Version:      "1.0.0",
				DefaultModel: frameworkmanifest.ModelRef{Name: modelName, Provider: "ollama"},
				AgentPaths:   frameworkmanifest.DefaultAgentPaths(ws),
				Model:        modelName,
				Agent:        agentName,
				Agents:       []string{agentName},
				Permissions: map[string]string{
					"file_write":  "ask",
					"file_edit":   "ask",
					"file_delete": "deny",
				},
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created workspace config at %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&modelName, "model", "", "Default model name")
	cmd.Flags().StringVar(&agentName, "agent", "", "Default agent name")
	return cmd
}

type workspaceInitConfig struct {
	Version      string                     `yaml:"version"`
	DefaultModel frameworkmanifest.ModelRef `yaml:"default_model"`
	AgentPaths   []string                   `yaml:"agent_paths"`
	Model        string                     `yaml:"model,omitempty"`
	Agent        string                     `yaml:"agent,omitempty"`
	Agents       []string                   `yaml:"agents,omitempty"`
	Permissions  map[string]string          `yaml:"permissions,omitempty"`
}

func buildProbeWorkspaceConfig(ws string) ayenitd.WorkspaceConfig {
	modelName := defaultModelName()
	if globalCfg != nil && globalCfg.DefaultModel.Name != "" {
		modelName = globalCfg.DefaultModel.Name
	}
	return ayenitd.WorkspaceConfig{
		Workspace:         ws,
		InferenceProvider: "ollama",
		InferenceEndpoint: defaultEndpoint(),
		InferenceModel:    modelName,
		ConfigPath:        cfgFile,
		SkipASTIndex:      true,
	}
}

func buildInspectionTarget(ws string) (*inspectionTarget, error) {
	reg, err := buildRegistry(ws)
	if err != nil {
		return nil, err
	}
	agentName := selectDefaultAgent(reg)
	manifest, ok := reg.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("agent %s not found", agentName)
	}
	spec := manifest.Spec.Agent
	if spec == nil {
		return nil, fmt.Errorf("agent %s missing spec.agent section", manifest.Metadata.Name)
	}
	spec = frameworkmanifest.ApplyManifestDefaultsForAgent(manifest.Metadata.Name, spec, manifest.Spec.Defaults)
	spec = frameworkmanifest.ResolveAgentSpec(globalCfg, spec)
	runtimeCfg := appruntime.DefaultConfig()
	runtimeCfg.Workspace = ws
	runtimeCfg.ManifestPath = manifest.SourcePath
	runtimeCfg.AgentName = agentName
	if err := runtimeCfg.Normalize(); err != nil {
		return nil, err
	}
	modelName := spec.Model.Name
	if modelName == "" {
		modelName = defaultModelName()
	}
	cfg := ayenitd.WorkspaceConfig{
		Workspace:         runtimeCfg.Workspace,
		ManifestPath:      runtimeCfg.ManifestPath,
		InferenceProvider: "ollama",
		InferenceEndpoint: defaultEndpoint(),
		InferenceModel:    modelName,
		ConfigPath:        runtimeCfg.ConfigPath,
		AgentsDir:         runtimeCfg.AgentsDir,
		AgentName:         agentName,
		SandboxBackend:    sandboxBackend,
		LogPath:           frameworkmanifest.New(ws).LogFile("ayenitd.log"),
		MemoryPath:        runtimeCfg.MemoryPath,
		SkipASTIndex:      true,
		HITLTimeout:       runtimeCfg.HITLTimeout,
		AuditLimit:        runtimeCfg.AuditLimit,
		Sandbox:           runtimeCfg.Sandbox,
	}
	return &inspectionTarget{
		workspace:    ws,
		agentName:    agentName,
		manifestPath: manifest.SourcePath,
		spec:         spec,
		cfg:          cfg,
	}, nil
}

func openWorkspaceForInspection(ctx context.Context, ws string) (*ayenitd.Workspace, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := buildInspectionTarget(ws)
	if err != nil {
		return nil, err
	}
	return openWorkspaceFn(ctx, target.cfg)
}
