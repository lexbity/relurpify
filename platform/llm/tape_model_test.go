package llm

import (
	"bytes"
	"bufio"
	"context"
	"encoding/json"
	"github.com/lexcodex/relurpify/framework/core"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubModel struct {
	streamText string
}

func (s stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}
func (s stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- s.streamText
	close(ch)
	return ch, nil
}
func (s stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "chat", FinishReason: "stop"}, nil
}
func (s stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "tools", FinishReason: "stop", ToolCalls: []core.ToolCall{{Name: "file_read", Args: map[string]any{"path": "x"}}}}, nil
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
	defer rec.Close()

	if _, err := rec.Generate(context.Background(), "p", &core.LLMOptions{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Chat(context.Background(), []core.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil {
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
	if resp, err := replay.Generate(context.Background(), "p", &core.LLMOptions{Model: "m"}); err != nil || resp.Text != "ok" {
		t.Fatalf("replay generate: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.Chat(context.Background(), []core.Message{{Role: "user", Content: "hi"}}, nil); err != nil || resp.Text != "chat" {
		t.Fatalf("replay chat: resp=%+v err=%v", resp, err)
	}
	if resp, err := replay.ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil || resp.Text != "tools" {
		t.Fatalf("replay chat_with_tools: resp=%+v err=%v", resp, err)
	}
	ch, err := replay.GenerateStream(context.Background(), "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for token := range ch {
		got += token
	}
	if got != "streamed" {
		t.Fatalf("replay stream got %q", got)
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

	f, err := os.Open(tape)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
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

func TestTapeModelReplayRejectsMismatchedModelHeader(t *testing.T) {
	tape := writeTapeFixture(t, []tapeEntry{
		{Kind: "_header", Request: tapeRequest{Header: &TapeHeader{Kind: "_header", ModelName: "model-a"}}},
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &core.LLMResponse{Text: "ok"}},
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
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &core.LLMResponse{Text: "ok"}},
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
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &core.LLMResponse{Text: "ok"}},
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
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "old prompt"}), Response: &core.LLMResponse{Text: "ok"}},
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
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &core.LLMResponse{Text: "ok"}},
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
		{Kind: "generate", Fingerprint: fingerprint("generate", tapeRequest{Prompt: "p"}), Response: &core.LLMResponse{Text: "ok"}},
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
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
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
