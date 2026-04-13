package agenttest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LatencyStats captures statistical information about tool latency
type LatencyStats struct {
	MinMs int64 `json:"min_ms"`
	MaxMs int64 `json:"max_ms"`
	AvgMs int64 `json:"avg_ms"`
	P95Ms int64 `json:"p95_ms"`
}

// ToolLatencyReport contains latency information for all tools
type ToolLatencyReport struct {
	ToolLatencies   map[string]LatencyStats `json:"tool_latencies"`
	TotalToolTimeMs int64                   `json:"total_tool_time_ms"`
}

// EvaluateLatencyExpectations checks if latency constraints are met
func EvaluateLatencyExpectations(expect ExpectSpec, transcript *ToolTranscriptArtifact) []string {
	var failures []string

	if transcript == nil {
		return nil
	}

	// Check per-tool latency constraints
	for tool, constraint := range expect.ToolCallLatencyMs {
		maxMs, err := parseLatencyConstraint(constraint)
		if err != nil {
			failures = append(failures, fmt.Sprintf("invalid latency constraint for %s: %s", tool, err))
			continue
		}

		actualMax := computeMaxLatency(transcript, tool)
		if actualMax > maxMs {
			failures = append(failures,
				fmt.Sprintf("tool %s max latency %dms exceeds constraint %dms", tool, actualMax, maxMs))
		}
	}

	// Check total tool time constraint
	if expect.MaxTotalToolTimeMs > 0 {
		total := computeTotalToolTime(transcript)
		if total > int64(expect.MaxTotalToolTimeMs) {
			failures = append(failures,
				fmt.Sprintf("total tool time %dms exceeds maximum %dms", total, expect.MaxTotalToolTimeMs))
		}
	}

	return failures
}

// parseLatencyConstraint parses a latency constraint like "<50" or "100"
// Returns the max allowed milliseconds
func parseLatencyConstraint(constraint string) (int64, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return 0, fmt.Errorf("empty constraint")
	}

	// Remove comparison operators
	constraint = strings.TrimPrefix(constraint, "<")
	constraint = strings.TrimPrefix(constraint, ">")
	constraint = strings.TrimPrefix(constraint, "<=")
	constraint = strings.TrimPrefix(constraint, ">=")
	constraint = strings.TrimPrefix(constraint, "=")
	constraint = strings.TrimSpace(constraint)

	// Parse the number
	val, err := strconv.ParseInt(constraint, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", constraint)
	}

	return val, nil
}

// computeMaxLatency finds the maximum latency for a specific tool
func computeMaxLatency(transcript *ToolTranscriptArtifact, tool string) int64 {
	if transcript == nil {
		return 0
	}

	tool = strings.TrimSpace(tool)
	var max int64 = 0

	for _, entry := range transcript.Entries {
		if strings.TrimSpace(entry.Tool) != tool {
			continue
		}
		if entry.DurationMS > max {
			max = entry.DurationMS
		}
	}

	return max
}

// computeTotalToolTime sums all tool execution times
func computeTotalToolTime(transcript *ToolTranscriptArtifact) int64 {
	if transcript == nil {
		return 0
	}

	var total int64 = 0
	for _, entry := range transcript.Entries {
		total += entry.DurationMS
	}
	return total
}

// BuildLatencyReport creates a comprehensive latency report from a transcript
func BuildLatencyReport(transcript *ToolTranscriptArtifact) *ToolLatencyReport {
	if transcript == nil {
		return nil
	}

	report := &ToolLatencyReport{
		ToolLatencies:   make(map[string]LatencyStats),
		TotalToolTimeMs: computeTotalToolTime(transcript),
	}

	// Group latencies by tool
	toolLatencies := make(map[string][]int64)
	for _, entry := range transcript.Entries {
		tool := strings.TrimSpace(entry.Tool)
		if tool != "" && entry.DurationMS > 0 {
			toolLatencies[tool] = append(toolLatencies[tool], entry.DurationMS)
		}
	}

	// Compute stats for each tool
	for tool, latencies := range toolLatencies {
		report.ToolLatencies[tool] = computeLatencyStats(latencies)
	}

	return report
}

// computeLatencyStats calculates min, max, avg, and p95 from a slice of latencies
func computeLatencyStats(latencies []int64) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}

	// Sort for percentile calculation
	sorted := make([]int64, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Calculate min and max
	min := sorted[0]
	max := sorted[len(sorted)-1]

	// Calculate average
	var sum int64 = 0
	for _, v := range sorted {
		sum += v
	}
	avg := sum / int64(len(sorted))

	// Calculate P95
	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	p95 := sorted[p95Index]

	return LatencyStats{
		MinMs: min,
		MaxMs: max,
		AvgMs: avg,
		P95Ms: p95,
	}
}

// computePercentile calculates the nth percentile from sorted data
func computePercentile(sorted []int64, percentile float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 100 {
		return sorted[len(sorted)-1]
	}

	index := int(float64(len(sorted)-1) * percentile / 100.0)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// FormatLatencyReport generates a human-readable latency report
func FormatLatencyReport(report *ToolLatencyReport) string {
	if report == nil {
		return "Latency Report: nil"
	}

	out := "Latency Report\n"
	out += "===============\n"
	out += fmt.Sprintf("Total Tool Time: %dms\n\n", report.TotalToolTimeMs)

	if len(report.ToolLatencies) == 0 {
		out += "No tool latency data recorded.\n"
		return out
	}

	out += "Per-Tool Statistics:\n"
	for tool, stats := range report.ToolLatencies {
		out += fmt.Sprintf("  %s: min=%dms, avg=%dms, p95=%dms, max=%dms\n",
			tool, stats.MinMs, stats.AvgMs, stats.P95Ms, stats.MaxMs)
	}

	return out
}

// CheckLatencyAgainstBaseline checks if current latency exceeds baseline by threshold
func CheckLatencyAgainstBaseline(current, baseline LatencyStats, tool string, threshold float64) []string {
	var warnings []string

	if baseline.MaxMs <= 0 {
		return nil
	}

	// Check if current max exceeds 2x baseline max (default threshold for warning)
	if threshold <= 0 {
		threshold = 2.0
	}

	limit := int64(float64(baseline.MaxMs) * threshold)

	if current.MaxMs > limit {
		warnings = append(warnings,
			fmt.Sprintf("tool %s max latency %dms exceeds %dx baseline (baseline max: %dms)",
				tool, current.MaxMs, int64(threshold), baseline.MaxMs))
	}

	return warnings
}

// GetQueueTimeStats extracts queue time statistics from transcript
func GetQueueTimeStats(transcript *ToolTranscriptArtifact, tool string) LatencyStats {
	if transcript == nil {
		return LatencyStats{}
	}

	tool = strings.TrimSpace(tool)
	var queueTimes []int64

	for _, entry := range transcript.Entries {
		if strings.TrimSpace(entry.Tool) == tool && entry.QueueTimeMS > 0 {
			queueTimes = append(queueTimes, entry.QueueTimeMS)
		}
	}

	return computeLatencyStats(queueTimes)
}
