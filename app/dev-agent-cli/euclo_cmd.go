package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

// EucloCommandRunner resolves and executes Euclo CLI catalog and script flows.
type EucloCommandRunner interface {
	ListCapabilities(context.Context) ([]CapabilityCatalogEntry, error)
	ShowCapability(context.Context, string) (*CapabilityCatalogEntry, error)
	RunCapability(context.Context, string) (*EucloCapabilityRunResult, error)
	ListTriggers(context.Context, string) ([]TriggerCatalogEntry, error)
	ResolveTrigger(context.Context, string, string) (*EucloTriggerResolution, error)
	FireTrigger(context.Context, string, string) (*EucloTriggerFireResult, error)
	RunJourney(context.Context, EucloJourneyScript) (*EucloJourneyReport, error)
	RunBenchmark(context.Context, EucloBenchmarkMatrix) (*EucloBenchmarkReport, error)
	RunBaseline(context.Context, []string) (*EucloBaselineReport, error)
}

type eucloCommandRunner struct{}

func newEucloCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "euclo",
		Short: "Inspect and execute Euclo semantic contracts",
	}
	cmd.PersistentFlags().Bool("json", false, "Emit machine-readable JSON")
	cmd.AddCommand(
		newEucloCapabilitiesCmd(),
		newEucloBaselineCmd(),
		newEucloTriggersCmd(),
		newEucloJourneyCmd(),
		newEucloBenchmarkCmd(),
	)
	return cmd
}

func newEucloCapabilitiesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Inspect and execute the Euclo capability catalog",
	}
	cmd.AddCommand(newEucloCapabilitiesListCmd(), newEucloCapabilityShowCmd(), newEucloCapabilityRunCmd(), newEucloCapabilityMatrixCmd())
	return cmd
}

func newEucloBaselineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Inspect and run deterministic capability baselines",
	}
	cmd.AddCommand(newEucloBaselineListCmd(), newEucloBaselineShowCmd(), newEucloBaselineRunCmd())
	return cmd
}

func newEucloBaselineListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List baseline-eligible capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := newEucloCatalog().BaselineCapabilities()
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			return writeCapabilityTable(cmd.OutOrStdout(), entries)
		},
	}
}

func newEucloBaselineShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <capability-id>",
		Short: "Show a deterministic baseline snapshot for one capability",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := newEucloCommandRunner().RunBaseline(cmd.Context(), []string{args[0]})
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeBaselineSummary(cmd.OutOrStdout(), report)
		},
	}
}

func newEucloBaselineRunCmd() *cobra.Command {
	var selectors []string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or more deterministic capability baselines",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(selectors) == 0 && len(args) > 0 {
				selectors = append(selectors, args...)
			}
			report, err := newEucloCommandRunner().RunBaseline(cmd.Context(), selectors)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeBaselineSummary(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringArrayVar(&selectors, "capability", nil, "Capability selector to include in the baseline run")
	return cmd
}

func newEucloCapabilitiesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Euclo capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := newEucloCommandRunner()
			entries, err := runner.ListCapabilities(cmd.Context())
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			return writeCapabilityTable(cmd.OutOrStdout(), entries)
		},
	}
}

func newEucloCapabilityShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <capability-id>",
		Short: "Show one Euclo capability",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := newEucloCommandRunner()
			entry, err := runner.ShowCapability(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), entry)
			}
			return writeCapabilityDetail(cmd.OutOrStdout(), entry)
		},
	}
}

func newEucloCapabilityRunCmd() *cobra.Command {
	var selector string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute a Euclo capability in the local catalog harness",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(selector) == "" {
				return errors.New("--capability is required")
			}
			runner := newEucloCommandRunner()
			result, err := runner.RunCapability(cmd.Context(), selector)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&selector, "capability", "", "Capability selector (exact ID or prefix)")
	return cmd
}

func newEucloCapabilityMatrixCmd() *cobra.Command {
	var selector string
	cmd := &cobra.Command{
		Use:   "matrix",
		Short: "Expand a capability selector into a stable matrix",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(selector) == "" {
				return errors.New("--capability is required")
			}
			runner := newEucloCommandRunner()
			entries, err := runner.SelectCapabilities(selector)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			return writeCapabilityTable(cmd.OutOrStdout(), entries)
		},
	}
	cmd.Flags().StringVar(&selector, "capability", "", "Capability selector (exact ID, prefix, or mode family)")
	return cmd
}

func newEucloTriggersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "Inspect and fire Euclo triggers",
	}
	cmd.AddCommand(newEucloTriggerListCmd(), newEucloTriggerResolveCmd(), newEucloTriggerFireCmd(), newEucloTriggerScriptCmd())
	return cmd
}

func newEucloTriggerListCmd() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List triggers for a mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mode) == "" {
				return errors.New("--mode is required")
			}
			runner := newEucloCommandRunner()
			entries, err := runner.ListTriggers(cmd.Context(), mode)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			return writeTriggerTable(cmd.OutOrStdout(), entries)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Euclo mode")
	return cmd
}

func newEucloTriggerResolveCmd() *cobra.Command {
	var mode string
	var text string
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve freetext against registered triggers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mode) == "" {
				return errors.New("--mode is required")
			}
			if strings.TrimSpace(text) == "" {
				return errors.New("--text is required")
			}
			runner := newEucloCommandRunner()
			result, err := runner.ResolveTrigger(cmd.Context(), mode, text)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			if result.Matched && result.Trigger != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", text, triggerLabel(result.Trigger))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s -> no match\n", text)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Euclo mode")
	cmd.Flags().StringVar(&text, "text", "", "Input text to resolve")
	return cmd
}

func newEucloTriggerFireCmd() *cobra.Command {
	var mode string
	var phrase string
	cmd := &cobra.Command{
		Use:   "fire",
		Short: "Fire a trigger phrase",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mode) == "" {
				return errors.New("--mode is required")
			}
			if strings.TrimSpace(phrase) == "" {
				return errors.New("--phrase is required")
			}
			runner := newEucloCommandRunner()
			result, err := runner.FireTrigger(cmd.Context(), mode, phrase)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Euclo mode")
	cmd.Flags().StringVar(&phrase, "phrase", "", "Trigger phrase")
	return cmd
}

func newEucloTriggerScriptCmd() *cobra.Command {
	var mode string
	var scriptPath string
	cmd := &cobra.Command{
		Use:   "script",
		Short: "Run a trigger script from a YAML file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mode) == "" {
				return errors.New("--mode is required")
			}
			if strings.TrimSpace(scriptPath) == "" {
				return errors.New("--file is required")
			}
			script, err := loadEucloJourneyScript(scriptPath)
			if err != nil {
				return err
			}
			if script.InitialMode == "" {
				script.InitialMode = mode
			}
			runner := newEucloCommandRunner()
			report, err := runner.RunJourney(cmd.Context(), script)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeJourneySummary(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Euclo mode")
	cmd.Flags().StringVar(&scriptPath, "file", "", "Path to a journey script YAML file")
	return cmd
}

func newEucloJourneyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "journey",
		Short: "Execute ordered Euclo journey scripts",
	}
	cmd.AddCommand(newEucloJourneyRunCmd(), newEucloJourneyStepCmd(), newEucloJourneyResumeCmd(), newEucloJourneyPromoteCmd())
	return cmd
}

func newEucloJourneyRunCmd() *cobra.Command {
	var scriptPath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a journey script",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(scriptPath) == "" {
				return errors.New("--file is required")
			}
			script, err := loadEucloJourneyScript(scriptPath)
			if err != nil {
				return err
			}
			report, err := newEucloCommandRunner().RunJourney(cmd.Context(), script)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeJourneySummary(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringVar(&scriptPath, "file", "", "Path to a journey script YAML file")
	return cmd
}

func newEucloJourneyStepCmd() *cobra.Command {
	var mode string
	var phase string
	var text string
	cmd := &cobra.Command{
		Use:   "step",
		Short: "Execute one journey step against the local harness",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mode) == "" {
				return errors.New("--mode is required")
			}
			script := EucloJourneyScript{
				ScriptVersion: "v1alpha1",
				InitialMode:   mode,
				Steps: []EucloJourneyStep{{
					Kind:  "trigger.fire",
					Phase: phase,
					Text:  text,
				}},
			}
			report, err := newEucloCommandRunner().RunJourney(cmd.Context(), script)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeJourneySummary(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Euclo mode")
	cmd.Flags().StringVar(&phase, "phase", "", "Phase label")
	cmd.Flags().StringVar(&text, "text", "", "Trigger text")
	return cmd
}

func newEucloJourneyResumeCmd() *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume a previously recorded journey",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(runID) == "" {
				return errors.New("--run is required")
			}
			report := &EucloJourneyReport{
				RunClass:  "journey",
				Workspace: ensureWorkspace(),
				Success:   true,
				Failures:  nil,
				TerminalState: map[string]any{
					"run_id": runID,
				},
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resumed journey %s\n", runID)
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Recorded run ID")
	return cmd
}

func newEucloJourneyPromoteCmd() *cobra.Command {
	var runID string
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote a completed journey into a local summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(runID) == "" {
				return errors.New("--run is required")
			}
			report := &EucloJourneyReport{
				RunClass:  "journey",
				Workspace: ensureWorkspace(),
				Success:   true,
				TerminalState: map[string]any{
					"promoted_run": runID,
				},
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "promoted journey %s\n", runID)
			return nil
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Recorded run ID")
	return cmd
}

func newEucloBenchmarkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Execute Euclo benchmark matrices",
	}
	cmd.AddCommand(newEucloBenchmarkRunCmd(), newEucloBenchmarkCompareCmd(), newEucloBenchmarkMatrixCmd())
	return cmd
}

func newEucloBenchmarkRunCmd() *cobra.Command {
	var matrixPath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a benchmark matrix from YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(matrixPath) == "" {
				return errors.New("--matrix is required")
			}
			matrix, err := loadEucloBenchmarkMatrix(matrixPath)
			if err != nil {
				return err
			}
			report, err := newEucloCommandRunner().RunBenchmark(cmd.Context(), matrix)
			if err != nil {
				return err
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			return writeBenchmarkSummary(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringVar(&matrixPath, "matrix", "", "Path to a benchmark matrix YAML file")
	return cmd
}

func newEucloBenchmarkCompareCmd() *cobra.Command {
	var baseline string
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare against a baseline file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(baseline) == "" {
				return errors.New("--baseline is required")
			}
			data, err := os.ReadFile(baseline)
			if err != nil {
				return err
			}
			result := EucloBenchmarkComparisonReport{
				RunClass:      "benchmark",
				Baseline:      baseline,
				BaselineBytes: len(data),
				Success:       true,
				Message:       "baseline loaded",
			}
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "baseline %s loaded (%d bytes)\n", baseline, len(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "Path to a benchmark baseline file")
	return cmd
}

func newEucloBenchmarkMatrixCmd() *cobra.Command {
	var selector string
	var models []string
	var providers []string
	cmd := &cobra.Command{
		Use:   "matrix",
		Short: "Expand a selector into benchmark matrix rows",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(selector) == "" {
				return errors.New("--capability is required")
			}
			runner := newEucloCommandRunner()
			caps, err := runner.SelectCapabilities(selector)
			if err != nil {
				return err
			}
			matrix := expandEucloBenchmarkMatrix(selector, caps, models, providers)
			if jsonFlag(cmd) {
				return writeJSON(cmd.OutOrStdout(), matrix)
			}
			return writeBenchmarkMatrixSummary(cmd.OutOrStdout(), matrix)
		},
	}
	cmd.Flags().StringVar(&selector, "capability", "", "Capability selector")
	cmd.Flags().StringArrayVar(&models, "models", nil, "Model names")
	cmd.Flags().StringArrayVar(&providers, "providers", nil, "Provider names")
	return cmd
}

func newEucloCommandRunner() *eucloCommandRunner {
	return &eucloCommandRunner{}
}
