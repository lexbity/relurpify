package cmd

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/testsuite/agenttest"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
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
	var outDir string
	var sandbox bool
	var timeout time.Duration
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
			ws := ensureWorkspace()
			if len(suites) == 0 {
				suites = discoverSuites(ws, agentName)
			}
			if len(suites) == 0 {
				return fmt.Errorf("no testsuites found; pass --suite <path> or add suites to testsuite/agenttests/")
			}
			r := &agenttest.Runner{}
			opts := agenttest.RunOptions{
				TargetWorkspace:    ws,
				OutputDir:          outDir,
				Sandbox:            sandbox,
				Timeout:            timeout,
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
			for _, suitePath := range suites {
				suite, err := agenttest.LoadSuite(suitePath)
				if err != nil {
					return err
				}
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				rep, err := r.RunSuite(ctx, suite, opts)
				if err != nil {
					return err
				}
				passed, total, skipped := 0, 0, 0
				for _, c := range rep.Cases {
					if c.Skipped {
						skipped++
						continue
					}
					total++
					if c.Success {
						passed++
					}
				}
				artifactDir := ""
				if len(rep.Cases) > 0 {
					artifactDir = filepath.Dir(rep.Cases[0].ArtifactsDir)
				}
				if skipped > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %d/%d passed (%d skipped) (artifacts: %s)\n", filepath.Base(suitePath), passed, total, skipped, artifactDir)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %d/%d passed (artifacts: %s)\n", filepath.Base(suitePath), passed, total, artifactDir)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&suites, "suite", nil, "Path to a testsuite YAML (repeatable)")
	cmd.Flags().StringVar(&agentName, "agent", "", "Run suites matching <agent> in testsuite/agenttests/")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for run artifacts (default: relurpify_cfg/test_runs/...)")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Run tool execution via gVisor/docker (requires runsc + docker)")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Second, "Per-case timeout")
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
	cfgDir := workspacecfg.New(ws).TestsuitesDir()
	if _, err := os.Stat(cfgDir); err == nil {
		matches, _ = filepath.Glob(filepath.Join(cfgDir, pattern))
	}
	return matches
}
