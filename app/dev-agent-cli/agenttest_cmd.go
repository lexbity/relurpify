package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
	"github.com/spf13/cobra"
)

type agentTestRunner interface {
	RunSuite(context.Context, *agenttest.Suite, agenttest.RunOptions) (*agenttest.SuiteReport, error)
}

var newAgentTestRunnerFn = func() agentTestRunner {
	return &agenttest.Runner{}
}

func newAgentTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agenttest",
		Short: "Run YAML-driven agent test suites",
	}
	cmd.AddCommand(newAgentTestRunCmd())
	cmd.AddCommand(newAgentTestPromoteCmd())
	cmd.AddCommand(newAgentTestRefreshCmd())
	cmd.AddCommand(newAgentTestTapesCmd())
	// Phase 8: Migration command removed - all YAML files migrated
	return cmd
}

func newAgentTestRunCmd() *cobra.Command {
	var suites []string
	var agentName string
	var caseName string
	var tags []string
	var lane string
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
	var capability string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or more agent testsuites",
		RunE: func(cmd *cobra.Command, args []string) error {
			if capability != "" {
				// Phase 4: capability-targeted scan - filter to cases that match the capability
				return runCapabilityTargeted(cmd.Context(), capability, suites, agentName)
			}
			preset, err := resolveAgentTestLane(lane)
			if err != nil {
				return err
			}
			if tier == "" {
				tier = preset.Tier
			}
			if profile == "" {
				profile = preset.Profile
			}
			if !strict {
				strict = preset.Strict
			}
			if !includeQuarantined {
				includeQuarantined = preset.IncludeQuarantined
			}
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
			r := newAgentTestRunnerFn()
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
			hadFailures := false
			totalInfraFailures := 0
			totalAssertFailures := 0
			for _, suite := range loadedSuites {
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				rep, err := r.RunSuite(ctx, suite, opts)
				if err != nil {
					return err
				}
				passed, total, skipped := rep.PassedCases, rep.PassedCases+rep.FailedCases, rep.SkippedCases
				artifactDir := ""
				if len(rep.Cases) > 0 {
					artifactDir = filepath.Dir(rep.Cases[0].ArtifactsDir)
				}
				totalInfraFailures += rep.InfraFailures
				totalAssertFailures += rep.AssertFailures
				if rep.Strict && passed != total {
					hadFailures = true
				}
				if skipped > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %d/%d passed (%d skipped, %d infra, %d assertion, %dms) (artifacts: %s)\n", filepath.Base(suite.SourcePath), rep.Profile, passed, total, skipped, rep.InfraFailures, rep.AssertFailures, rep.DurationMS, artifactDir)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %d/%d passed (%d infra, %d assertion, %dms) (artifacts: %s)\n", filepath.Base(suite.SourcePath), rep.Profile, passed, total, rep.InfraFailures, rep.AssertFailures, rep.DurationMS, artifactDir)
				}
				if rep.Performance.CasesWithBaseline > 0 {
					if rep.Performance.CasesAboveBaseline > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  Performance: %d within baseline, %d above baseline\n", rep.Performance.CasesWithinBaseline, rep.Performance.CasesAboveBaseline)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "  Performance: %d within baseline\n", rep.Performance.CasesWithinBaseline)
					}
				}
				if rep.Benchmark != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  Benchmark: family=%s score=%.1f success_rate=%.2f\n", rep.Benchmark.ScoreFamily, rep.Benchmark.Summary.OverallScore, rep.Benchmark.Summary.SuccessRate)
					if rep.Benchmark.Comparison.BaselineFound {
						fmt.Fprintf(cmd.OutOrStdout(), "    baseline=%s delta=%.1f within_variance=%t\n", rep.Benchmark.Comparison.BaselinePath, rep.Benchmark.Comparison.Delta, rep.Benchmark.Comparison.WithinVariance)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "    baseline missing\n")
					}
				}
			}
			if hadFailures {
				return fmt.Errorf("one or more agenttest suites failed in strict mode (%d infra failures, %d assertion/agent failures)", totalInfraFailures, totalAssertFailures)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Run suites matching <agent> in testsuite/agenttests/")
	cmd.Flags().StringVar(&caseName, "case", "", "Only run a single case by name")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Only run cases carrying at least one matching tag (repeatable)")
	cmd.Flags().StringVar(&lane, "lane", "", "Apply a CI lane preset (pr-smoke|merge-stable|quarantined-live)")
	cmd.Flags().StringVar(&tier, "tier", "", "Only run suites in the requested tier (smoke|stable|live-flaky|quarantined)")
	cmd.Flags().StringVar(&profile, "profile", "", "Override execution profile (live|record|replay|developer-live|ci-live|ci-replay)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail the process if any non-skipped case fails; implied by ci-live and ci-replay profiles")
	cmd.Flags().BoolVar(&includeQuarantined, "include-quarantined", false, "Include suites marked quarantined")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts (default: workspace-local relurpify_cfg/test_runs/...)")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Run tool execution via gVisor/docker (requires runsc + docker)")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
	cmd.Flags().DurationVar(&bootstrapTimeout, "bootstrap-timeout", 30*time.Second, "Per-case bootstrap timeout for agent/runtime setup before execution")
	cmd.Flags().BoolVar(&skipASTIndex, "skip-ast-index", true, "Default true for live agenttests: skip AST/bootstrap indexing during setup; use --skip-ast-index=false for dedicated AST-enabled end-to-end runs")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retry attempts per case for backend reset/retry handling; use -1 to disable retries")
	cmd.Flags().StringVar(&model, "model", "", "Override model name for all cases")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override Ollama endpoint for all cases")
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
	cmd.Flags().StringVar(&capability, "capability", "", "Run only cases matching the specified capability ID (Phase 4)")
	return cmd
}

// runCapabilityTargeted scans all suites and runs only cases matching the specified capability.
// Phase 4: Implements capability-targeted test discovery.
func runCapabilityTargeted(ctx context.Context, capabilityID string, suites []string, agentName string) error {
	ws := ensureWorkspace()
	selectedSuites := suites
	if len(selectedSuites) == 0 {
		selectedSuites = discoverSuites(ws, agentName)
	}
	if len(selectedSuites) == 0 {
		return fmt.Errorf("no testsuites found; pass --suite <path> or add suites to testsuite/agenttests/")
	}

	var matchingCases []*agenttest.Suite
	for _, suitePath := range selectedSuites {
		suite, err := agenttest.LoadSuite(suitePath)
		if err != nil {
			return err
		}
		var filteredCases []agenttest.CaseSpec
		for _, c := range suite.Spec.Cases {
			// Phase 8: Updated to use Benchmark.Euclo
			if c.CapabilityDirectRun != nil && c.CapabilityDirectRun.CapabilityID == capabilityID {
				filteredCases = append(filteredCases, c)
				continue
			}
			euclo := c.Expect.Benchmark
			if euclo != nil && euclo.Euclo != nil && euclo.Euclo.PrimaryRelurpicCapability == capabilityID {
				filteredCases = append(filteredCases, c)
				continue
			}
			// Check supporting capabilities
			if euclo != nil && euclo.Euclo != nil {
				for _, supp := range euclo.Euclo.SupportingRelurpicCapabilities {
					if supp == capabilityID {
						filteredCases = append(filteredCases, c)
						break
					}
				}
			}
		}
		if len(filteredCases) > 0 {
			// Create a copy of suite with only matching cases
			filteredSuite := *suite
			filteredSuite.Spec.Cases = filteredCases
			matchingCases = append(matchingCases, &filteredSuite)
		}
	}

	if len(matchingCases) == 0 {
		return fmt.Errorf("no test cases found for capability %q", capabilityID)
	}

	r := newAgentTestRunnerFn()
	opts := agenttest.RunOptions{
		TargetWorkspace: ws,
	}

	hadFailures := false
	for _, suite := range matchingCases {
		rep, err := r.RunSuite(ctx, suite, opts)
		if err != nil {
			return err
		}
		passed, total := rep.PassedCases, rep.PassedCases+rep.FailedCases
		fmt.Printf("%s: %d/%d passed for capability %s\n", filepath.Base(suite.SourcePath), passed, total, capabilityID)
		if rep.Strict && passed != total {
			hadFailures = true
		}
	}

	if hadFailures {
		return fmt.Errorf("one or more capability-targeted tests failed")
	}
	return nil
}

func newAgentTestPromoteCmd() *cobra.Command {
	var suitePath string
	var runDir string
	var caseName string
	var all bool

	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote recorded run tapes into the golden tape directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(suitePath) == "" {
				return errors.New("--suite is required")
			}
			if strings.TrimSpace(runDir) == "" {
				return errors.New("--run is required")
			}
			if !all && strings.TrimSpace(caseName) == "" {
				return errors.New("pass --case <name> or --all")
			}
			return promoteAgentTestRun(ensureWorkspace(), suitePath, runDir, caseName, all, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&suitePath, "suite", "", "Path to a testsuite YAML")
	cmd.Flags().StringVar(&runDir, "run", "", "Path to a completed agenttest run directory")
	cmd.Flags().StringVar(&caseName, "case", "", "Promote a single case by name")
	cmd.Flags().BoolVar(&all, "all", false, "Promote all passing cases from the run")
	return cmd
}

func newAgentTestRefreshCmd() *cobra.Command {
	var suitePath string
	var caseName string
	var tags []string
	var model string
	var endpoint string
	var outDir string
	var timeout time.Duration
	var bootstrapTimeout time.Duration
	var skipASTIndex bool
	var maxRetries int

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Re-record live tapes for a suite or case and promote them to golden tapes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(suitePath) == "" {
				return errors.New("--suite is required")
			}
			ws := ensureWorkspace()
			suite, err := agenttest.LoadSuite(suitePath)
			if err != nil {
				return err
			}
			suite, err = filterAgentTestSuiteCases(suite, caseName, tags)
			if err != nil {
				return fmt.Errorf("%s: %w", suitePath, err)
			}
			suite.Spec.Recording.Strategy = "live"
			suite.Spec.Recording.Mode = "record"
			suite.Spec.Recording.Tape = ""
			r := newAgentTestRunnerFn()
			opts := agenttest.RunOptions{
				TargetWorkspace:  ws,
				OutputDir:        outDir,
				Timeout:          timeout,
				BootstrapTimeout: bootstrapTimeout,
				SkipASTIndex:     skipASTIndex,
				MaxRetries:       maxRetries,
				ModelOverride:    model,
				EndpointOverride: endpoint,
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			report, err := r.RunSuite(ctx, suite, opts)
			if err != nil {
				return err
			}
			runRoot := reportRunRoot(report)
			if runRoot == "" {
				return errors.New("unable to determine run directory for refresh")
			}
			passed := report.PassedCases == len(report.Cases)
			if !passed {
				return fmt.Errorf("refresh run failed: %d/%d cases passed; tapes not promoted", report.PassedCases, len(report.Cases))
			}
			promoteAll := strings.TrimSpace(caseName) == ""
			return promoteAgentTestRun(ws, suitePath, runRoot, caseName, promoteAll, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&suitePath, "suite", "", "Path to a testsuite YAML")
	cmd.Flags().StringVar(&caseName, "case", "", "Refresh a single case by name")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Only refresh cases carrying at least one matching tag (repeatable)")
	cmd.Flags().StringVar(&model, "model", "", "Override model name for the refresh run")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override Ollama endpoint for the refresh run")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
	cmd.Flags().DurationVar(&bootstrapTimeout, "bootstrap-timeout", 30*time.Second, "Per-case bootstrap timeout")
	cmd.Flags().BoolVar(&skipASTIndex, "skip-ast-index", true, "Default true for live agenttests: skip AST/bootstrap indexing during setup; use --skip-ast-index=false for dedicated AST-enabled end-to-end runs")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retry attempts per case")
	return cmd
}

func filterAgentTestSuiteCases(suite *agenttest.Suite, caseName string, tags []string) (*agenttest.Suite, error) {
	if suite == nil {
		return nil, errors.New("suite is required")
	}
	filtered := agenttest.FilterSuiteCasesByTags(suite, tags)
	if strings.TrimSpace(caseName) == "" {
		if len(filtered.Spec.Cases) == 0 {
			if len(tags) == 0 {
				return nil, errors.New("suite has no cases")
			}
			return nil, fmt.Errorf("no cases matched tags %s", strings.Join(tags, ", "))
		}
		return filtered, nil
	}

	selected := *filtered
	selected.Spec = filtered.Spec
	selected.Spec.Cases = nil
	for _, c := range filtered.Spec.Cases {
		if c.Name == caseName {
			selected.Spec.Cases = append(selected.Spec.Cases, c)
		}
	}
	if len(selected.Spec.Cases) == 0 {
		if len(tags) == 0 {
			return nil, fmt.Errorf("case %q not found", caseName)
		}
		return nil, fmt.Errorf("case %q not found after applying tags %s", caseName, strings.Join(tags, ", "))
	}
	return &selected, nil
}

func newAgentTestTapesCmd() *cobra.Command {
	var suites []string
	var agentName string

	cmd := &cobra.Command{
		Use:   "tapes",
		Short: "Report golden tape coverage and staleness",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := ensureWorkspace()
			selectedSuites := suites
			if len(selectedSuites) == 0 {
				selectedSuites = discoverSuites(ws, agentName)
			}
			if len(selectedSuites) == 0 {
				return fmt.Errorf("no testsuites found; pass --suite <path> or add suites to testsuite/agenttests/")
			}
			return reportAgentTestTapes(ws, selectedSuites, cmd.OutOrStdout(), time.Now().UTC())
		},
	}
	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Only report suites matching <agent> in testsuite/agenttests/")
	return cmd
}

// discoverSuites returns suite paths from testsuite/agenttests/ (the canonical
// source), optionally filtered by agent name prefix.
func discoverSuites(ws, agentName string) []string {
	canonicalDir := filepath.Join(ws, "testsuite", "agenttests")
	pattern := "*.testsuite.yaml"
	if agentName != "" {
		pattern = fmt.Sprintf("%s*.testsuite.yaml", sanitizeName(agentName))
	}
	matches, _ := filepath.Glob(filepath.Join(canonicalDir, pattern))
	if len(matches) > 0 {
		return matches
	}
	// Fallback: check relurpify_cfg/testsuites/ for locally-added suites.
	cfgDir := manifest.New(ws).TestsuitesDir()
	if _, err := os.Stat(cfgDir); err == nil {
		matches, _ = filepath.Glob(filepath.Join(cfgDir, pattern))
	}
	return matches
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
