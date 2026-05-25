package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/configcheck"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
	namedfactory "codeburg.org/lexbit/relurpify/named/factory"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"github.com/spf13/cobra"
)

var (
	registerAgentFn            = fauthorization.RegisterAgent
	registerBuiltinProvidersFn = appruntime.RegisterBuiltinProviders
	instantiateNamedAgentFn    = namedfactory.InstantiateByName
)

// newStartCmd constructs the development CLI command that runs an agent.
func newStartCmd() *cobra.Command {
	var mode string
	var agentName string
	var instruction string
	var dryRun bool
	var autoApprove bool
	var resumeLatestWorkflow bool
	var resumeSession bool
	var workflowID string
	var rerunFromStepID string
	var skipASTIndex bool
	var logPath string
	var eventsLogPath string
	var telemetryPath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a development agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			runCtx := cmd.Context()
			if runCtx == nil {
				runCtx = context.Background()
			}
			ws := ensureWorkspace()
			if report := configcheck.ValidateWorkspaceTree(ws); report.HasErrors() {
				fmt.Fprintln(cmd.ErrOrStderr(), report.Error())
				return report
			}
			reg, err := buildRegistry(ws)
			if err != nil {
				return err
			}
			if agentName == "" {
				agentName = selectDefaultAgent(reg)
			}
			agentManifest, ok := reg.Get(agentName)
			if !ok {
				return fmt.Errorf("agent %s not found", agentName)
			}
			spec := agentManifest.Spec.Agent
			if spec == nil {
				return fmt.Errorf("agent %s missing spec.agent section", agentManifest.Metadata.Name)
			}
			spec = frameworkmanifest.ApplyManifestDefaultsForAgent(agentManifest.Metadata.Name, spec, agentManifest.Spec.Defaults)
			spec = frameworkmanifest.ResolveAgentSpec(spec)
			logLLM := false
			logAgent := false
			if workspaceCfg != nil && workspaceCfg.Logging.Level != nil && *workspaceCfg.Logging.Level == "debug" {
				logLLM = true
				logAgent = true
			}
			if spec.Logging != nil {
				if spec.Logging.LLM != nil {
					logLLM = *spec.Logging.LLM
				}
				if spec.Logging.Agent != nil {
					logAgent = *spec.Logging.Agent
				}
			}
			mode = agentTestSurface.ResolveStartMode(mode, spec)
			if instruction == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Agent %s ready in %s mode. Provide --instruction to execute a task.\n", agentName, mode)
				return nil
			}
			// Prepend resume session trigger phrase if --resume-session flag is set
			if resumeSession && !strings.Contains(strings.ToLower(instruction), "resume session") {
				instruction = "resume session: " + instruction
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %s in %s mode with instruction: %s\n", agentName, mode, instruction)
				return nil
			}
			runtimeCfg := appruntime.DefaultConfig()
			runtimeCfg.Workspace = ws
			runtimeCfg.ManifestPath = agentManifest.SourcePath
			runtimeCfg.AgentName = agentName
			if err := runtimeCfg.Normalize(); err != nil {
				return err
			}
			modelName := spec.Model.Name
			if modelName == "" {
				modelName = defaultModelName()
			}
			secrets := cfgload.LoadSecrets(os.Environ())
			workspace, err := workspaceOpenFn(runCtx, agentenv.WorkspaceConfig{
				Workspace:         runtimeCfg.Workspace,
				ManifestPath:      runtimeCfg.ManifestPath,
				InferenceProvider: "ollama",
				InferenceEndpoint: defaultEndpoint(),
				InferenceModel:    modelName,
				ConfigPath:        runtimeCfg.ConfigPath,
				AgentsDir:         runtimeCfg.AgentsDir,
				AgentName:         agentName,
				SandboxBackend:    sandboxBackend,
				LogPath:           logPath,
			}, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey}, workspaceRegistrationFuncsFn())
			if err != nil {
				return err
			}
			defer func() {
				_ = workspace.Close()
			}()
			if workspace.ServiceManager != nil {
				if err := workspace.ServiceManager.StartAll(runCtx); err != nil {
					return err
				}
			}
			registration := workspace.Registration
			if registration == nil {
				return fmt.Errorf("workspace registration missing")
			}
			if workspace.CompiledPolicy == nil {
				return fmt.Errorf("compiled policy missing from workspace")
			}
			if autoApprove {
				registration.Permissions.SetDefaultPolicy(agentspec.AgentPermissionAllow)
				spec.Bash.Default = agentspec.AgentPermissionAllow
			} else if registration.HITL != nil {
				hitlEvents, unsub := registration.HITL.Subscribe(4)
				defer unsub()
				go func() {
					scanner := bufio.NewScanner(os.Stdin)
					for event := range hitlEvents {
						if event.Type != fauthorization.HITLEventRequested || event.Request == nil {
							continue
						}
						req := event.Request
						fmt.Fprintf(os.Stderr, "\n[HITL] Permission request: %s\n  Action: %s\n  Allow? [y/N]: ",
							req.Justification, req.Permission.Action)
						var response string
						if scanner.Scan() {
							response = strings.TrimSpace(strings.ToLower(scanner.Text()))
						}
						if response == "y" || response == "yes" {
							_ = registration.HITL.Approve(fauthorization.PermissionDecision{
								RequestID:  req.ID,
								ApprovedBy: "cli-user",
								Scope:      fauthorization.GrantScopeSession,
							})
						} else {
							_ = registration.HITL.Deny(req.ID, "denied by user")
						}
					}
				}()
			}
			if workspace.Environment.Config != nil && workspace.Environment.Config.AgentSpec != nil {
				workspace.Environment.Registry.UseAgentSpec(registration.ID, workspace.Environment.Config.AgentSpec)
			}
			spec = workspace.AgentSpec
			if spec == nil {
				return fmt.Errorf("workspace agent spec missing")
			}
			if spec.Logging != nil {
				if spec.Logging.LLM != nil {
					logLLM = *spec.Logging.LLM
				}
				if spec.Logging.Agent != nil {
					logAgent = *spec.Logging.Agent
				}
			}
			cfg := &core.Config{
				Name:              agentName,
				Model:             modelName,
				MaxIterations:     8,
				NativeToolCalling: spec.NativeToolCallingEnabled(),
				AgentSpec:         spec,
				DebugLLM:          logLLM,
				DebugAgent:        logAgent,
			}
			providerRuntime := &appruntime.Runtime{
				Workspace:    workspace,
				Tools:        workspace.Environment.Registry,
				Model:        workspace.Environment.Model,
				IndexManager: workspace.Environment.IndexManager,
				SearchEngine: workspace.Environment.SearchEngine,
				Memory:       workspace.Environment.WorkingMemory,
			}
			if err := registerBuiltinProvidersFn(runCtx, providerRuntime); err != nil {
				return err
			}
			workspace.Environment.Config = cfg
			agent := instantiateNamedAgentFn(runtimeCfg.Workspace, agentName, workspace.Environment)
			if agent == nil {
				return fmt.Errorf("agent %s unavailable", agentName)
			}
			execCtx, stopSignals := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
			defer stopSignals()
			ctx, cancel := context.WithTimeout(execCtx, 10*time.Minute)
			defer cancel()
			startedAt := time.Now()
			task := &core.Task{
				ID:          fmt.Sprintf("cli-%d", time.Now().UnixNano()),
				Instruction: instruction,
				Type:        string(core.TaskTypeCodeGeneration),
				Context:     agentTestSurface.BuildStartTaskContext(mode, ws),
			}
			if resumeLatestWorkflow {
				task.Context["resume_latest_workflow"] = true
			}
			if workflowID != "" {
				task.Context["workflow_id"] = workflowID
			}
			if rerunFromStepID != "" {
				task.Context["rerun_from_step_id"] = rerunFromStepID
			}
			env := contextdata.NewEnvelope(task.ID, "")
			env.WorkingData["task.id"] = task.ID
			env.WorkingData["task.type"] = string(task.Type)
			env.WorkingData["task.instruction"] = task.Instruction
			result, err := agent.Execute(ctx, task, env)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				return encoder.Encode(map[string]any{
					"task_id":   task.ID,
					"mode":      mode,
					"node_id":   result.NodeID,
					"duration":  time.Since(startedAt).String(),
					"workspace": ws,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Agent complete (node=%s): %+v\n", result.NodeID, result.Data)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "default", "Execution mode hint passed to the active surface adapter")
	cmd.Flags().StringVar(&agentName, "agent", "", "Agent name from manifest registry")
	cmd.Flags().StringVar(&instruction, "instruction", "", "Instruction to execute")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without executing")
	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Auto-approve all HITL permission requests (skips interactive prompts)")
	cmd.Flags().BoolVar(&resumeLatestWorkflow, "resume-latest-workflow", false, "Resume the latest persisted architect workflow instead of starting from scratch")
	cmd.Flags().BoolVar(&resumeSession, "resume-session", false, "Trigger session resume flow at start")
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Resume or continue the specified workflow ID")
	cmd.Flags().StringVar(&rerunFromStepID, "rerun-from-step", "", "Replay a workflow from the specified step ID")
	cmd.Flags().BoolVar(&skipASTIndex, "skip-ast-index", true, "Default true for CLI startup: skip AST/bootstrap indexing during setup; use --skip-ast-index=false for dedicated AST-enabled end-to-end runs")
	cmd.Flags().StringVar(&logPath, "log", "", "Override workspace log file path")
	cmd.Flags().StringVar(&eventsLogPath, "events-log", "", "Optional SQLite event log path")
	cmd.Flags().StringVar(&telemetryPath, "telemetry", "", "Optional JSON telemetry file path")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a machine-readable JSON execution summary")
	return cmd
}

// selectDefaultAgent picks the first registry entry so users can run commands
// without specifying --agent.
func selectDefaultAgent(reg *agentRegistry) string {
	if _, ok := reg.Get("testfu"); ok {
		return "testfu"
	}
	list := reg.List()
	if len(list) == 0 {
		return "coding"
	}
	return list[0].Name
}

// defaultModelName returns the preferred model from config or falls back to a
// safe local default.
func defaultModelName() string {
	if workspaceCfg != nil && workspaceCfg.Model.DefaultName != nil && *workspaceCfg.Model.DefaultName != "" {
		return *workspaceCfg.Model.DefaultName
	}
	return "codellama:13b"
}

// defaultEndpoint resolves the Ollama endpoint, honoring overrides from env.
func defaultEndpoint() string {
	if val := cfgload.LoadEnvOverrides(os.Environ()).OllamaHost; val != "" {
		return val
	}
	return "http://localhost:11434"
}
