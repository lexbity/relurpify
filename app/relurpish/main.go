package main

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func init() {
	cfg.SubprocessToolFactory = func(m ports.ToolManifest) any {
		return subprocess.NewTool(m, nil)
	}
}

var (
	cfg          = runtimesvc.DefaultConfig()
	envSnapshot  []string
	envOverrides config.EnvOverrides
	secrets      config.Secrets
)

// main bootstraps the relurpish CLI/TUI entrypoint.
func main() {
	envSnapshot = config.ProcessEnv()
	var ovErr error
	envOverrides, ovErr = config.LoadEnvOverrides(envSnapshot)
	if ovErr != nil {
		log.Fatalf("invalid environment: %v", ovErr)
	}
	secrets = config.LoadSecrets(envSnapshot)
	tui.SetReduceMotionPreference(config.LoadReduceMotionPreference(envSnapshot))
	tui.SetTerminalNamePreference(config.LoadTerminalName(envSnapshot))
	cfg.EnvOverrides = append([]string(nil), envSnapshot...)
	cfg.Editor = envOverrides.Editor
	cfg.SharedRoot = config.ResolveSharedRoot(envOverrides.XDGDataHome)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	root := newRootCmd()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newRootCmd wires all subcommands and persistent flags.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "relurpish",
		Short:         "Bubble Tea shell for the Relurpify agent runtime",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if offline, _ := cmd.Flags().GetBool("offline"); offline {
				cfg.InferenceProvider = "offline"
			}
			return cfg.Normalize()
		},
	}
	root.PersistentFlags().StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "Workspace directory")
	root.PersistentFlags().StringVar(&cfg.InferenceEndpoint, "inference-endpoint", cfg.InferenceEndpoint, "Inference backend endpoint URL")
	root.PersistentFlags().StringVar(&cfg.InferenceModel, "inference-model", cfg.InferenceModel, "Inference backend model name")
	root.PersistentFlags().StringVar(&cfg.InferenceProvider, "inference-provider", cfg.InferenceProvider, "Inference backend provider (ollama, lmstudio, offline)")
	root.PersistentFlags().Bool("offline", false, "Use offline backend (sugar for --inference-provider offline)")
	root.PersistentFlags().StringVar(&cfg.SandboxBackend, "sandbox-backend", cfg.SandboxBackend, "Sandbox backend to use (gvisor or docker)")
	root.PersistentFlags().StringVar(&cfg.Sandbox.RunscPath, "runsc", cfg.Sandbox.RunscPath, "runsc binary path")
	root.PersistentFlags().StringVar(&cfg.Sandbox.ContainerRuntime, "container-runtime", cfg.Sandbox.ContainerRuntime, "Container runtime (docker/containerd)")
	root.PersistentFlags().StringVar(&cfg.Sandbox.Platform, "sandbox-platform", cfg.Sandbox.Platform, "Sandbox platform hint (gVisor: kvm/ptrace)")

	root.AddCommand(newDoctorCmd(), newStatusCmd(), newChatCmd())
	return root
}

func newDoctorCmd() *cobra.Command {
	var fix bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local runtime dependencies and workspace configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, fix, yes)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Overwrite or materialize starter workspace configuration from templates")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply doctor initialization/fix actions without prompting")
	return cmd
}

// newStatusCmd renders diagnostics for the workspace.
func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show workspace diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, func(ctx context.Context, rt *runtimesvc.Runtime) error {
				return runTUI(ctx, rt)
			})
		},
	}
	return cmd
}

// newChatCmd starts the chat-first TUI.
func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start the relurpish chat shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, func(ctx context.Context, rt *runtimesvc.Runtime) error {
				return runTUI(ctx, rt)
			})
		},
	}
	return cmd
}

// runWithRuntime ensures the runtime is created and cleaned up for the command.
func runWithRuntime(cmd *cobra.Command, fn func(context.Context, *runtimesvc.Runtime) error) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := runtimesvc.BootstrapStartupState(ctx, cfg, secrets); err != nil {
		return err
	}
	rt, err := runtimesvc.New(ctx, cfg, secrets)
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close(context.Background()) }()
	return fn(ctx, rt)
}

// runTUI launches the Bubble Tea program.
func runTUI(ctx context.Context, rt *runtimesvc.Runtime) error {
	// Prevent stdlib logger output (used by some debug paths) from drawing over the TUI.
	if rt != nil && rt.AgentWorkspace() != nil && rt.AgentWorkspace().Logger != nil {
		log.SetOutput(rt.AgentWorkspace().Logger.Writer())
	}
	if rt != nil {
		tui.SetEditor(rt.Config.Editor)
	}
	return tui.PTYSafe(func() error {
		return tui.RunWithSurface(ctx, rt, euclotui.NewSurfaceFactory())
	})
}

func runDoctor(cmd *cobra.Command, fix, yes bool) error {
	return tui.PTYSafe(func() error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		// Config validation is done by config.Load during runtime
		// initialization. Pre-flight errors (file not found, etc.) are
		// captured by BootstrapStartupState → BuildDoctorReport.
		state, err := runtimesvc.BootstrapStartupState(ctx, cfg, secrets)
		if err != nil {
			return err
		}
		report := state.Report
		renderDoctorReport(cmd.OutOrStdout(), report)

		shouldOfferInit := report.NeedsInitialization() || fix
		if shouldOfferInit {
			overwrite := fix
			if yes || confirmDoctorAction(cmd.InOrStdin(), cmd.OutOrStdout(), doctorPrompt(report, fix)) {
				if err := runtimesvc.InitializeWorkspaceFromTemplates(cfg, overwrite); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Workspace starter configuration written to relurpify_cfg/")
				report = runtimesvc.BuildDoctorReport(ctx, cfg, secrets)
				renderDoctorReport(cmd.OutOrStdout(), report)
			}
		}
		if report.HasBlockingIssues() {
			return fmt.Errorf("doctor found blocking issues")
		}
		return nil
	})
}

func renderDoctorReport(w io.Writer, report runtimesvc.DoctorReport) {
	_, _ = fmt.Fprintf(w, "Workspace: %s\n", report.Workspace)
	_, _ = fmt.Fprintf(w, "Config root: %s\n", report.ConfigRoot)
	_, _ = fmt.Fprintf(w, "Workspace present: %s\n", yesNo(report.WorkspacePresent))
	_, _ = fmt.Fprintf(w, "Config file: %s", yesNo(report.ConfigExists))
	if report.ConfigError != "" {
		_, _ = fmt.Fprintf(w, " (%s)", report.ConfigError)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Manifest file: %s", yesNo(report.ManifestExists))
	if report.ManifestError != "" {
		_, _ = fmt.Fprintf(w, " (%s)", report.ManifestError)
	}
	_, _ = fmt.Fprintln(w)
	if len(report.ManifestWarnings) > 0 {
		_, _ = fmt.Fprintln(w, "Manifest warnings:")
		for _, warning := range report.ManifestWarnings {
			_, _ = fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
	if len(report.DeprecationNotices) > 0 {
		_, _ = fmt.Fprintln(w, "Deprecation notices:")
		for _, notice := range report.DeprecationNotices {
			_, _ = fmt.Fprintf(w, "  - %s\n", notice)
		}
	}
	if report.ManifestFingerprint != "" {
		_, _ = fmt.Fprintf(w, "Manifest fingerprint: %s\n", report.ManifestFingerprint)
	}
	if report.ManifestPolicySummary != "" {
		_, _ = fmt.Fprintf(w, "Manifest policy: %s\n", report.ManifestPolicySummary)
	}
	if len(report.ProtectedPaths) > 0 {
		_, _ = fmt.Fprintf(w, "Sandbox roots: %s\n", strings.Join(report.ProtectedPaths, ", "))
	}
	if report.Inference.SelectedProfile != "" {
		_, _ = fmt.Fprintf(w, "  profile: %s\n", report.Inference.SelectedProfile)
	}
	if report.Inference.ProfileReason != "" {
		_, _ = fmt.Fprintf(w, "  profile_reason: %s\n", report.Inference.ProfileReason)
	}
	if report.Inference.ProfileSource != "" {
		_, _ = fmt.Fprintf(w, "  profile_source: %s\n", report.Inference.ProfileSource)
	}
	_, _ = fmt.Fprintln(w, "Inference backend:")
	_, _ = fmt.Fprintf(w, "  provider: %s\n", cmp.Or(report.Inference.Provider, "unknown"))
	_, _ = fmt.Fprintf(w, "  endpoint: %s\n", cmp.Or(report.Inference.Endpoint, "-"))
	_, _ = fmt.Fprintf(w, "  state: %s\n", cmp.Or(string(report.Inference.State), "unknown"))
	_, _ = fmt.Fprintf(w, "  tool_calling_mode: %s\n", cmp.Or(report.Inference.ToolCallingMode, "unknown"))
	if len(report.Inference.Models) > 0 {
		_, _ = fmt.Fprintf(w, "  models: %s\n", strings.Join(report.Inference.Models, ", "))
	} else {
		_, _ = fmt.Fprintln(w, "  models: -")
	}
	if report.Inference.SelectedModel != "" {
		_, _ = fmt.Fprintf(w, "  selected: %s\n", report.Inference.SelectedModel)
	}
	if report.Inference.Error != "" {
		_, _ = fmt.Fprintf(w, "  error: %s\n", report.Inference.Error)
	}
	_, _ = fmt.Fprintln(w, "Dependencies:")
	for _, dep := range report.Dependencies {
		status := "ok"
		if !dep.Available {
			status = "missing"
		}
		severity := "warning"
		if dep.Required {
			severity = "required"
		}
		if dep.Blocking {
			severity = "blocking"
		}
		if dep.Details != "" {
			_, _ = fmt.Fprintf(w, "  - %s: %s [%s] (%s)\n", dep.Name, status, severity, dep.Details)
		} else {
			_, _ = fmt.Fprintf(w, "  - %s: %s [%s]\n", dep.Name, status, severity)
		}
	}
	if report.HasBlockingIssues() {
		_, _ = fmt.Fprintln(w, "Result: blocking issues detected")
	} else {
		_, _ = fmt.Fprintln(w, "Result: ready")
	}
}

func doctorPrompt(report runtimesvc.DoctorReport, fix bool) string {
	if report.NeedsInitialization() {
		return "Initialize relurpify_cfg/ from starter templates? [y/N]: "
	}
	if fix {
		return "Overwrite current starter config and manifest from templates? [y/N]: "
	}
	return "Apply doctor fixes? [y/N]: "
}

func confirmDoctorAction(in io.Reader, out io.Writer, prompt string) bool {
	if prompt == "" {
		return false
	}
	_, _ = fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
