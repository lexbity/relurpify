package plan

import "codeburg.org/lexbit/relurpify/framework/retrieval"

type ConfidenceDegradation struct {
	AnchorDriftPenalty   float64
	SymbolMissingPenalty float64
	Threshold            float64
}

func DefaultConfidenceDegradation() ConfidenceDegradation {
	return ConfidenceDegradation{
		AnchorDriftPenalty:   0.2,
		SymbolMissingPenalty: 0.3,
		Threshold:            0.4,
	}
}

func RecalculateConfidence(step *PlanStep, driftedAnchors []string, missingSymbols []string, cfg ConfidenceDegradation) float64 {
	if step == nil {
		return 0
	}
	score := step.ConfidenceScore
	score -= float64(len(driftedAnchors)) * cfg.AnchorDriftPenalty
	score -= float64(len(missingSymbols)) * cfg.SymbolMissingPenalty
	if score < 0 {
		return 0
	}
	return score
}

func EvidenceGateAllows(gate *EvidenceGate, evidence retrieval.MixedEvidenceResult, activeAnchors map[string]bool, availableSymbols map[string]bool) bool {
	if gate == nil {
		return true
	}
	for _, anchor := range gate.RequiredAnchors {
		if !activeAnchors[anchor] {
			return false
		}
	}
	for _, symbol := range gate.RequiredSymbols {
		if !availableSymbols[symbol] {
			return false
		}
	}
	if gate.MaxTotalLoss > 0 && evidence.Derivation != nil && evidence.Derivation.TotalLoss() > gate.MaxTotalLoss {
		return false
	}
	return true
}
