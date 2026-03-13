package main

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/testsuite/agenttest"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"time"
)

func newAgentTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agenttest",
		Short: "Run YAML-driven agent test suites",
	}
	cmd.AddCommand(newAgentTestRunCmd())
	return cmd
}

func newAgentTestRunCmd() *cobra.Command {
	var suites []string
	var agentName string
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
	var model string
	var endpoint string
	var maxIterations int
	var debugLLM bool
	var debugAgent bool
	var ollamaReset string
	var ollamaBin string
	var ollamaService string
	var ollamaResetBetween bool
	var ollamaResetOn []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or more agent testsuites",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				suite = agenttest.FilterSuiteCasesByTags(suite, tags)
				if len(suite.Spec.Cases) == 0 {
					continue
				}
				loadedSuites = append(loadedSuites, suite)
			}
			if len(loadedSuites) == 0 {
				return fmt.Errorf("no testsuites matched the requested filters")
			}
			r := &agenttest.Runner{}
			opts := agenttest.RunOptions{
				TargetWorkspace:    ws,
				OutputDir:          outDir,
				Sandbox:            sandbox,
				Timeout:            timeout,
				BootstrapTimeout:   bootstrapTimeout,
				SkipASTIndex:       skipASTIndex,
				Profile:            profile,
				Strict:             strict,
				ModelOverride:      model,
				EndpointOverride:   endpoint,
				MaxIterations:      maxIterations,
				DebugLLM:           debugLLM,
				DebugAgent:         debugAgent,
				OllamaReset:        ollamaReset,
				OllamaBinary:       ollamaBin,
				OllamaService:      ollamaService,
				OllamaResetBetween: ollamaResetBetween,
				OllamaResetOn:      ollamaResetOn,
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
			}
			if hadFailures {
				return fmt.Errorf("one or more agenttest suites failed in strict mode (%d infra failures, %d assertion/agent failures)", totalInfraFailures, totalAssertFailures)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Run suites matching <agent> in testsuite/agenttests/")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Only run cases carrying at least one matching tag (repeatable)")
	cmd.Flags().StringVar(&lane, "lane", "", "Apply a CI lane preset (pr-smoke|merge-stable|quarantined-live)")
	cmd.Flags().StringVar(&tier, "tier", "", "Only run suites in the requested tier (smoke|stable|live-flaky|quarantined)")
	cmd.Flags().StringVar(&profile, "profile", "", "Override execution profile (live|record|replay|developer-live|ci-live|ci-replay)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail the process if any non-skipped case fails; implied by ci-live and ci-replay profiles")
	cmd.Flags().BoolVar(&includeQuarantined, "include-quarantined", false, "Include suites marked quarantined")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts (default: relurpify_cfg/test_runs/...)")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Run tool execution via gVisor/docker (requires runsc + docker)")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
	cmd.Flags().DurationVar(&bootstrapTimeout, "bootstrap-timeout", 30*time.Second, "Per-case bootstrap timeout for agent/runtime setup before execution")
	cmd.Flags().BoolVar(&skipASTIndex, "skip-ast-index", false, "Test-only fast path: skip AST/bootstrap indexing during agenttest setup")
	cmd.Flags().StringVar(&model, "model", "", "Override model name for all cases")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Override Ollama endpoint for all cases")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 8, "Override max iterations for agent loops")
	cmd.Flags().BoolVar(&debugLLM, "debug-llm", false, "Enable verbose LLM telemetry logging")
	cmd.Flags().BoolVar(&debugAgent, "debug-agent", false, "Enable verbose agent debug logging")
	cmd.Flags().StringVar(&ollamaReset, "ollama-reset", "none", "Reset strategy: none|model|server")
	cmd.Flags().StringVar(&ollamaBin, "ollama-bin", "ollama", "Ollama CLI binary name/path")
	cmd.Flags().StringVar(&ollamaService, "ollama-service", "ollama", "systemd service name for server restarts")
	cmd.Flags().BoolVar(&ollamaResetBetween, "ollama-reset-between", false, "Reset before each case")
	cmd.Flags().StringArrayVar(&ollamaResetOn, "ollama-reset-on", []string{
		"(?i)context deadline exceeded",
		"(?i)connection reset",
		"(?i)EOF",
		"(?i)too many requests",
	}, "Regex patterns that trigger reset+retry (repeatable)")
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
	cfgDir := config.New(ws).TestsuitesDir()
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

type agentTestLanePreset struct {
	Tier               string
	Profile            string
	Strict             bool
	IncludeQuarantined bool
}

func resolveAgentTestLane(name string) (agentTestLanePreset, error) {
	switch name {
	case "":
		return agentTestLanePreset{}, nil
	case "pr-smoke":
		return agentTestLanePreset{
			Tier:    "smoke",
			Profile: "ci-live",
			Strict:  true,
		}, nil
	case "merge-stable":
		return agentTestLanePreset{
			Tier:    "stable",
			Profile: "ci-live",
			Strict:  true,
		}, nil
	case "quarantined-live":
		return agentTestLanePreset{
			Profile:            "ci-live",
			IncludeQuarantined: true,
			Strict:             true,
		}, nil
	default:
		return agentTestLanePreset{}, fmt.Errorf("unknown agenttest lane %q", name)
	}
}
