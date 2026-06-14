package llm

import (
	"context"
	"path/filepath"
	"testing"
)

const (
	eucloSmokeTapePath   = "../../testsuite/tapes/euclo_gemma4_smoke.tape.jsonl"
	eucloSmokePromptText = "read the workspace and summarize it"
	eucloSmokeModelText  = "gemma4:e4b"
	eucloSmokeReplyText  = "phase 16 canonical euclo smoke corpus"
)

func TestTapeModelReplaysCommittedEucloSmoke(t *testing.T) {
	tapePath := filepath.Clean(eucloSmokeTapePath)
	inspection, err := InspectTape(tapePath)
	if err != nil {
		t.Fatalf("inspect tape: %v", err)
	}
	if inspection.Header == nil {
		t.Fatal("expected tape header")
	}
	model, err := NewTapeModel(nil, tapePath, string(TapeReplay))
	if err != nil {
		t.Fatalf("open tape model: %v", err)
	}
	if err := model.ConfigureHeader(TapeHeader{
		ProviderID:       "tape",
		ModelName:        eucloSmokeModelText,
		ModelDigest:      inspection.Header.ModelDigest,
		FrameworkVersion: inspection.Header.FrameworkVersion,
		RecordedAt:       inspection.Header.RecordedAt,
		SuiteName:        inspection.Header.SuiteName,
		CaseName:         inspection.Header.CaseName,
	}); err != nil {
		t.Fatalf("configure header: %v", err)
	}
	resp, err := model.Generate(context.Background(), eucloSmokePromptText, &LLMOptions{Model: eucloSmokeModelText})
	if err != nil {
		t.Fatalf("replay generate: %v", err)
	}
	if resp == nil || resp.Text != eucloSmokeReplyText {
		t.Fatalf("response = %#v, want text %q", resp, eucloSmokeReplyText)
	}
}
