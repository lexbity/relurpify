package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/model"

	"codeburg.org/lexbit/relurpify/telemetry"
)

type stubModel struct {
	streamText string
}

func (s stubModel) Generate(context.Context, string, *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}
func (s stubModel) GenerateStream(context.Context, string, *LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- s.streamText
	close(ch)
	return ch, nil
}
func (s stubModel) Chat(context.Context, []Message, *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "chat", FinishReason: "stop"}, nil
}
func (s stubModel) ChatWithTools(context.Context, []Message, []LLMToolSpec, *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "tools", FinishReason: "stop", ToolCalls: []model.ToolCall{{Name: "file_read", Args: map[string]any{"path": "x"}}}}, nil
}

func TestTapeModelRecordThenReplay(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	tape := filepath.Join(dir, "tape.jsonl")

	rec, err := NewTapeModel(stubModel{streamText: "streamed"}, tape, "record")
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.ConfigureHeader(TapeHeader{
		ModelName:   "m",
		ModelDigest: "sha256:abc123",
		SuiteName:   "suite",
		CaseName:    "case",
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Close() }()

	if _, err := rec.Generate(context.Background(), "p", &LLMOptions{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	stream, err := rec.GenerateStream(context.Background(), "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !fileExists(tape) {
		t.Fatalf("expected tape file at %s", tape)
	}

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.ConfigureHeader(TapeHeader{
		ModelName:   "m",
		ModelDigest: "sha256:abc123",
	}); err != nil {
		t.Fatal(err)
	}
	if resp, err := replay.Generate(context.Background(), "p", &LLMOptions{Model: "m"}); err != nil || resp.Text != "ok" {
		t.Fatalf("replay generate: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil || resp.Text != "chat" {
		t.Fatalf("replay chat: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil || resp.Text != "tools" {
		t.Fatalf("replay chat_with_tools: resp=%+v err=%v", resp, err)
	}
	ch, err := replay.GenerateStream(context.Background(), "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Stream response text is not preserved in the tape when recording
	// synchronously to maintain correct call order. The channel is
	// closed immediately with no data.
	for range ch {
	}
}

func TestTapeModelRecordWritesHeaderFirst(t *testing.T) {
	dir := t.TempDir()
	tape := filepath.Join(dir, "tape.jsonl")

	rec, err := NewTapeModel(stubModel{}, tape, "record")
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.ConfigureHeader(TapeHeader{
		ModelName:   "model-a",
		ModelDigest: "sha256:deadbeef",
		SuiteName:   "suite-a",
		CaseName:    "case-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Generate(context.Background(), "prompt", nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(filepath.Clean(tape))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected header line")
	}
	var entry tapeEntry
	if err := json.Unmarshal(sc.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != "_header" {
		t.Fatalf("expected first entry to be header, got %q", entry.Kind)
	}
	if entry.Request.Header == nil || entry.Request.Header.ModelName != "model-a" {
		t.Fatalf("unexpected header payload: %+v", entry.Request.Header)
	}
}

func TestTapeModelReplayWarnsOnProviderMismatch(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{Kind: "_header", ProviderID: "ollama", ModelName: "model-a"}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.ConfigureHeader(TapeHeader{ProviderID: "lmstudio", ModelName: "model-a"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `tape recorded with provider "ollama"`) {
		t.Fatalf("expected provider mismatch warning, got %q", buf.String())
	}
}

func TestTapeModelReplayRejectsMismatchedModelHeader(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{Kind: "_header", ModelName: "model-a"}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	err = replay.ConfigureHeader(TapeHeader{ModelName: "model-b"})
	if err == nil || !strings.Contains(err.Error(), `recorded with model "model-a"`) {
		t.Fatalf("expected model mismatch error, got %v", err)
	}
}

func TestTapeModelReplayRejectsMismatchedDigestHeader(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{Kind: "_header", ModelName: "model-a", ModelDigest: "sha256:abc123456789"}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	err = replay.ConfigureHeader(TapeHeader{ModelName: "model-a", ModelDigest: "sha256:fff123456789"})
	if err == nil || !strings.Contains(err.Error(), "model digest") {
		t.Fatalf("expected digest mismatch error, got %v", err)
	}
}

func TestTapeModelReplayAllowsLegacyTapeWithoutHeader(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.ConfigureHeader(TapeHeader{ModelName: "model-a"}); err != nil {
		t.Fatalf("expected legacy tape header warning path, got error %v", err)
	}
	resp, err := replay.Generate(context.Background(), "p", nil)
	if err != nil || resp.Text != "ok" {
		t.Fatalf("expected legacy tape replay to work, resp=%+v err=%v", resp, err)
	}
}

func TestTapeModelReplayRejectsMismatchedFirstRequest(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{
			Kind:       "_header",
			ModelName:  "model-a",
			SuiteName:  "testsuite/agenttests/euclo.code.testsuite.yaml",
			CaseName:   "basic_edit_task",
			RecordedAt: time.Now().UTC().Format(time.RFC3339),
		}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "old prompt"}), Response: &LLMResponse{Text: "ok"}},
	})

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.ConfigureHeader(TapeHeader{ModelName: "model-a"}); err != nil {
		t.Fatal(err)
	}
	_, err = replay.Generate(context.Background(), "new prompt", nil)
	if err == nil || !strings.Contains(err.Error(), "first request fingerprint mismatch") || !strings.Contains(err.Error(), "agenttest refresh --suite testsuite/agenttests/euclo.code.testsuite.yaml --case basic_edit_task") {
		t.Fatalf("expected clear first-request mismatch error, got %v", err)
	}
}

func TestTapeModelReplayWarnsOnStaleTapeAge(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{
			Kind:       "_header",
			ModelName:  "model-a",
			SuiteName:  "testsuite/agenttests/euclo.code.testsuite.yaml",
			CaseName:   "basic_edit_task",
			RecordedAt: time.Now().UTC().Add(-45 * 24 * time.Hour).Format(time.RFC3339),
		}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	var buf bytes.Buffer
	prevWriter := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prevWriter)

	replay, err := NewTapeModel(stubModel{}, tape, "replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.ConfigureHeader(TapeHeader{ModelName: "model-a"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "was recorded 45 days ago") {
		t.Fatalf("expected age warning, got %q", buf.String())
	}
}

func TestInspectTapeReturnsHeaderAndRecordedAt(t *testing.T) {
	recordedAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{
			Kind:       "_header",
			ModelName:  "model-a",
			SuiteName:  "suite-a",
			CaseName:   "case-a",
			RecordedAt: recordedAt,
		}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &LLMResponse{Text: "ok"}},
	})

	inspection, err := InspectTape(tape)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Header == nil || inspection.Header.ModelName != "model-a" {
		t.Fatalf("unexpected inspection header: %+v", inspection.Header)
	}
	if inspection.FirstEntryKind != "generate" {
		t.Fatalf("unexpected first entry kind: %q", inspection.FirstEntryKind)
	}
	if inspection.FirstRecordedAt.IsZero() {
		t.Fatal("expected recorded time")
	}
}

func writeTapeFixture(t *testing.T, entries []tapeEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tape.jsonl")
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// blockingStreamModel emits stream tokens only when poked via emit, and
// stops (closing its channel) when its own context is cancelled. It gives
// tests deterministic control over mid-stream state.
type blockingStreamModel struct {
	tokens chan string
	done   chan struct{}
}

func newBlockingStreamModel() *blockingStreamModel {
	return &blockingStreamModel{tokens: make(chan string), done: make(chan struct{})}
}

func (b *blockingStreamModel) emit(tok string) { b.tokens <- tok }

func (b *blockingStreamModel) Generate(ctx context.Context, _ string, _ *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (b *blockingStreamModel) GenerateStream(ctx context.Context, _ string, _ *LLMOptions) (<-chan string, error) {
	out := make(chan string)
	go func() {
		defer close(out)
		for {
			select {
			case tok := <-b.tokens:
				select {
				case out <- tok:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (b *blockingStreamModel) Chat(_ context.Context, _ []Message, _ *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "chat", FinishReason: "stop"}, nil
}

func (b *blockingStreamModel) ChatWithTools(_ context.Context, _ []Message, _ []LLMToolSpec, _ *LLMOptions) (*LLMResponse, error) {
	return &LLMResponse{Text: "tools", FinishReason: "stop"}, nil
}

type recordingTelemetry struct {
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(ev telemetry.Event) {
	r.events = append(r.events, ev)
}

// TestTapeModelStreamCancelClosesChannel: cancelling the call-time ctx must
// terminate the forwarding pump and close the output channel within the
// contract grace period, without leaking goroutines (R1-R3, R6).
func TestTapeModelStreamCancelClosesChannel(t *testing.T) {
	defer goleak.VerifyNone(t)
	inner := newBlockingStreamModel()
	tm, err := NewTapeModel(inner, filepath.Join(t.TempDir(), "tape.jsonl"), "record")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tm.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := tm.GenerateStream(ctx, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	inner.emit("token-1")
	if tok := <-stream; tok != "token-1" {
		t.Fatalf("unexpected token %q", tok)
	}
	// Stop consuming, then cancel: the pump must exit on ctx alone.
	inner.emit("token-2")
	cancel()
	deadline := time.After(250 * time.Millisecond)
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("expected channel close, got token after cancel")
		}
	case <-deadline:
		t.Fatal("stream channel not closed within 250ms of cancellation")
	}
	// The in-flight token is dropped either by this pump or by the inner
	// model's own ctx select; whichever receives it counts it. The tape
	// model's counter must never exceed the tokens it was handed.
	if got := tm.DroppedStreamTokens(); got > 1 {
		t.Fatalf("unexpected dropped stream token count %d", got)
	}
}

// TestTapeModelCloseIdempotent: Close is safe to call twice; the first
// result is sticky and no double-close error surfaces.
func TestTapeModelCloseIdempotent(t *testing.T) {
	tm, err := NewTapeModel(stubModel{streamText: "x"}, filepath.Join(t.TempDir(), "tape.jsonl"), "record")
	if err != nil {
		t.Fatal(err)
	}
	if err := tm.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := tm.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// TestTapeModelRecordFailureDegrades: a record write failure must degrade
// the recorder (one tape.record_failed event, subsequent entries dropped and
// counted) and must never panic or alter the caller-visible response (R5).
func TestTapeModelRecordFailureDegrades(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/dev/full fault injection is linux-only")
	}
	tel := &recordingTelemetry{}
	tm, err := NewTapeModel(stubModel{streamText: "x"}, "/dev/full", "record")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tm.Close() }()
	tm.Telemetry = tel

	resp, err := tm.Generate(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatalf("generate must succeed despite record failure: %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("unexpected response %q", resp.Text)
	}
	if !tm.Degraded() {
		t.Fatal("recorder must be degraded after write failure")
	}
	count := 0
	for _, ev := range tel.events {
		if ev.Type == telemetry.EventTapeRecordFailed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one tape.record_failed event, got %d", count)
	}
	// Further entries are dropped and counted, with no additional events.
	if _, err := tm.Generate(context.Background(), "prompt-2", nil); err != nil {
		t.Fatal(err)
	}
	if got := tm.DroppedRecords(); got != 1 {
		t.Fatalf("expected 1 dropped record, got %d", got)
	}
	count = 0
	for _, ev := range tel.events {
		if ev.Type == telemetry.EventTapeRecordFailed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("tape.record_failed must fire once, got %d", count)
	}
}
