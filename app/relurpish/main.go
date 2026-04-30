package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

var (
	cfg         = runtimesvc.DefaultConfig()
	startServer bool
)

// main bootstraps the relurpish CLI/TUI entrypoint.
func main() {
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
			return cfg.Normalize()
		},
	}
	root.PersistentFlags().StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "Workspace directory")
	root.PersistentFlags().StringVar(&cfg.ManifestPath, "manifest", cfg.ManifestPath, "Agent manifest path")
	root.PersistentFlags().StringVar(&cfg.InferenceEndpoint, "inference-endpoint", cfg.InferenceEndpoint, "Inference backend endpoint URL")
	root.PersistentFlags().StringVar(&cfg.InferenceModel, "inference-model", cfg.InferenceModel, "Inference backend model name")
	root.PersistentFlags().StringVar(&cfg.SandboxBackend, "sandbox-backend", cfg.SandboxBackend, "Sandbox backend to use (gvisor or docker)")
	root.PersistentFlags().StringVar(&cfg.AgentName, "agent", cfg.AgentLabel(), "Agent preset (coding, planner, react, reflection)")
	root.PersistentFlags().StringVar(&cfg.ServerAddr, "addr", cfg.ServerAddr, "HTTP server listen address")
	root.PersistentFlags().StringVar(&cfg.Sandbox.RunscPath, "runsc", cfg.Sandbox.RunscPath, "runsc binary path")
	root.PersistentFlags().StringVar(&cfg.Sandbox.ContainerRuntime, "container-runtime", cfg.Sandbox.ContainerRuntime, "Container runtime (docker/containerd)")
	root.PersistentFlags().StringVar(&cfg.Sandbox.Platform, "sandbox-platform", cfg.Sandbox.Platform, "Sandbox platform hint (gVisor: kvm/ptrace)")
	root.PersistentFlags().BoolVar(&startServer, "serve", false, "Launch the HTTP API server alongside the TUI")

	root.AddCommand(newDoctorCmd(), newStatusCmd(), newChatCmd(), newServeCmd())
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

// newServeCmd runs only the HTTP server, useful for automation.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run only the HTTP API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, func(cmdCtx context.Context, rt *runtimesvc.Runtime) error {
				stop, err := rt.StartServer(cmdCtx, cfg.ServerAddr)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "relurpish API listening on %s\n", cfg.ServerAddr)
				<-cmdCtx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return stop(shutdownCtx)
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
	rt, err := runtimesvc.New(ctx, cfg)
	if err != nil {
		if shouldRunDoctorFallback(err) {
			fmt.Fprintln(cmd.ErrOrStderr(), "Workspace is not ready. Running doctor...")
			if doctorErr := runDoctor(cmd, false, false); doctorErr != nil {
				return doctorErr
			}
			rt, err = runtimesvc.New(ctx, cfg)
			if err == nil {
				defer rt.Close()
				return fn(ctx, rt)
			}
		}
		return err
	}
	defer rt.Close()
	return fn(ctx, rt)
}

// runTUI optionally starts the server and then launches the Bubble Tea program.
func runTUI(ctx context.Context, rt *runtimesvc.Runtime) error {
	var stop func(context.Context) error
	var err error
	if startServer {
		stop, err = rt.StartServer(ctx, cfg.ServerAddr)
		if err != nil {
			return err
		}
		defer stop(context.Background())
	}
	// Prevent stdlib logger output (used by some debug paths) from drawing over the TUI.
	if rt != nil && rt.Logger != nil {
		log.SetOutput(rt.Logger.Writer())
	}
	plugin := &tui.EucloPlugin{
		SetupTabs: euclotui.RegisterEucloTabs,
	}
	return tui.RunWithEuclo(ctx, rt, plugin)
}

func runDoctor(cmd *cobra.Command, fix, yes bool) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	report := runtimesvc.BuildDoctorReport(ctx, cfg)
	renderDoctorReport(cmd.OutOrStdout(), report)

	shouldOfferInit := report.NeedsInitialization() || fix
	if shouldOfferInit {
		overwrite := fix
		if yes || confirmDoctorAction(cmd.InOrStdin(), cmd.OutOrStdout(), doctorPrompt(report, fix)) {
			if err := runtimesvc.InitializeWorkspaceFromTemplates(cfg, overwrite); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Workspace starter configuration written to relurpify_cfg/")
			report = runtimesvc.BuildDoctorReport(ctx, cfg)
			renderDoctorReport(cmd.OutOrStdout(), report)
		}
	}
	if report.HasBlockingIssues() {
		return fmt.Errorf("doctor found blocking issues")
	}
	return nil
}

func renderDoctorReport(w io.Writer, report runtimesvc.DoctorReport) {
	fmt.Fprintf(w, "Workspace: %s\n", report.Workspace)
	fmt.Fprintf(w, "Config root: %s\n", report.ConfigRoot)
	fmt.Fprintf(w, "Workspace present: %s\n", yesNo(report.WorkspacePresent))
	fmt.Fprintf(w, "Config file: %s", yesNo(report.ConfigExists))
	if report.ConfigError != "" {
		fmt.Fprintf(w, " (%s)", report.ConfigError)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Manifest file: %s", yesNo(report.ManifestExists))
	if report.ManifestError != "" {
		fmt.Fprintf(w, " (%s)", report.ManifestError)
	}
	fmt.Fprintln(w)
	if len(report.ManifestWarnings) > 0 {
		fmt.Fprintln(w, "Manifest warnings:")
		for _, warning := range report.ManifestWarnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
	if len(report.DeprecationNotices) > 0 {
		fmt.Fprintln(w, "Deprecation notices:")
		for _, notice := range report.DeprecationNotices {
			fmt.Fprintf(w, "  - %s\n", notice)
		}
	}
	if report.ManifestFingerprint != "" {
		fmt.Fprintf(w, "Manifest fingerprint: %s\n", report.ManifestFingerprint)
	}
	if report.ManifestPolicySummary != "" {
		fmt.Fprintf(w, "Manifest policy: %s\n", report.ManifestPolicySummary)
	}
	if len(report.ProtectedPaths) > 0 {
		fmt.Fprintf(w, "Sandbox roots: %s\n", strings.Join(report.ProtectedPaths, ", "))
	}
	if report.Inference.SelectedProfile != "" {
		fmt.Fprintf(w, "  profile: %s\n", report.Inference.SelectedProfile)
	}
	if report.Inference.ProfileReason != "" {
		fmt.Fprintf(w, "  profile_reason: %s\n", report.Inference.ProfileReason)
	}
	if report.Inference.ProfileSource != "" {
		fmt.Fprintf(w, "  profile_source: %s\n", report.Inference.ProfileSource)
	}
	fmt.Fprintln(w, "Inference backend:")
	fmt.Fprintf(w, "  provider: %s\n", firstNonEmpty(report.Inference.Provider, "unknown"))
	fmt.Fprintf(w, "  endpoint: %s\n", firstNonEmpty(report.Inference.Endpoint, "-"))
	fmt.Fprintf(w, "  state: %s\n", firstNonEmpty(string(report.Inference.State), "unknown"))
	if len(report.Inference.Models) > 0 {
		fmt.Fprintf(w, "  models: %s\n", strings.Join(report.Inference.Models, ", "))
	} else {
		fmt.Fprintln(w, "  models: -")
	}
	if report.Inference.SelectedModel != "" {
		fmt.Fprintf(w, "  selected: %s\n", report.Inference.SelectedModel)
	}
	if report.Inference.Error != "" {
		fmt.Fprintf(w, "  error: %s\n", report.Inference.Error)
	}
	fmt.Fprintln(w, "Dependencies:")
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
			fmt.Fprintf(w, "  - %s: %s [%s] (%s)\n", dep.Name, status, severity, dep.Details)
		} else {
			fmt.Fprintf(w, "  - %s: %s [%s]\n", dep.Name, status, severity)
		}
	}
	if report.HasBlockingIssues() {
		fmt.Fprintln(w, "Result: blocking issues detected")
	} else {
		fmt.Fprintln(w, "Result: ready")
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
	fmt.Fprint(out, prompt)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shouldRunDoctorFallback(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return os.IsNotExist(err) ||
		strings.Contains(msg, "missing spec.agent") ||
		strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "missing spec.agent.model.name")
}
