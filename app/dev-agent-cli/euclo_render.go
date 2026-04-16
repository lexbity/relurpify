package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func jsonFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup("json")
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup("json")
	}
	return flag != nil && flag.Value.String() == "true"
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeCapabilityTable(w io.Writer, entries []CapabilityCatalogEntry) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tOWNER\tMODE\tCLASS\tLAYER\tBASELINE\tSUMMARY")
	for _, entry := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\t%s\n", entry.ID, entry.PrimaryOwner, entry.ModeFamilies[0], entry.ExecutionClass, entry.PreferredTestLayer, entry.BaselineEligible, entry.Summary)
	}
	return tw.Flush()
}

func writeCapabilityDetail(w io.Writer, entry *CapabilityCatalogEntry) error {
	if entry == nil {
		return errors.New("capability entry is nil")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FIELD\tVALUE")
	fmt.Fprintf(tw, "id\t%s\n", entry.ID)
	fmt.Fprintf(tw, "display_name\t%s\n", entry.DisplayName)
	fmt.Fprintf(tw, "primary_owner\t%s\n", entry.PrimaryOwner)
	fmt.Fprintf(tw, "mode_family\t%s\n", entry.ModeFamilies[0])
	fmt.Fprintf(tw, "execution_class\t%s\n", entry.ExecutionClass)
	fmt.Fprintf(tw, "preferred_test_layer\t%s\n", entry.PreferredTestLayer)
	fmt.Fprintf(tw, "allowed_test_layers\t%s\n", strings.Join(entry.AllowedTestLayers, ", "))
	fmt.Fprintf(tw, "baseline_eligible\t%t\n", entry.BaselineEligible)
	fmt.Fprintf(tw, "primary_capable\t%t\n", entry.PrimaryCapable)
	fmt.Fprintf(tw, "supporting_only\t%t\n", entry.SupportingOnly)
	fmt.Fprintf(tw, "mutability\t%s\n", entry.Mutability)
	fmt.Fprintf(tw, "supporting_routines\t%s\n", strings.Join(entry.SupportingRoutines, ", "))
	fmt.Fprintf(tw, "expected_artifact_kinds\t%s\n", strings.Join(entry.ExpectedArtifactKinds, ", "))
	fmt.Fprintf(tw, "supported_transition_targets\t%s\n", strings.Join(entry.SupportedTransitionTargets, ", "))
	fmt.Fprintf(tw, "supporting_capabilities\t%s\n", strings.Join(entry.SupportingCapabilities, ", "))
	fmt.Fprintf(tw, "transition_compatible\t%s\n", strings.Join(entry.TransitionCompatible, ", "))
	fmt.Fprintf(tw, "benchmark_eligible\t%t\n", entry.BenchmarkEligible)
	fmt.Fprintf(tw, "summary\t%s\n", entry.Summary)
	return tw.Flush()
}

func writeTriggerTable(w io.Writer, entries []TriggerCatalogEntry) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODE\tFAMILY\tPHASES\tPHRASE\tCAPABILITY\tPHASE\tREQUIRES\tDESCRIPTION")
	for _, entry := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", triggerModeDisplay(entry.Mode), entry.ModeIntentFamily, strings.Join(entry.ModePhases, ","), strings.Join(entry.Phrases, ", "), entry.CapabilityID, entry.PhaseJump, entry.RequiresMode, entry.Description)
	}
	return tw.Flush()
}

func triggerModeDisplay(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "global"
	}
	return mode
}

func writeJourneySummary(w io.Writer, report *EucloJourneyReport) error {
	if report == nil {
		return errors.New("journey report is nil")
	}
	fmt.Fprintf(w, "journey %s: %d/%d steps succeeded\n", report.RunClass, countSuccessfulJourneySteps(report.Steps), len(report.Steps))
	if len(report.Failures) > 0 {
		for _, failure := range report.Failures {
			fmt.Fprintf(w, "  %s\n", failure)
		}
	}
	return nil
}

func writeBenchmarkSummary(w io.Writer, report *EucloBenchmarkReport) error {
	if report == nil {
		return errors.New("benchmark report is nil")
	}
	fmt.Fprintf(w, "benchmark %s\n", report.Matrix.Name)
	fmt.Fprintf(w, "  cases: %d total, %d passed, %d failed\n", report.Summary.TotalCases, report.Summary.PassedCases, report.Summary.FailedCases)
	fmt.Fprintf(w, "  journey cases: %d\n", report.Summary.JourneyCases)
	fmt.Fprintf(w, "  success: %t\n", report.Success)
	if len(report.Summary.UniqueCapabilities) > 0 {
		fmt.Fprintf(w, "  capabilities: %s\n", strings.Join(report.Summary.UniqueCapabilities, ", "))
	}
	if len(report.Summary.UniqueModels) > 0 {
		fmt.Fprintf(w, "  models: %s\n", strings.Join(report.Summary.UniqueModels, ", "))
	}
	if len(report.Summary.UniqueProviders) > 0 {
		fmt.Fprintf(w, "  providers: %s\n", strings.Join(report.Summary.UniqueProviders, ", "))
	}
	if len(report.Cases) > 0 {
		fmt.Fprintln(w, "  rows:")
		for _, row := range report.Cases {
			fmt.Fprintf(w, "    [%d] cap=%s model=%s provider=%s success=%t\n", row.Index, row.Capability, row.Model, row.Provider, row.Success)
			if row.Message != "" {
				fmt.Fprintf(w, "      %s\n", row.Message)
			}
		}
	}
	return nil
}

func writeBaselineSummary(w io.Writer, report *EucloBaselineReport) error {
	if report == nil {
		return errors.New("baseline report is nil")
	}
	fmt.Fprintf(w, "baseline %s\n", report.Layer)
	fmt.Fprintf(w, "  exact: %t\n", report.Exact)
	fmt.Fprintf(w, "  benchmark_aggregation_disabled: %t\n", report.BenchmarkAggregationDisabled)
	fmt.Fprintf(w, "  success: %t\n", report.Success)
	if len(report.Capabilities) > 0 {
		fmt.Fprintln(w, "  capabilities:")
		for _, entry := range report.Capabilities {
			status := "ok"
			if !entry.Success {
				status = "fail"
			}
			fmt.Fprintf(w, "    - %s [%s] %s\n", entry.Selector, status, entry.Message)
		}
	}
	if len(report.Failures) > 0 {
		fmt.Fprintln(w, "  failures:")
		for _, failure := range report.Failures {
			fmt.Fprintf(w, "    %s\n", failure)
		}
	}
	return nil
}

func writeBenchmarkMatrixSummary(w io.Writer, matrix []EucloBenchmarkCaseReport) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "IDX\tCAPABILITY\tMODEL\tPROVIDER\tSUCCESS\tMESSAGE")
	for _, row := range matrix {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%t\t%s\n", row.Index, row.Capability, row.Model, row.Provider, row.Success, row.Message)
	}
	return tw.Flush()
}

func expandEucloBenchmarkMatrix(selector string, capabilities []CapabilityCatalogEntry, models, providers []string) []EucloBenchmarkCaseReport {
	if len(capabilities) == 0 {
		capabilities = []CapabilityCatalogEntry{{ID: selector}}
	}
	if len(models) == 0 {
		models = []string{""}
	}
	if len(providers) == 0 {
		providers = []string{""}
	}
	rows := make([]EucloBenchmarkCaseReport, 0, len(capabilities)*len(models)*len(providers))
	for _, capability := range capabilities {
		for _, model := range models {
			for _, provider := range providers {
				rows = append(rows, EucloBenchmarkCaseReport{
					RunClass:   "benchmark",
					Capability: capability.ID,
					Model:      strings.TrimSpace(model),
					Provider:   strings.TrimSpace(provider),
					Success:    true,
					Message:    "expanded matrix row",
				})
			}
		}
	}
	return rows
}

func countSuccessfulJourneySteps(steps []EucloJourneyStepReport) int {
	count := 0
	for _, step := range steps {
		if step.Success {
			count++
		}
	}
	return count
}

func summarizeEucloBenchmarkCases(cases []EucloBenchmarkCaseReport) EucloBenchmarkSummary {
	summary := EucloBenchmarkSummary{
		TotalCases: len(cases),
	}
	capSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	providerSet := map[string]struct{}{}
	for _, entry := range cases {
		if entry.Journey != nil {
			summary.JourneyCases++
		}
		if entry.Success {
			summary.PassedCases++
		} else {
			summary.FailedCases++
		}
		if strings.TrimSpace(entry.Capability) != "" {
			capSet[entry.Capability] = struct{}{}
		}
		if strings.TrimSpace(entry.Model) != "" {
			modelSet[entry.Model] = struct{}{}
		}
		if strings.TrimSpace(entry.Provider) != "" {
			providerSet[entry.Provider] = struct{}{}
		}
	}
	summary.UniqueCapabilities = sortedSetKeys(capSet)
	summary.UniqueModels = sortedSetKeys(modelSet)
	summary.UniqueProviders = sortedSetKeys(providerSet)
	return summary
}

func sortedSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
