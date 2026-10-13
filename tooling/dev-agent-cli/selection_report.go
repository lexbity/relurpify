package main

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
)

// The route-selection record contract: the eventName and the attribute keys
// the dispatchers stamp on every euclo.route.selected record.
const (
	eventRouteSelected = "euclo.route.selected"
	attrRouteID        = "route_id"
	attrDecidedBy      = "decided_by"
	attrFallbackTaken  = "fallback_taken"
	attrTier2Used      = "tier2_used"
	attrTier2Outcome   = "tier2_outcome"
	attrDegradation    = "degradation"
)

// selectionSummary aggregates the euclo.route.selected telemetry events of one
// run directory into the per-suite selection decision statistics (FR-22):
// chosen-route histogram, deciding-rule histogram, fallback rate, Tier-2
// invocation rate, and Tier-2 rejection rate. The events carry the redacted
// utterance digest, never the raw utterance (D11).
type selectionSummary struct {
	FilesScanned        int
	Total               int
	RouteHistogram      map[string]int
	DecidedByHistogram  map[string]int
	FallbackCount       int
	Tier2UsedCount      int
	Tier2RejectionCount int
	DegradationCount    int
}

// buildSelectionSummary reads every OTLP-JSON telemetry segment under the run
// directory and aggregates the selection decision events. A run directory
// without telemetry produces an empty summary, not an error.
func buildSelectionSummary(runDir string) (*selectionSummary, error) {
	summary := &selectionSummary{
		RouteHistogram:     make(map[string]int),
		DecidedByHistogram: make(map[string]int),
	}
	files := collectTelemetryFiles(runDir)
	if len(files) == 0 {
		return summary, nil
	}
	for _, path := range files {
		if err := scanSelectionTelemetryFile(path, summary); err != nil {
			return nil, fmt.Errorf("scan telemetry %s: %w", path, err)
		}
	}
	summary.FilesScanned = len(files)
	return summary, nil
}

// collectTelemetryFiles walks the run directory for OTLP-JSON segments
// (*otlp*.jsonl): the agenttest harness writes <dir>/agenttest-otlp.jsonl and
// the workspace leg writes <state>/telemetry/otlp/relurpify-YYYY-MM-DD.jsonl.
func collectTelemetryFiles(runDir string) []string {
	var files []string
	_ = filepath.WalkDir(runDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // one unreadable path must not abort the run report
		}
		name := filepath.Base(path)
		if !d.IsDir() && strings.HasSuffix(name, ".jsonl") && strings.Contains(name, "otlp") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// scanSelectionTelemetryFile aggregates the euclo.route.selected records of
// one OTLP-JSON segment file (D11): unparseable content is skipped by the
// reader — a torn line must never abort the whole run report.
func scanSelectionTelemetryFile(path string, summary *selectionSummary) error {
	records, err := otelleg.ReadSegmentLogRecords(path)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.Name != eventRouteSelected {
			continue
		}
		summary.Total++
		if routeID := rec.Attr(attrRouteID); routeID != "" {
			summary.RouteHistogram[routeID]++
		}
		if decidedBy := rec.Attr(attrDecidedBy); decidedBy != "" {
			summary.DecidedByHistogram[decidedBy]++
		}
		if taken, _ := rec.Attributes[attrFallbackTaken].(bool); taken {
			summary.FallbackCount++
		}
		if used, _ := rec.Attributes[attrTier2Used].(bool); used {
			summary.Tier2UsedCount++
		}
		if tier2Rejection(rec.Attr(attrTier2Outcome)) {
			summary.Tier2RejectionCount++
		}
		if degradation := rec.Attr(attrDegradation); degradation != "" {
			summary.DegradationCount++
		}
	}
	return nil
}

// tier2Rejection reports whether a bounded Tier-2 outcome left the
// deterministic winner in place (the model was consulted but did not decide).
func tier2Rejection(outcome string) bool {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "", "applied":
		return false
	case "rejected", "unparseable", "low_confidence", "unavailable":
		return true
	default:
		return false
	}
}

// printSelectionSummary renders the selection decision statistics of a run
// directory.
func printSelectionSummary(out io.Writer, summary *selectionSummary) error {
	if summary == nil {
		return nil
	}
	if err := writef(out, "Selection Decisions:\n"); err != nil {
		return err
	}
	if err := writef(out, "  telemetry files scanned: %d\n", summary.FilesScanned); err != nil {
		return err
	}
	if err := writef(out, "  dispatches: %d\n", summary.Total); err != nil {
		return err
	}
	if summary.Total == 0 {
		return writef(out, "  (no euclo.route.selected events recorded)\n")
	}
	if err := writef(out, "  chosen routes:\n"); err != nil {
		return err
	}
	for _, route := range sortedHistogramKeys(summary.RouteHistogram) {
		if err := writef(out, "    %s: %d\n", route, summary.RouteHistogram[route]); err != nil {
			return err
		}
	}
	if err := writef(out, "  decided by:\n"); err != nil {
		return err
	}
	for _, rule := range sortedHistogramKeys(summary.DecidedByHistogram) {
		if err := writef(out, "    %s: %d\n", rule, summary.DecidedByHistogram[rule]); err != nil {
			return err
		}
	}
	fallbackRate := float64(summary.FallbackCount) / float64(summary.Total) * 100
	tier2Rate := float64(summary.Tier2UsedCount) / float64(summary.Total) * 100
	if err := writef(out, "  fallback rate: %.1f%% (%d/%d)\n", fallbackRate, summary.FallbackCount, summary.Total); err != nil {
		return err
	}
	if err := writef(out, "  tier2 invocation rate: %.1f%% (%d/%d)\n", tier2Rate, summary.Tier2UsedCount, summary.Total); err != nil {
		return err
	}
	if summary.Tier2UsedCount > 0 {
		rejectionRate := float64(summary.Tier2RejectionCount) / float64(summary.Tier2UsedCount) * 100
		if err := writef(out, "  tier2 rejection rate: %.1f%% (%d/%d)\n", rejectionRate, summary.Tier2RejectionCount, summary.Tier2UsedCount); err != nil {
			return err
		}
	}
	if summary.DegradationCount > 0 {
		if err := writef(out, "  default-recipe degradations: %d\n", summary.DegradationCount); err != nil {
			return err
		}
	}
	return nil
}

func sortedHistogramKeys(histogram map[string]int) []string {
	keys := make([]string, 0, len(histogram))
	for key := range histogram {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
