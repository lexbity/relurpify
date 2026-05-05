// Package intake provides functions for processing user requests.
package intake

import (
    "context"
)

// ClassifyCapabilitiesWithLLM uses an LLM to select capabilities based on the instruction, family, streamed context, and negative constraints.
// If model is nil, it returns an empty slice and "" operator.
func ClassifyCapabilitiesWithLLM(ctx context.Context, model LanguageModel, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error) {
    if model == nil {
        return nil, "", nil
    }
    classifier := NewLLMCapabilityClassifier(model)
    return classifier.Classify(ctx, instruction, familyID, streamedContext, negativeConstraints)
}
