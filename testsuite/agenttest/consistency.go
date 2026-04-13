package agenttest

import (
	"fmt"
	"math"
)

// ConsistencyReport aggregates results from multiple runs
type ConsistencyReport struct {
	Runs                int     `json:"runs"`
	SuccessRate         float64 `json:"success_rate"`
	ToolCountVariance   float64 `json:"tool_count_variance"`
	FingerprintDistance float64 `json:"fingerprint_distance"`
	DeterminismScore    float64 `json:"determinism_score"`
}

// ComputeConsistency performs statistical analysis across multiple run reports
func ComputeConsistency(reports []CaseReport) *ConsistencyReport {
	if len(reports) == 0 {
		return nil
	}

	report := &ConsistencyReport{
		Runs: len(reports),
	}

	// Calculate success rate
	successes := 0
	for _, r := range reports {
		if r.Success {
			successes++
		}
	}
	report.SuccessRate = float64(successes) / float64(len(reports))

	// Calculate tool count variance
	toolCounts := make([]int, len(reports))
	for i, r := range reports {
		total := 0
		for _, count := range r.ToolCalls {
			total += count
		}
		toolCounts[i] = total
	}
	report.ToolCountVariance = computeVariance(toolCounts)

	return report
}

// ComputeConsistencyWithFingerprints extends analysis with fingerprint comparison
func ComputeConsistencyWithFingerprints(reports []CaseReport, fingerprints []*ToolSequenceFingerprint) *ConsistencyReport {
	report := ComputeConsistency(reports)
	if report == nil || len(fingerprints) < 2 {
		return report
	}

	// Calculate average fingerprint distance between all pairs
	var totalDistance float64
	var pairCount int

	for i := 0; i < len(fingerprints); i++ {
		for j := i + 1; j < len(fingerprints); j++ {
			dist := FingerprintDistance(fingerprints[i], fingerprints[j])
			totalDistance += dist
			pairCount++
		}
	}

	if pairCount > 0 {
		report.FingerprintDistance = totalDistance / float64(pairCount)
		report.DeterminismScore = DeterminismScore(report.FingerprintDistance)
	}

	return report
}

// computeVariance calculates population variance of a slice of ints
func computeVariance(values []int) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Calculate mean
	sum := 0
	for _, v := range values {
		sum += v
	}
	mean := float64(sum) / float64(len(values))

	// Calculate variance
	var variance float64
	for _, v := range values {
		diff := float64(v) - mean
		variance += diff * diff
	}
	variance = variance / float64(len(values))

	return variance
}

// computeStandardDeviation calculates standard deviation
func computeStandardDeviation(values []int) float64 {
	return math.Sqrt(computeVariance(values))
}

// VarianceReport provides detailed variance analysis
type VarianceReport struct {
	Mean          float64 `json:"mean"`
	Variance      float64 `json:"variance"`
	StdDev        float64 `json:"std_dev"`
	Min           int     `json:"min"`
	Max           int     `json:"max"`
	Range         int     `json:"range"`
	CoefficientOfVariation float64 `json:"cv"`
}

// AnalyzeToolCallVariance provides detailed analysis of tool call variance
func AnalyzeToolCallVariance(reports []CaseReport) VarianceReport {
	if len(reports) == 0 {
		return VarianceReport{}
	}

	// Collect total tool call counts
	toolCounts := make([]int, len(reports))
	for i, r := range reports {
		total := 0
		for _, count := range r.ToolCalls {
			total += count
		}
		toolCounts[i] = total
	}

	// Calculate statistics
	min := toolCounts[0]
	max := toolCounts[0]
	sum := 0

	for _, v := range toolCounts {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}

	mean := float64(sum) / float64(len(toolCounts))
	variance := computeVariance(toolCounts)
	stdDev := math.Sqrt(variance)

	// Coefficient of variation (stdDev / mean), useful for comparing variance across different scales
	cv := 0.0
	if mean > 0 {
		cv = stdDev / mean
	}

	return VarianceReport{
		Mean:                   mean,
		Variance:               variance,
		StdDev:                 stdDev,
		Min:                    min,
		Max:                    max,
		Range:                  max - min,
		CoefficientOfVariation: cv,
	}
}

// FormatConsistencyReport generates a human-readable consistency report
func FormatConsistencyReport(report *ConsistencyReport) string {
	if report == nil {
		return "Consistency Report: nil"
	}

	out := "Consistency Report\n"
	out += "====================\n"
	out += fmt.Sprintf("Runs: %d\n", report.Runs)
	out += fmt.Sprintf("Success Rate: %.1f%%\n", report.SuccessRate*100)
	out += fmt.Sprintf("Tool Count Variance: %.2f\n", report.ToolCountVariance)
	out += fmt.Sprintf("Fingerprint Distance: %.3f\n", report.FingerprintDistance)
	out += fmt.Sprintf("Determinism Score: %.2f%%\n", report.DeterminismScore*100)

	// Interpretation
	if report.DeterminismScore >= 0.9 {
		out += "Interpretation: Highly deterministic (>=90% consistent)\n"
	} else if report.DeterminismScore >= 0.7 {
		out += "Interpretation: Moderately deterministic (70-90% consistent)\n"
	} else if report.DeterminismScore >= 0.5 {
		out += "Interpretation: Low determinism (50-70% consistent)\n"
	} else {
		out += "Interpretation: Non-deterministic (<50% consistent)\n"
	}

	return out
}

// IsDeterministic checks if a determinism score meets a threshold
func IsDeterministic(score float64, threshold string) bool {
	// Parse threshold like ">0.9" or "0.8"
	var target float64
	var op string

	if _, err := fmt.Sscanf(threshold, ">%f", &target); err == nil {
		op = ">"
	} else if _, err := fmt.Sscanf(threshold, ">=%f", &target); err == nil {
		op = ">="
	} else {
		// Default to >= for bare numbers
		fmt.Sscanf(threshold, "%f", &target)
		op = ">="
	}

	switch op {
	case ">":
		return score > target
	case ">=":
		return score >= target
	default:
		return score >= target
	}
}
