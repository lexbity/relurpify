package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ClassificationResponse is the expected LLM response structure.
type ClassificationResponse struct {
	Predicates  []string `json:"predicates"`
	Confidence  float32  `json:"confidence"`
	Reasoning   string   `json:"reasoning"`
	Ambiguities []string `json:"ambiguities,omitempty"`
}

// buildClassificationPrompt constructs a prompt for goal analysis.
func buildClassificationPrompt(instruction string, availablePredicates []string) string {
	predicateList := "file_content_known, edit_plan_known, file_modified, test_result_known, relevant_symbols_known"
	if len(availablePredicates) > 0 {
		predicateList = strings.Join(availablePredicates, ", ")
	}

	prompt := fmt.Sprintf(`You are a task planning assistant. Analyze the following task instruction and determine which world-state predicates must be satisfied to complete it.

Available predicates:
%s

Task instruction: "%s"

Respond with ONLY a JSON object (no markdown, no explanation) with this exact structure:
{
  "predicates": ["predicate1", "predicate2"],
  "confidence": 0.95,
  "reasoning": "brief explanation of why these predicates are needed",
  "ambiguities": ["any unclear aspects of the task"]
}

Rules:
1. Use only predicates from the available list.
2. Order predicates in logical dependency order (prerequisites first).
3. Confidence: 0.0-1.0 (1.0 = very clear, 0.5 = ambiguous).
4. If confidence < 0.6, populate ambiguities with clarifying questions.
5. If the task is very simple (e.g., just reading), return minimal predicates.
6. If the task requires modification and testing, include all relevant stages.
`, predicateList, instruction)

	return prompt
}

// ParseClassificationResponse extracts predicates from LLM JSON response.
func ParseClassificationResponse(rawResponse string) (*ClassificationResponse, error) {
	// Try to extract JSON from response (LLM might include markdown or extra text)
	jsonStart := strings.Index(rawResponse, "{")
	jsonEnd := strings.LastIndex(rawResponse, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonStart >= jsonEnd {
		return nil, fmt.Errorf("no JSON found in response: %s", rawResponse)
	}

	jsonStr := rawResponse[jsonStart : jsonEnd+1]
	var resp ClassificationResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal classification response: %w", err)
	}

	if len(resp.Predicates) == 0 {
		return nil, fmt.Errorf("no predicates in response")
	}

	return &resp, nil
}

// classifyWithLLM invokes the language model for goal classification.
func classifyWithLLM(model contracts.LanguageModel, instruction string, availablePredicates []string) (*ClassificationResponse, error) {
	if model == nil {
		return nil, fmt.Errorf("language model is nil")
	}

	prompt := buildClassificationPrompt(instruction, availablePredicates)

	// Invoke model with default options
	resp, err := model.Generate(context.Background(), prompt, &contracts.LLMOptions{})
	if err != nil {
		return nil, fmt.Errorf("model.Generate failed: %w", err)
	}

	if resp == nil || resp.Text == "" {
		return nil, fmt.Errorf("empty response from language model")
	}

	return ParseClassificationResponse(resp.Text)
}

// PredicatesFromRegistry extracts all unique predicates from an operator registry.
func PredicatesFromRegistry(reg *types.OperatorRegistry) []string {
	if reg == nil {
		return nil
	}

	seen := make(map[string]bool)
	var predicates []string

	for _, op := range reg.All() {
		for _, pre := range op.Preconditions {
			if !seen[string(pre)] {
				seen[string(pre)] = true
				predicates = append(predicates, string(pre))
			}
		}
		for _, eff := range op.Effects {
			if !seen[string(eff)] {
				seen[string(eff)] = true
				predicates = append(predicates, string(eff))
			}
		}
	}

	return predicates
}
