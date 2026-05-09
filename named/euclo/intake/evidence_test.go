package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestBuildIntentEvidenceFromNormalizedEnvelope(t *testing.T) {
	envelope := &TaskEnvelope{
		TaskID:                  "task-1",
		SessionID:               "session-1",
		Instruction:             "Please review the file",
		TaskType:                "review",
		ContextHint:             "go-backend",
		SessionHint:             "continue",
		FollowUpHint:            "check the tests",
		WorkspaceScopes:         []string{"named/euclo"},
		SessionPins:             []string{"README.md"},
		ExplicitFiles:           []string{"named/euclo/intake/types.go"},
		NegativeConstraintSeeds: []string{"don't change the API"},
		ExplicitVerification:    true,
		CleanMessage:            "Please review the file",
	}

	evidence := BuildIntentEvidence(envelope)
	if evidence == nil {
		t.Fatal("expected evidence")
	}
	if evidence.ActionType != "review" {
		t.Fatalf("ActionType = %q, want review", evidence.ActionType)
	}
	if evidence.Target != "file_set" {
		t.Fatalf("Target = %q, want file_set", evidence.Target)
	}
	if evidence.Scope != "multi_file" {
		t.Fatalf("Scope = %q, want multi_file", evidence.Scope)
	}
	if evidence.ExpectedVerb != "review" {
		t.Fatalf("ExpectedVerb = %q, want review", evidence.ExpectedVerb)
	}
	if evidence.RequiresClarification {
		t.Fatal("expected clarification to be unnecessary for grounded explicit-file request")
	}
}

func TestBuildIntentEvidenceFlagsMissingFields(t *testing.T) {
	evidence := BuildIntentEvidence(&TaskEnvelope{
		Instruction:  "help me with this",
		CleanMessage: "help me with this",
	})
	if evidence == nil {
		t.Fatal("expected evidence")
	}
	if !evidence.RequiresClarification {
		t.Fatal("expected clarification to be required")
	}
	want := map[string]bool{
		"action_type": true,
		"target":      true,
		"grounding":   true,
	}
	for _, field := range evidence.MissingFields {
		delete(want, field)
	}
	if len(want) != 0 {
		t.Fatalf("missing fields not reported: %#v", want)
	}
}

func TestIntakePipelineWritesEvidenceToEnvelope(t *testing.T) {
	node := NewIntakePipelineNode("test-intake", nil, 128, contextstream.ModeBlocking, nil)
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("task.input", &core.Task{
		ID:          "task-1",
		Instruction: "Please review named/euclo/intake/types.go",
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if got, ok := env.GetWorkingValue("euclo.intent_evidence"); !ok || got == nil {
		t.Fatal("expected intent evidence in envelope")
	}
	if got, ok := result.Data["intent_evidence"]; !ok || got == nil {
		t.Fatal("expected intent evidence in result data")
	}
	if got, ok := result.Data["missing_fields"]; !ok || got == nil {
		t.Fatal("expected missing fields in result data")
	}
}
