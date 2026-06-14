package tapes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

const (
	eucloSmokeSuiteName = "euclo_gemma4_smoke"
	eucloSmokeCaseName  = "smoke"
	eucloSmokeModelName = "gemma4:e4b"
	eucloSmokeProvider  = "tape"
	eucloSmokePrompt    = "read the workspace and summarize it"
	eucloSmokeResponse  = "phase 16 canonical euclo smoke corpus"
)

func TestCommittedEucloTapeValidates(t *testing.T) {
	tapePath := filepath.Join("..", "..", "..", "testsuite", "tapes", "euclo_gemma4_smoke.tape.jsonl")
	inspection, err := llm.InspectTape(tapePath)
	if err != nil {
		t.Fatalf("inspect tape: %v", err)
	}
	if inspection.Header == nil {
		t.Fatal("expected tape header")
	}
	if inspection.Header.ProviderID != eucloSmokeProvider {
		t.Fatalf("provider id = %q, want %s", inspection.Header.ProviderID, eucloSmokeProvider)
	}
	if inspection.Header.ModelName != eucloSmokeModelName {
		t.Fatalf("model name = %q, want %q", inspection.Header.ModelName, eucloSmokeModelName)
	}
	if inspection.Header.SuiteName != eucloSmokeSuiteName || inspection.Header.CaseName != eucloSmokeCaseName {
		t.Fatalf("unexpected tape header: %+v", inspection.Header)
	}
	if inspection.EntryCount != 2 {
		t.Fatalf("entry count = %d, want 2", inspection.EntryCount)
	}

	model, err := llm.NewTapeModel(nil, tapePath, string(llm.TapeReplay))
	if err != nil {
		t.Fatalf("open tape model: %v", err)
	}
	if err := model.ConfigureHeader(llm.TapeHeader{
		ProviderID:       eucloSmokeProvider,
		ModelName:        eucloSmokeModelName,
		ModelDigest:      inspection.Header.ModelDigest,
		FrameworkVersion: inspection.Header.FrameworkVersion,
		RecordedAt:       inspection.Header.RecordedAt,
		SuiteName:        eucloSmokeSuiteName,
		CaseName:         eucloSmokeCaseName,
	}); err != nil {
		t.Fatalf("configure header: %v", err)
	}
	resp, err := model.Generate(context.Background(), eucloSmokePrompt, &llm.LLMOptions{Model: eucloSmokeModelName})
	if err != nil {
		t.Fatalf("replay generate: %v", err)
	}
	if resp == nil || resp.Text != eucloSmokeResponse {
		t.Fatalf("response = %#v, want text %q", resp, eucloSmokeResponse)
	}
}

func TestCommittedEucloLineageValidates(t *testing.T) {
	lineagePath := filepath.Join("..", "..", "..", "testsuite", "tapes", "euclo_gemma4_smoke.lineage.json")
	data, err := os.ReadFile(filepath.Clean(lineagePath))
	if err != nil {
		t.Fatalf("read lineage: %v", err)
	}
	var record PromotionLineageRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode lineage: %v", err)
	}
	if record.SuiteName != eucloSmokeSuiteName {
		t.Fatalf("suite name = %q, want %q", record.SuiteName, eucloSmokeSuiteName)
	}
	if record.CaseName != eucloSmokeCaseName {
		t.Fatalf("case name = %q, want %q", record.CaseName, eucloSmokeCaseName)
	}
	if record.Model != eucloSmokeModelName || record.Provider != eucloSmokeProvider {
		t.Fatalf("unexpected lineage record: %+v", record)
	}
	if len(record.PromotedArtifacts) != 1 || record.PromotedArtifacts[0] != "tape.jsonl" {
		t.Fatalf("promoted artifacts = %+v, want tape.jsonl", record.PromotedArtifacts)
	}
	if record.DestinationTape != "testsuite/tapes/euclo_gemma4_smoke.tape.jsonl" {
		t.Fatalf("destination tape = %q", record.DestinationTape)
	}
}
