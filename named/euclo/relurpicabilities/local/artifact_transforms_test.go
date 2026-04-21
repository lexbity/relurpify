package local

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestBuildVerificationSummaryPayload_TracksProvenanceCounts(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ID:      "verify-1",
			Kind:    euclotypes.ArtifactKindVerification,
			Summary: "go test ./... passed",
			Payload: map[string]any{
				"summary":    "go test ./... passed",
				"provenance": "executed",
				"run_id":     "run-1",
			},
		},
		{
			ID:      "verify-2",
			Kind:    euclotypes.ArtifactKindVerification,
			Summary: "reused existing verification evidence",
			Payload: map[string]any{
				"summary":    "reused existing verification evidence",
				"provenance": "reused",
				"run_id":     "run-1",
			},
		},
	}

	payload := buildVerificationSummaryPayload(artifacts)
	if payload["provenance"] != "reused" {
		t.Fatalf("expected last-seen provenance to surface, got %#v", payload["provenance"])
	}
	if payload["run_id"] != "run-1" {
		t.Fatalf("expected run id, got %#v", payload["run_id"])
	}
	if payload["executed_check_count"] != 1 {
		t.Fatalf("expected executed count 1, got %#v", payload["executed_check_count"])
	}
	if payload["reused_check_count"] != 1 {
		t.Fatalf("expected reused count 1, got %#v", payload["reused_check_count"])
	}
}

func TestReviewFinding_IncludesTraceabilityAndImpactedFiles(t *testing.T) {
	finding := reviewFinding("critical", "pkg/foo.go:10", "panic in hot path", "return an error", 0.9, "correctness", "pkg/foo.go", []string{"Foo"}, "line_scan")

	if finding["review_source"] != "euclo:review.findings" {
		t.Fatalf("unexpected review source %#v", finding["review_source"])
	}
	impactedFiles, ok := finding["impacted_files"].([]string)
	if !ok || len(impactedFiles) != 1 || impactedFiles[0] != "pkg/foo.go" {
		t.Fatalf("unexpected impacted files %#v", finding["impacted_files"])
	}
	traceability, ok := finding["traceability"].(map[string]any)
	if !ok || traceability["source"] != "line_scan" {
		t.Fatalf("unexpected traceability %#v", finding["traceability"])
	}
}
