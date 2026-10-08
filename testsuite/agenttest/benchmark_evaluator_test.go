package agenttest

import (
	"strings"
	"testing"
)

func transcriptFor(entries ...ToolTranscriptEntry) *ToolTranscriptArtifact {
	if len(entries) == 0 {
		return nil
	}
	return &ToolTranscriptArtifact{Entries: entries}
}

func TestEvaluateBenchmarkNilSpec(t *testing.T) {
	eval := EvaluateBenchmark(&CaseReport{}, nil, nil)
	if len(eval.Observations) != 0 || len(eval.Failures) != 0 {
		t.Fatalf("expected empty evaluation for nil spec, got %+v", eval)
	}
}

func TestEvaluateBenchmarkToolsExpected(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_read": 2, "go build": 0}}
	spec := &BenchmarkSpec{ToolsExpected: []string{"file_read", "go build"}}
	eval := EvaluateBenchmark(report, spec, nil)

	if len(eval.Observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(eval.Observations))
	}
	if !eval.Observations[0].Matched {
		t.Fatalf("expected file_read to be matched, got %+v", eval.Observations[0])
	}
	if eval.Observations[1].Matched {
		t.Fatalf("expected uninvoked go build not to be matched, got %+v", eval.Observations[1])
	}
	if len(eval.Failures) != 0 {
		t.Fatalf("presence mismatches are advisory: got failures %#v", eval.Failures)
	}
}

func TestEvaluateBenchmarkToolsNotExpectedFails(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_write": 1}}
	eval := EvaluateBenchmark(report, &BenchmarkSpec{ToolsNotExpected: []string{"file_write"}}, nil)
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure when a negative tool assertion is violated")
	}
	if !strings.Contains(eval.Failures[0], "file_write") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}

func TestEvaluateBenchmarkMaxToolCallsThreshold(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_read": 3, "file_write": 2}}
	eval := EvaluateBenchmark(report, &BenchmarkSpec{MaxToolCallsHint: 4}, nil)
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure when max_tool_calls_hint is exceeded")
	}
	if !strings.Contains(eval.Failures[0], "exceed") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}

func TestEvaluateBenchmarkTokenBudget(t *testing.T) {
	report := &CaseReport{TokenUsage: tokenUsage(500, 200)}
	spec := &BenchmarkSpec{TokenBudget: &TokenBudgetHint{MaxTotal: 600, MaxPrompt: 1000}}
	eval := EvaluateBenchmark(report, spec, nil)
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure when total token budget is exceeded")
	}
	for _, failure := range eval.Failures {
		if !strings.Contains(failure, "exceed budget") {
			t.Fatalf("unexpected failure: %q", failure)
		}
	}
}

func TestEvaluateBenchmarkToolSequenceExpected(t *testing.T) {
	transcript := transcriptFor(
		ToolTranscriptEntry{Tool: "file_read", Success: true},
		ToolTranscriptEntry{Tool: "file_write", Success: true},
		ToolTranscriptEntry{Tool: "go build", Success: true},
	)
	// file_read before file_write is satisfied.
	eval := EvaluateBenchmark(&CaseReport{}, &BenchmarkSpec{ToolSequenceExpected: []string{"file_read", "file_write"}}, transcript)
	if len(eval.Observations) != 1 || !eval.Observations[0].Matched {
		t.Fatalf("expected matched sequence observation, got %#v", eval.Observations)
	}

	// file_write before file_read is not satisfied by the transcript.
	eval = EvaluateBenchmark(&CaseReport{}, &BenchmarkSpec{ToolSequenceExpected: []string{"file_write", "file_read"}}, transcript)
	if eval.Observations[0].Matched {
		t.Fatalf("expected mismatched sequence observation, got %+v", eval.Observations[0])
	}
}

func TestEvaluateBenchmarkToolSuccessRate(t *testing.T) {
	transcript := transcriptFor(
		ToolTranscriptEntry{Tool: "go build", Success: true},
		ToolTranscriptEntry{Tool: "go build", Success: true},
		ToolTranscriptEntry{Tool: "go build", Success: false},
	)
	// 2/3 = 66% — meets a 50% expectation, misses a 70% expectation.
	report := &CaseReport{ToolCalls: map[string]int{"go build": 3}}
	eval := EvaluateBenchmark(report, &BenchmarkSpec{ToolSuccessRate: map[string]int{"go build": 50}}, transcript)
	if !eval.Observations[0].Matched {
		t.Fatalf("expected 66%% to meet 50%% expectation, got %+v", eval.Observations[0])
	}
	eval = EvaluateBenchmark(report, &BenchmarkSpec{ToolSuccessRate: map[string]int{"go build": 70}}, transcript)
	if eval.Observations[0].Matched {
		t.Fatalf("expected 66%% to miss 70%% expectation, got %+v", eval.Observations[0])
	}
}

func TestEvaluateBenchmarkToolCallLatencyThreshold(t *testing.T) {
	report := &CaseReport{ToolLatencies: map[string]LatencyStats{"go build": {MaxMs: 2500}}}
	eval := EvaluateBenchmark(report, &BenchmarkSpec{ToolCallLatencyMs: map[string]int{"go build": 2000}}, nil)
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure when tool latency exceeds the threshold")
	}
	if !strings.Contains(eval.Failures[0], "latency") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}

func TestEvaluateBenchmarkToolRecoveryObserved(t *testing.T) {
	transcript := transcriptFor(
		ToolTranscriptEntry{Tool: "go build", Success: false},
		ToolTranscriptEntry{Tool: "go build", Success: true},
	)
	eval := EvaluateBenchmark(&CaseReport{}, &BenchmarkSpec{ToolRecoveryObserved: true}, transcript)
	if len(eval.Observations) == 0 || !eval.Observations[0].Matched {
		t.Fatalf("expected recovery to be observed, got %#v", eval.Observations)
	}
}

func tokenUsage(prompt, completion int) TokenUsageReport {
	return TokenUsageReport{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		LLMCalls:         1,
	}
}
