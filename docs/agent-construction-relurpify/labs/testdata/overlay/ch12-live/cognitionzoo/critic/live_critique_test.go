//go:build live

package critic_test

// Live-model proof for the critic paradigm (book 2, Chapter 12). These
// tests are excluded from normal builds; run with:
//
//	go test ./cognitionzoo/critic/ -tags live -count=1 -run Live -v
//
// They speak to a real local model (ollama, gemma4:e4b — the book's
// model), so the only deterministic thing under test is the framework's
// handling of whatever comes back: strict decode, the retry, the honest
// close. The delegate is a recording double — the live surface under
// study is the critique phase's verdict, not the drafter.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"

	// The ollama provider kind registers from its package init — the
	// composition root imports it in production; a live test imports it
	// directly.
	_ "codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

// liveGemma4 returns the live model behind the book's endpoint, or skips
// the test when the backend is not reachable — a lab machine without
// Ollama should skip, not fail.
func liveGemma4(t *testing.T) model.LanguageModel {
	t.Helper()
	backend, err := llm.New(llm.ProviderConfig{
		Provider: "ollama",
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
		Model:    "gemma4:e4b",
		Timeout:  90 * time.Second,
	}, llm.ProviderSecrets{})
	if err != nil {
		t.Skipf("live backend unavailable: %v", err)
	}
	if err := pingLive(t, backend.Model()); err != nil {
		t.Skipf("live model not reachable: %v", err)
	}
	return backend.Model()
}

func pingLive(t *testing.T, m model.LanguageModel) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := m.GenerateStream(ctx, "ping", nil)
	if err != nil {
		return err
	}
	for range ch {
	}
	return nil
}

// runLiveCritique executes one directive-mode critique against the live
// model and classifies the outcome: a decoded verdict, a decode failure
// (the model's JSON was not valid), or a framework error.
func runLiveCritique(t *testing.T, m model.LanguageModel) (verdict string, decodeFailed bool, err error) {
	t.Helper()
	delegate := &recordingDelegate{}
	cfg := &execution.Config{Model: "gemma4:e4b", Telemetry: nil}
	deps := &paradigm.Deps{Model: m, Config: cfg, PermissionChecker: allowAllChecker()}
	agent := critic.New(deps, delegate, critic.WithCritiqueCriteria(
		"Judge correctness, error handling, and naming. The material is a small Python slugify."))

	env := contextdata.NewEnvelope("live-critic", "live-session")
	started := time.Now()
	_, execErr := agent.Execute(context.Background(), &execution.Task{ID: "live", Instruction: "critique the draft"}, env)
	elapsed := time.Since(started)

	if execErr != nil {
		if strings.Contains(execErr.Error(), "not a valid verdict") || strings.Contains(execErr.Error(), "verdict") {
			return "", true, nil // the model spoke, the decoder refused — recorded, not hidden
		}
		return "", false, execErr
	}
	v, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict")
	t.Logf("live critique: verdict=%q in %s", v, elapsed.Truncate(time.Millisecond))
	return v, false, nil
}

// TestLiveCritiqueUnderGemma4 runs the critique phase N times against the
// real model and tallies honestly: verdict distribution, decode failures,
// wall time. The hard assertions are the framework's own honesty: every
// run closes, and at least one verdict decodes. The distribution itself
// is the chapter's flake-tier evidence, logged not asserted.
func TestLiveCritiqueUnderGemma4(t *testing.T) {
	m := liveGemma4(t)

	const runs = 5
	tally := map[string]int{}
	decodeFailures := 0
	for i := 0; i < runs; i++ {
		verdict, decodeFailed, err := runLiveCritique(t, m)
		if err != nil {
			t.Fatalf("run %d: framework error: %v", i+1, err)
		}
		if decodeFailed {
			decodeFailures++
			tally["<invalid-json>"]++
			continue
		}
		switch verdict {
		case "pass", "issues", "fail":
			tally[verdict]++
		default:
			t.Fatalf("run %d: impossible verdict %q escaped strict decode", i+1, verdict)
		}
	}

	t.Logf("verdict tally over %d runs: %s (decode failures: %d)", runs, formatTally(tally), decodeFailures)
	if decodeFailures == runs {
		t.Fatal("every live critique failed strict decode — the model never produced a valid verdict object")
	}
	// Flake-tier honesty (chapter thesis): a nonzero decode-failure rate
	// classifies the live case live-flaky, not stable. Logged as the
	// classification the tally supports.
	if decodeFailures > 0 {
		t.Logf("classification: live-flaky (%d/%d runs failed strict verdict decode)", decodeFailures, runs)
	} else {
		t.Logf("classification: stable (all %d verdicts decoded)", runs)
	}
}

// TestLiveCritiqueDeterminismProbe asks the same critique twice and
// compares. Temperature is 0; a small local model under no load is
// *mostly* deterministic — this probe demonstrates the "mostly": the
// framework treats any divergence as data, never as an error.
func TestLiveCritiqueDeterminismProbe(t *testing.T) {
	m := liveGemma4(t)

	first, firstFailed, err := runLiveCritique(t, m)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, secondFailed, err := runLiveCritique(t, m)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	switch {
	case firstFailed || secondFailed:
		t.Logf("probe: at least one run failed decode (%v/%v) — nondeterminism observed at the format layer", firstFailed, secondFailed)
	case first == second:
		t.Logf("probe: identical verdicts (%q) — deterministic under zero temperature", first)
	default:
		t.Logf("probe: verdicts differ (%q vs %q) at temperature 0 — real-model nondeterminism, recorded not punished", first, second)
	}
}

func formatTally(tally map[string]int) string {
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, tally[k]))
	}
	return strings.Join(parts, " ")
}
