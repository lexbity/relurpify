package cmd

import (
	"bufio"
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/agents"
	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/llm"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// newStartCmd constructs the `relurpify start` CLI command that runs an agent.
func newStartCmd() *cobra.Command {
	var mode string
	var agentName string
	var instruction string
	var dryRun bool
	var autoApprove bool
	var noSandbox bool
	var resumeLatestWorkflow bool
	var workflowID string
	var rerunFromStepID string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a coding agent session",
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
			spec = agents.ApplyManifestDefaults(spec, manifest.Spec.Defaults)
			spec = agents.ResolveAgentSpec(globalCfg, spec)
			if mode == "" {
				if spec.Mode != "" {
					mode = string(spec.Mode)
				} else {
					mode = string(agents.ModeCode)
				}
			}
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
			if instruction == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Agent %s ready in %s mode. Provide --instruction to execute a task.\n", agentName, mode)
				return nil
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %s in %s mode with instruction: %s\n", agentName, mode, instruction)
				return nil
			}
			runtimeCfg := appruntime.DefaultConfig()
			runtimeCfg.Workspace = ws
			runtimeCfg.ManifestPath = manifest.SourcePath
			if err := runtimeCfg.Normalize(); err != nil {
				return err
			}
			registration, err := fruntime.RegisterAgent(runCtx, fruntime.RuntimeConfig{
				ManifestPath: runtimeCfg.ManifestPath,
				Sandbox:      runtimeCfg.Sandbox,
				AuditLimit:   runtimeCfg.AuditLimit,
				BaseFS:       runtimeCfg.Workspace,
				HITLTimeout:  runtimeCfg.HITLTimeout,
			})
			if err != nil {
				return err
			}
			// In CLI mode there is no TUI to handle HITL approval prompts.
			if autoApprove {
				// --yes: bypass HITL by setting the default policy to allow and
				// overriding the bash_permissions default so cli_* tools don't ask.
				registration.Permissions.SetDefaultPolicy(fruntime.AgentPermissionAllow)
				spec.Bash.Default = core.AgentPermissionAllow
			} else {
				// Subscribe to the HITL broker and ask the user interactively on stdin.
				hitlEvents, unsub := registration.HITL.Subscribe(4)
				defer unsub()
				go func() {
					scanner := bufio.NewScanner(os.Stdin)
					for event := range hitlEvents {
						if event.Type != fruntime.HITLEventRequested || event.Request == nil {
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
							_ = registration.HITL.Approve(fruntime.PermissionDecision{
								RequestID:  req.ID,
								ApprovedBy: "cli-user",
								Scope:      fruntime.GrantScopeSession,
							})
						} else {
							_ = registration.HITL.Deny(req.ID, "denied by user")
						}
					}
				}()
			}
			var runner fruntime.CommandRunner
			if noSandbox {
				runner = fruntime.NewLocalCommandRunner(runtimeCfg.Workspace, nil)
			} else {
				sandboxRunner, err := fruntime.NewSandboxCommandRunner(registration.Manifest, registration.Runtime, runtimeCfg.Workspace)
				if err != nil {
					return err
				}
				runner = sandboxRunner
			}
			tools, indexManager, err := appruntime.BuildToolRegistry(ws, runner, appruntime.ToolRegistryOptions{
				AgentID:           registration.ID,
				PermissionManager: registration.Permissions,
				AgentSpec:         spec,
			})
			if err != nil {
				return err
			}
			spec, skillResults := agents.ApplySkills(ws, spec, manifest.Spec.Skills, tools, registration.Permissions, registration.ID)
			if spec.Logging != nil {
				if spec.Logging.LLM != nil {
					logLLM = *spec.Logging.LLM
				}
				if spec.Logging.Agent != nil {
					logAgent = *spec.Logging.Agent
				}
			}
			tools.UseAgentSpec(registration.ID, spec)
			providerRuntime := &appruntime.Runtime{
				Tools:        tools,
				Context:      core.NewContext(),
				Registration: registration,
			}
			if err := appruntime.RegisterSkillProviders(runCtx, providerRuntime, manifest.Spec.Skills); err != nil {
				return err
			}
			telemetry := telemetry.LoggerTelemetry{Logger: log.Default()}
			tools.UseTelemetry(telemetry)
			if registration.Permissions != nil {
				tools.UsePermissionManager(registration.ID, registration.Permissions)
			}
			memoryPath := filepath.Join(ws, "relurpify_cfg", "memory")
			memory, err := memory.NewHybridMemory(memoryPath)
			if err != nil {
				return err
			}
			modelName := spec.Model.Name
			if modelName == "" {
				modelName = defaultModelName()
			}
			client := llm.NewClient(defaultEndpoint(), modelName)
			client.SetDebugLogging(logLLM)
			agent := &agents.CodingAgent{
				Model:             llm.NewInstrumentedModel(client, telemetry, logLLM),
				Tools:             tools,
				Memory:            memory,
				IndexManager:      indexManager,
				CheckpointPath:    filepath.Join(ws, "relurpify_cfg", "sessions", "checkpoints"),
				WorkflowStatePath: filepath.Join(ws, "relurpify_cfg", "sessions", "workflow_state.db"),
			}
			cfg := &core.Config{
				Name:              agentName,
				Model:             modelName,
				OllamaEndpoint:    defaultEndpoint(),
				MaxIterations:     8,
				OllamaToolCalling: spec.ToolCallingEnabled(),
				AgentSpec:         spec,
				DebugLLM:          logLLM,
				DebugAgent:        logAgent,
			}
			if err := agent.Initialize(cfg); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			task := &core.Task{
				ID:          fmt.Sprintf("cli-%d", time.Now().UnixNano()),
				Instruction: instruction,
				Type:        core.TaskTypeCodeGeneration,
				Context: map[string]any{
					"mode": mode,
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
			for _, skill := range skillResults {
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
			fmt.Fprintf(cmd.OutOrStdout(), "Agent complete (node=%s): %+v\n", result.NodeID, result.Data)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", string(agents.ModeCode), "Execution mode (code, architect, ask, debug, security, docs)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Agent name from manifest registry")
	cmd.Flags().StringVar(&instruction, "instruction", "", "Instruction to execute")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without executing")
	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Auto-approve all HITL permission requests (skips interactive prompts)")
	cmd.Flags().BoolVar(&noSandbox, "no-sandbox", false, "Run commands directly on the host instead of inside a gVisor/Docker sandbox")
	cmd.Flags().BoolVar(&resumeLatestWorkflow, "resume-latest-workflow", false, "Resume the latest persisted architect workflow instead of starting from scratch")
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Resume or continue the specified workflow ID")
	cmd.Flags().StringVar(&rerunFromStepID, "rerun-from-step", "", "Replay a workflow from the specified step ID")
	return cmd
}

// selectDefaultAgent picks the first registry entry so users can run commands
// without specifying --agent.
func selectDefaultAgent(reg *agents.Registry) string {
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
