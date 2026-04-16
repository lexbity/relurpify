package intake

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// ClassifyCapabilityIntent performs capability-level classification using Tier 1 (static keywords),
// Tier 2 (LLM semantic), and Tier 3 (fallback). Result is returned directly; callers are
// responsible for persisting to state.
// This is extracted from agent_state_helpers.go classifyCapabilityIntent.
func ClassifyCapabilityIntent(
	ctx context.Context,
	task *core.Task,
	instruction string,
	modeID string,
	classifier CapabilityClassifier,
) (eucloruntime.CapabilityClassificationResult, error) {
	if classifier == nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("classifier not available")
	}

	seq, op, err := classifier.Classify(ctx, instruction, modeID)
	if err != nil {
		return eucloruntime.CapabilityClassificationResult{}, fmt.Errorf("euclo capability classification: %w", err)
	}

	return eucloruntime.CapabilityClassificationResult{
		Sequence: seq,
		Operator: op,
		Source:   "classifier",
		Meta:     "", // Could be enriched with match details
	}, nil
}

// DefaultCapabilityClassifier creates a default classifier using the relurpic registry.
func DefaultCapabilityClassifier(registry *euclorelurpic.Registry, model core.LanguageModel, extraKeywords map[string][]string) CapabilityClassifier {
	if registry == nil {
		registry = euclorelurpic.DefaultRegistry()
	}
	return &defaultClassifier{
		registry:      registry,
		model:         model,
		extraKeywords: extraKeywords,
	}
}

type defaultClassifier struct {
	registry      *euclorelurpic.Registry
	model         core.LanguageModel
	extraKeywords map[string][]string
}

func (d *defaultClassifier) Classify(ctx context.Context, instruction, modeID string) ([]string, string, error) {
	classifier := &eucloruntime.CapabilityIntentClassifier{
		Registry:      d.registry,
		ExtraKeywords: d.extraKeywords,
		Model:         d.model,
	}

	result, err := classifier.Classify(ctx, instruction, modeID)
	if err != nil {
		return nil, "", err
	}

	return result.Sequence, result.Operator, nil
}
