package intake

import (
	"context"
	"strings"
	"testing"
)

func TestLLMCapabilityClassifierClassifyUsesPromptAndParsesResponse(t *testing.T) {
	model := &recordingLanguageModel{
		response: CompletionResponse{
			Text: "prefix {\"capability_id\":\"euclo:cap.bisect\"} suffix",
		},
	}

	classifier := NewLLMCapabilityClassifier(model)
	capabilityID, source, err := classifier.Classify(
		context.Background(),
		"fix the failing test",
		"debug",
		"streamed context",
		[]string{"do not modify the API"},
	)
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}

	if source != "llm" {
		t.Fatalf("expected llm source, got %q", source)
	}
	if capabilityID != "euclo:cap.bisect" {
		t.Fatalf("unexpected capability id: %q", capabilityID)
	}

	if model.lastRequest.Prompt == "" {
		t.Fatal("expected prompt to be recorded")
	}
	for _, want := range []string{
		"Task: fix the failing test",
		"Winning family: debug",
		"Context:",
		"streamed context",
		"Constraints (DO NOT use capabilities that violate these):",
		"do not modify the API",
		`{"capability_id": "cap_id"}`,
	} {
		if !strings.Contains(model.lastRequest.Prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, model.lastRequest.Prompt)
		}
	}
}

func TestLLMCapabilityClassifierParseResponseErrors(t *testing.T) {
	classifier := NewLLMCapabilityClassifier(&recordingLanguageModel{})

	if _, _, err := classifier.parseResponse("no json here"); err == nil {
		t.Fatal("expected error for missing JSON")
	}

	if _, _, err := classifier.parseResponse("{\"capability_id\":\"\"}"); err == nil {
		t.Fatal("expected error for empty capability id")
	}

	if _, _, err := classifier.parseResponse("{bad json}"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLLMCapabilityClassifierClassifyWithoutModel(t *testing.T) {
	classifier := NewLLMCapabilityClassifier(nil)
	if _, _, err := classifier.Classify(context.Background(), "instruction", "debug", "", nil); err == nil {
		t.Fatal("expected error when no model is configured")
	}
}

func TestClassifyCapabilityWithLLMNoModel(t *testing.T) {
	capabilityID, source, err := ClassifyCapabilityWithLLM(context.Background(), nil, "instruction", "debug", "", nil)
	if err != nil {
		t.Fatalf("expected nil error when model is nil, got %v", err)
	}
	if capabilityID != "" || source != "" {
		t.Fatalf("expected empty capability and source, got %q %q", capabilityID, source)
	}
}

type recordingLanguageModel struct {
	lastRequest CompletionRequest
	response    CompletionResponse
	err         error
}

func (m *recordingLanguageModel) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.lastRequest = req
	if m.err != nil {
		return CompletionResponse{}, m.err
	}
	if m.response.Text == "" {
		return CompletionResponse{Text: "{\"capability_id\":\"fallback\"}"}, nil
	}
	return m.response, nil
}
