package intake

import "context"

// ClassifyCapabilityWithLLM uses an LLM to select a single capability ID based on the instruction,
// family, streamed context, and negative constraints.
func ClassifyCapabilityWithLLM(ctx context.Context, model LanguageModel, instruction, familyID, streamedContext string, negativeConstraints []string) (string, string, error) {
	if model == nil {
		return "", "", nil
	}
	classifier := NewLLMCapabilityClassifier(model)
	return classifier.Classify(ctx, instruction, familyID, streamedContext, negativeConstraints)
}
