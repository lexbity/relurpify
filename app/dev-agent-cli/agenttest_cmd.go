package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
	agenttapes "codeburg.org/lexbit/relurpify/testsuite/agenttest/tapes"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"github.com/spf13/cobra"
)

type agentTestRunner interface {
	RunSuite(context.Context, *agenttest.Suite, agenttest.RunOptions) (*agenttest.SuiteReport, error)
}

var newAgentTestRunnerFn = func() agentTestRunner {
	return &agenttest.Runner{}
}

const (
	agentTestRunName      = "run"
	agentTestPromoteName  = "promote"
	agentTestReportName   = "report"
	agentTestRerecordName = "rerecord"
)

func newAgentTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agenttest",
		Short: "Run YAML-driven agent test suites",
	}
	cmd.AddCommand(newAgentTestRunCmd())
	cmd.AddCommand(newAgentTestPromoteCmd())
	cmd.AddCommand(newAgentTestReportCmd())
	cmd.AddCommand(newAgentTestRerecordCmd())
	return cmd
}

func newAgentTestRunCmd() *cobra.Command {
	var suites []string
	var agentName string
	var caseName string
	var tags []string
	var tier string
	var profile string
	var strict bool
	var includeQuarantined bool
	var outDir string
	var sandbox bool
	var timeout time.Duration
	var bootstrapTimeout time.Duration
	var skipASTIndex bool
	var maxRetries int
	var model string
	var endpoint string
	var maxIterations int
	var debugLLM bool
	var debugAgent bool
	var backendReset string
	var backendBin string
	var backendService string
	var backendResetBetween bool
	var backendResetOn []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or more agent testsuites",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			selectedSuites := suites
			if len(selectedSuites) == 0 {
				selectedSuites = discoverSuites(ws, agentName)
			}
			if len(selectedSuites) == 0 {
				return fmt.Errorf("no testsuites found; pass --suite <path> or add suites to testsuite/agenttests/")
			}
			loadedSuites := make([]*agenttest.Suite, 0, len(selectedSuites))
			for _, suitePath := range selectedSuites {
				suite, err := agenttest.LoadSuite(suitePath)
				if err != nil {
					return err
				}
				if !shouldRunAgentTestSuite(suite, tier, profile, includeQuarantined) {
					continue
				}
				suite, err = filterAgentTestSuiteCases(suite, caseName, tags)
				if err != nil {
					return fmt.Errorf("%s: %w", suitePath, err)
				}
				loadedSuites = append(loadedSuites, suite)
			}
			if len(loadedSuites) == 0 {
				return fmt.Errorf("no testsuites matched the requested filters")
			}
			opts := agenttest.RunOptions{
				TargetWorkspace:     ws,
				OutputDir:           outDir,
				Sandbox:             sandbox,
				Timeout:             timeout,
				BootstrapTimeout:    bootstrapTimeout,
				SkipASTIndex:        skipASTIndex,
				Profile:             profile,
				Strict:              strict,
				MaxRetries:          maxRetries,
				SharedRoot:          config.ResolveSharedRoot(""),
				ModelOverride:       model,
				EndpointOverride:    endpoint,
				MaxIterations:       maxIterations,
				DebugLLM:            debugLLM,
				DebugAgent:          debugAgent,
				BackendReset:        backendReset,
				BackendBinary:       backendBin,
				BackendService:      backendService,
				BackendResetBetween: backendResetBetween,
				BackendResetOn:      backendResetOn,
			}
			runner := newAgentTestRunnerFn()
			for _, suite := range loadedSuites {
				rep, err := runner.RunSuite(cmd.Context(), suite, opts)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %d/%d passed (%d infra, %d assertion)\n", filepath.Base(suite.SourcePath), rep.Profile, rep.PassedCases, rep.PassedCases+rep.FailedCases, rep.InfraFailures, rep.AssertFailures)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Run suites matching <agent> in testsuite/agenttests/")
	cmd.Flags().StringVar(&caseName, "case", "", "Only run a single case by name")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Only run cases carrying at least one matching tag (repeatable)")
	cmd.Flags().StringVar(&tier, "tier", "", "Only run suites in the requested tier (smoke|stable|live-flaky|quarantined)")
	cmd.Flags().StringVar(&profile, "profile", "", "Override execution profile (live|record|replay|developer-live|ci-live|ci-replay)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail the process if any non-skipped case fails")
	cmd.Flags().BoolVar(&includeQuarantined, "include-quarantined", false, "Include suites marked quarantined")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts")
	cmd.Flags().BoolVar(&sandbox, "sandbox", true, "Run tool execution via gVisor/docker")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
	cmd.Flags().DurationVar(&bootstrapTimeout, "bootstrap-timeout", 30*time.Second, "Per-case bootstrap timeout")
	cmd.Flags().BoolVar(&skipASTIndex, "skip-ast-index", true, "Skip AST/bootstrap indexing during setup")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retry attempts per case")
	cmd.Flags().StringVar(&model, "model", "", "Override model name for all cases")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override inference endpoint for all cases")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 8, "Override max iterations for agent loops")
	cmd.Flags().BoolVar(&debugLLM, "debug-llm", false, "Enable verbose LLM telemetry logging")
	cmd.Flags().BoolVar(&debugAgent, "debug-agent", false, "Enable verbose agent debug logging")
	cmd.Flags().StringVar(&backendReset, "backend-reset", "none", "Reset strategy: none|model|server")
	cmd.Flags().StringVar(&backendBin, "backend-bin", "ollama", "Inference backend CLI binary name/path")
	cmd.Flags().StringVar(&backendService, "backend-service", "ollama", "systemd service name for backend restarts")
	cmd.Flags().BoolVar(&backendResetBetween, "backend-reset-between", false, "Reset before each case")
	cmd.Flags().StringArrayVar(&backendResetOn, "backend-reset-on", []string{
		"(?i)context deadline exceeded",
		"(?i)connection reset",
		"(?i)EOF",
		"(?i)too many requests",
	}, "Regex patterns that trigger backend reset+retry (repeatable)")
	return cmd
}

func newAgentTestPromoteCmd() *cobra.Command {
	var suites []string
	var agentName string
	var runDir string
	var caseName string
	var all bool

	cmd := &cobra.Command{
		Use:   agentTestPromoteName,
		Short: "Promote a completed agenttest run into golden tape fixtures",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			suitePath, err := resolveSingleSuitePath(ws, suites, agentName)
			if err != nil {
				return err
			}
			if strings.TrimSpace(runDir) == "" {
				return fmt.Errorf("--run is required")
			}
			if !all && strings.TrimSpace(caseName) == "" {
				return fmt.Errorf("either --case or --all is required")
			}
			report, err := agenttapes.PromoteRun(ws, suitePath, runDir, caseName, all)
			if err != nil {
				return err
			}
			for _, cr := range report.Cases {
				if cr.SourceTapePath != "" && cr.DestinationTapePath != "" {
					if err := writef(cmd.OutOrStdout(), "promoted %s -> %s\n", cr.SourceTapePath, cr.DestinationTapePath); err != nil {
						return err
					}
				}
				if cr.SourceInteractionTapePath != "" && cr.DestinationInteractionTapePath != "" {
					if err := writef(cmd.OutOrStdout(), "promoted %s -> %s\n", cr.SourceInteractionTapePath, cr.DestinationInteractionTapePath); err != nil {
						return err
					}
				}
				if cr.DestinationBaselinePath != "" {
					if err := writef(cmd.OutOrStdout(), "promoted baseline %s\n", cr.DestinationBaselinePath); err != nil {
						return err
					}
				}
				if cr.LineagePath != "" {
					if err := writef(cmd.OutOrStdout(), "wrote lineage %s\n", cr.LineagePath); err != nil {
						return err
					}
				}
				if len(cr.PromotedArtifacts) > 0 {
					if err := writef(cmd.OutOrStdout(), "artifacts: %s\n", strings.Join(cr.PromotedArtifacts, ", ")); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Promote suites matching <agent> in testsuite/agenttests/")
	cmd.Flags().StringVar(&runDir, "run", "", "Path to the completed run directory")
	cmd.Flags().StringVar(&caseName, "case", "", "Promote a single case by name")
	cmd.Flags().BoolVar(&all, "all", false, "Promote all successful cases in the run")
	return cmd
}

func newAgentTestReportCmd() *cobra.Command {
	var suites []string
	var agentName string

	cmd := &cobra.Command{
		Use:   agentTestReportName,
		Short: "Report golden tape and baseline coverage",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			suitePaths, err := resolveTapeSuitePaths(ws, suites, agentName)
			if err != nil {
				return err
			}
			report, err := agenttapes.BuildCoverageReport(ws, suitePaths, time.Now().UTC())
			if err != nil {
				return err
			}
			return printCoverageReport(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Report suites matching <agent> in testsuite/agenttests/")
	return cmd
}

func newAgentTestRerecordCmd() *cobra.Command {
	var suites []string
	var agentName string

	cmd := &cobra.Command{
		Use:   agentTestRerecordName,
		Short: "Plan rerecording for missing or stale golden tapes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			suitePaths, err := resolveTapeSuitePaths(ws, suites, agentName)
			if err != nil {
				return err
			}
			plan, err := agenttapes.BuildRerecordPlan(ws, suitePaths, time.Now().UTC())
			if err != nil {
				return err
			}
			return printRerecordPlan(cmd.OutOrStdout(), plan)
		},
	}
	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Plan suites matching <agent> in testsuite/agenttests/")
	return cmd
}

func ensureWorkspace() string {
	if strings.TrimSpace(workspace) != "" {
		return workspace
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	workspace = wd
	return workspace
}

func discoverSuites(ws, agentName string) []string {
	canonicalDir := filepath.Join(ws, "testsuite", "agenttests")
	prefix := ""
	if strings.TrimSpace(agentName) != "" {
		prefix = sanitizeName(agentName)
	}
	suffix := ".testsuite.yaml"

	readSuites := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var matches []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, suffix) && (prefix == "" || strings.HasPrefix(name, prefix)) {
				matches = append(matches, filepath.Join(dir, name))
			}
		}
		return matches
	}

	matches := readSuites(canonicalDir)
	if len(matches) > 0 {
		return matches
	}
	fallbackDir := config.New(ws).TestsuitesDir()
	return readSuites(fallbackDir)
}

func resolveTapeSuitePaths(ws string, suites []string, agentName string) ([]string, error) {
	if len(suites) == 0 {
		suites = discoverSuites(ws, agentName)
	}
	if len(suites) == 0 {
		return nil, fmt.Errorf("no testsuites found; pass --suite <path> or add suites to testsuite/agenttests/")
	}
	return append([]string(nil), suites...), nil
}

func resolveSingleSuitePath(ws string, suites []string, agentName string) (string, error) {
	suitePaths, err := resolveTapeSuitePaths(ws, suites, agentName)
	if err != nil {
		return "", err
	}
	if len(suitePaths) != 1 {
		return "", fmt.Errorf("promote requires exactly one suite; got %d", len(suitePaths))
	}
	return suitePaths[0], nil
}

func printCoverageReport(out io.Writer, report *agenttapes.CoverageReport) error {
	if report == nil {
		return nil
	}
	if err := writef(out, "Workspace: %s\n", report.Workspace); err != nil {
		return err
	}
	if err := writef(out, "Totals: %d suites, %d cases, %d present, %d missing, %d stale\n", report.Totals.Suites, report.Totals.Cases, report.Totals.Present, report.Totals.Missing, report.Totals.Stale); err != nil {
		return err
	}
	for _, suite := range report.Suites {
		if err := writef(out, "Suite: %s\n", suite.SuiteName); err != nil {
			return err
		}
		for _, entry := range suite.Entries {
			if err := writef(out, "  %s / %s\n", entry.CaseName, entry.ModelName); err != nil {
				return err
			}
			if err := writef(out, "    tape: %s\n", entry.Status); err != nil {
				return err
			}
			if err := writef(out, "    baseline: %s\n", entry.BaselineStatus); err != nil {
				return err
			}
		}
	}
	return nil
}

func printRerecordPlan(out io.Writer, plan *agenttapes.RerecordPlan) error {
	if plan == nil {
		return nil
	}
	if err := writef(out, "Workspace: %s\n", plan.Workspace); err != nil {
		return err
	}
	if err := writef(out, "Rerecord targets: %d\n", plan.Totals.Entries); err != nil {
		return err
	}
	for _, entry := range plan.Entries {
		if err := writef(out, "  %s / %s\n", entry.Entry.CaseName, entry.Entry.ModelName); err != nil {
			return err
		}
		if err := writef(out, "    reason: %s\n", entry.Reason); err != nil {
			return err
		}
		if entry.SuggestedCommand != "" {
			if err := writef(out, "    command: %s\n", entry.SuggestedCommand); err != nil {
				return err
			}
		}
	}
	return nil
}

func writef(out io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(out, format, args...)
	return err
}

func filterAgentTestSuiteCases(suite *agenttest.Suite, caseName string, tags []string) (*agenttest.Suite, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite required")
	}
	filtered := *suite
	filtered.Spec.Cases = nil
	for _, c := range suite.Spec.Cases {
		if strings.TrimSpace(caseName) != "" && !strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(caseName)) {
			continue
		}
		if len(tags) > 0 && !c.MatchesAnyTag(tags) {
			continue
		}
		filtered.Spec.Cases = append(filtered.Spec.Cases, c)
	}
	if len(filtered.Spec.Cases) == 0 {
		return nil, fmt.Errorf("no cases matched the requested filters")
	}
	return &filtered, nil
}

func shouldRunAgentTestSuite(suite *agenttest.Suite, tier, profile string, includeQuarantined bool) bool {
	if suite == nil {
		return false
	}
	if suite.Metadata.Quarantined && !includeQuarantined {
		return false
	}
	if !suite.MatchesTier(tier) {
		return false
	}
	if !suite.MatchesProfile(profile) {
		return false
	}
	return true
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "agent"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return "agent"
	}
	return out
}
