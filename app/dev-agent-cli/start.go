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

	"codeburg.org/lexbit/relurpify/agents"
	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	"codeburg.org/lexbit/relurpify/ayenitd"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/config"
	contractpkg "codeburg.org/lexbit/relurpify/framework/contract"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"github.com/spf13/cobra"
)

var (
	registerAgentFn                       = fauthorization.RegisterAgent
	openWorkspaceFn                       = ayenitd.Open
	registerBuiltinProvidersFn            = appruntime.RegisterBuiltinProviders
	registerBuiltinRelurpicCapabilitiesFn = agents.RegisterBuiltinRelurpicCapabilitiesWithOptions
	registerAgentCapabilitiesFn           = agents.RegisterAgentCapabilities
	buildFromSpecFn                       = agents.BuildFromSpec
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
			reg, err := buildRegistry(ws)
			if err != nil {
				return err
			}
			if agentName == "" {
				agentName = selectDefaultAgent(reg)
			}
			manifest, ok := reg.Get(agentName)
			if !ok {
				return fmt.Errorf("agent %s not found", agentName)
			}
			spec := manifest.Spec.Agent
			if spec == nil {
				return fmt.Errorf("agent %s missing spec.agent section", manifest.Metadata.Name)
			}
			spec = contractpkg.ApplyManifestDefaultsForAgent(manifest.Metadata.Name, spec, manifest.Spec.Defaults)
			spec = contractpkg.ResolveAgentSpec(globalCfg, spec)
			modeFlagChanged := cmd.Flags().Changed("mode")
			logLLM := false
			logAgent := false
			if globalCfg != nil {
				logLLM = globalCfg.Logging.LLM
				logAgent = globalCfg.Logging.Agent
			}
			if spec.Logging != nil {
				if spec.Logging.LLM != nil {
					logLLM = *spec.Logging.LLM
				}
				if spec.Logging.Agent != nil {
					logAgent = *spec.Logging.Agent
				}
			}
			eucloOutput := isEucloAgent(agentName, spec)
			if eucloOutput {
				requestedMode := strings.TrimSpace(mode)
				if !modeFlagChanged {
					fmt.Fprint(cmd.OutOrStdout(), eucloReadyHint(agentName))
					return nil
				}
				mode = requestedMode
				if mode == "" || strings.EqualFold(mode, "default") {
					return fmt.Errorf("unknown euclo mode %q; valid modes: %s", requestedMode, strings.Join(eucloModeNames(eucloModeRegistry()), ", "))
				}
				if err := validateEucloMode(mode); err != nil {
					return err
				}
			} else if mode == "" {
				if spec.Mode != "" {
					mode = string(spec.Mode)
				} else {
					mode = "default"
				}
			}
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
			runtimeCfg.ManifestPath = manifest.SourcePath
			runtimeCfg.AgentName = agentName
			if err := runtimeCfg.Normalize(); err != nil {
				return err
			}
			modelName := spec.Model.Name
			if modelName == "" {
				modelName = defaultModelName()
			}
			wsCfg := ayenitd.WorkspaceConfig{
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
				TelemetryPath:     telemetryPath,
				EventsPath:        eventsLogPath,
				MemoryPath:        runtimeCfg.MemoryPath,
				MaxIterations:     8,
				SkipASTIndex:      skipASTIndex,
				HITLTimeout:       runtimeCfg.HITLTimeout,
				AuditLimit:        runtimeCfg.AuditLimit,
				Sandbox:           runtimeCfg.Sandbox,
				DebugLLM:          logLLM,
				DebugAgent:        logAgent,
			}
			if wsCfg.LogPath == "" {
				wsCfg.LogPath = config.New(wsCfg.Workspace).LogFile("ayenitd.log")
			}
			openedWS, err := openWorkspaceFn(runCtx, wsCfg)
			if err != nil {
				return err
			}
			defer func() {
				_ = openedWS.Close()
			}()
			if openedWS.ServiceManager != nil {
				if err := openedWS.ServiceManager.StartAll(runCtx); err != nil {
					return err
				}
			}
			registration := openedWS.Registration
			if registration == nil {
				return fmt.Errorf("workspace registration missing")
			}
			if autoApprove {
				registration.Permissions.SetDefaultPolicy(core.AgentPermissionAllow)
				spec.Bash.Default = core.AgentPermissionAllow
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
			paths := config.New(wsCfg.Workspace)
			if openedWS.CompiledPolicy == nil {
				return fmt.Errorf("compiled policy missing from workspace")
			}
			registration.Policy = openedWS.CompiledPolicy.Engine
			openedWS.Environment.Registry.SetPolicyEngine(openedWS.CompiledPolicy.Engine)
			if openedWS.Environment.Config != nil && openedWS.Environment.Config.AgentSpec != nil {
				openedWS.Environment.Registry.UseAgentSpec(registration.ID, openedWS.Environment.Config.AgentSpec)
			}
			spec = openedWS.AgentSpec
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
			learningBroker := archaeolearning.NewBroker(0)
			relurpicOpts := []agents.RelurpicOption{
				agents.WithIndexManager(openedWS.Environment.IndexManager),
				agents.WithGraphDB(graphDBFromEnv(openedWS.Environment)),
				agents.WithRetrievalDB(retrievalDBFromEnv(openedWS.Environment)),
				agents.WithPlanStore(openedWS.Environment.PlanStore),
				agents.WithGuidanceBroker(openedWS.Environment.GuidanceBroker),
				agents.WithWorkflowStore(openedWS.Environment.WorkflowStore),
			}
			if err := registerBuiltinRelurpicCapabilitiesFn(
				openedWS.Environment.Registry, openedWS.Environment.Model, openedWS.Environment.Config, relurpicOpts...,
			); err != nil {
				return fmt.Errorf("register relurpic capabilities: %w", err)
			}
			agentEnv := agents.AgentEnvironment{
				Config:        openedWS.Environment.Config,
				CommandPolicy: openedWS.Environment.CommandPolicy,
				Model:         openedWS.Environment.Model,
				Registry:      openedWS.Environment.Registry,
				IndexManager:  openedWS.Environment.IndexManager,
				SearchEngine:  openedWS.Environment.SearchEngine,
				Memory:        openedWS.Environment.Memory,
			}
			if err := registerAgentCapabilitiesFn(openedWS.Environment.Registry, agentEnv); err != nil {
				return fmt.Errorf("register agent capabilities: %w", err)
			}
			cfg := &core.Config{
				Name:              agentName,
				Model:             modelName,
				InferenceEndpoint: defaultEndpoint(),
				MaxIterations:     8,
				NativeToolCalling: spec.NativeToolCallingEnabled(),
				AgentSpec:         spec,
				DebugLLM:          logLLM,
				DebugAgent:        logAgent,
			}
			providerRuntime := &appruntime.Runtime{
				Tools:        openedWS.Environment.Registry,
				Context:      core.NewContext(),
				Registration: registration,
				AgentSpec:    spec,
				Model:        openedWS.Environment.Model,
				IndexManager: openedWS.Environment.IndexManager,
				SearchEngine: openedWS.Environment.SearchEngine,
				Memory:       openedWS.Environment.Memory,
			}
			if err := registerBuiltinProvidersFn(runCtx, providerRuntime); err != nil {
				return err
			}
			agentEnv.Config = cfg
			var agent graph.WorkflowExecutor
			if eucloOutput {
				agent = buildAndWireEucloAgentFn(openedWS, learningBroker)
				if agent == nil {
					return fmt.Errorf("euclo agent builder returned nil")
				}
			} else {
				var buildErr error
				agent, buildErr = buildFromSpecFn(agentEnv, *spec)
				if buildErr != nil {
					agent, buildErr = buildFromSpecFn(agentEnv, core.AgentRuntimeSpec{Implementation: "react"})
				}
				if buildErr != nil {
					return buildErr
				}
			}
			if react, ok := agent.(*agents.ReActAgent); ok {
				react.CheckpointPath = paths.CheckpointsDir()
			}
			execCtx, stopSignals := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
			defer stopSignals()
			ctx, cancel := context.WithTimeout(execCtx, 10*time.Minute)
			defer cancel()
			startedAt := time.Now()
			task := &core.Task{
				ID:          fmt.Sprintf("cli-%d", time.Now().UnixNano()),
				Instruction: instruction,
				Type:        core.TaskTypeCodeGeneration,
				Context: map[string]any{
					"mode":      mode,
					"workspace": ws,
				},
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
			state := core.NewContext()
			state.Set("task.id", task.ID)
			state.Set("task.type", string(task.Type))
			state.Set("task.instruction", task.Instruction)
			for _, skill := range openedWS.SkillResults {
				if !skill.Applied || skill.Paths.Root == "" {
					continue
				}
				state.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
			}
			if task.ID != "" {
				defer state.ClearHandleScope(task.ID)
			}
			result, err := agent.Execute(ctx, task, state)
			if err != nil {
				return err
			}
			summary := buildExecutionSummary(task.ID, mode, result, state, openedWS.SkillResults, time.Since(startedAt), eucloOutput)
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				return encoder.Encode(summary)
			}
			if eucloOutput {
				fmt.Fprintf(
					cmd.OutOrStdout(),
					"Euclo complete (node=%s, mode=%s, artifact_kinds=%v, artifact_paths=%v, recorded=%v)\n",
					summary.ResultNode, summary.Mode, summary.ArtifactKinds, summary.ArtifactPaths, summary.Recorded,
				)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Agent complete (node=%s): %+v\n", result.NodeID, result.Data)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "default", "Execution mode (code, architect, ask, debug, security, docs)")
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
func selectDefaultAgent(reg *agents.Registry) string {
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
	if globalCfg != nil && globalCfg.DefaultModel.Name != "" {
		return globalCfg.DefaultModel.Name
	}
	return "codellama:13b"
}

// defaultEndpoint resolves the Ollama endpoint, honoring overrides from env.
func defaultEndpoint() string {
	if val := os.Getenv("OLLAMA_HOST"); val != "" {
		return val
	}
	return "http://localhost:11434"
}
