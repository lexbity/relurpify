package testsuite

import (
	"context"
	"encoding/json"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestEndToEndCheckpointResumeFromPersistedArtifact(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "resume.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCheckpointRepository(repo),
		orchestrate.WithPersistenceWriter(writer),
	)

	task := &core.Task{
		ID:          "task-resume-checkpoint",
		Type:        "euclo",
		Instruction: "add a cache to the handler",
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}

	firstEnv := contextdata.NewEnvelope(task.ID, "session-resume-checkpoint")
	seedTask(firstEnv, task.Instruction, "resume.go")
	runPreIngestion(t, firstEnv, dir, []string{"resume.go"})
	firstEnv.RequestCheckpoint("materialize for resume", 5, true)
	euclostate.SetStreamResult(firstEnv, nil)
	if err := graph.Execute(ctxWithTrigger(context.Background()), firstEnv); err != nil {
		t.Fatalf("first execute failed: %v", err)
	}

	artifact, err := persistence.LoadLatestCheckpointArtifact(context.Background(), repo, "session-resume-checkpoint", "checkpoint")
	if err != nil {
		t.Fatalf("load latest checkpoint: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected checkpoint artifact to be loaded")
	}

	rehydrated := contextdata.NewEnvelope(task.ID, "session-resume-checkpoint")
	seedTask(rehydrated, task.Instruction, "resume.go")
	var snapshot struct {
		WorkingData map[string]json.RawMessage `json:"working_data"`
	}
	if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
		t.Fatalf("unmarshal checkpoint payload: %v", err)
	}
	if raw, ok := snapshot.WorkingData["euclo.intent_classification"]; ok {
		var classification intake.IntentClassification
		if err := json.Unmarshal(raw, &classification); err != nil {
			t.Fatalf("rehydrate classification: %v", err)
		}
		euclostate.SetIntentClassification(rehydrated, &classification)
	}

	if err := graph.SetStart("euclo.capability_classify"); err != nil {
		t.Fatalf("set resume start failed: %v", err)
	}
	if err := graph.Execute(ctxWithTrigger(context.Background()), rehydrated); err != nil {
		t.Fatalf("resume execute failed: %v", err)
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("resume execution kind = %q, want thoughtrecipe", got)
	}
	if !mustBoolValue(t, rehydrated, "euclo.execution.completed") {
		t.Fatal("expected resume execution to complete")
	}
}

func TestEndToEndCheckpointResumeThoughtRecipePath(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "thoughtrecipe.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture")
	thoughtrecipes := newThoughtRecipeRegistry(t, &thoughtrecipepkg.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.review",
		Name:     "review",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "review"},
	})
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
		orchestrate.WithCheckpointRepository(repo),
		orchestrate.WithPersistenceWriter(writer),
	)

	task := &core.Task{
		ID:          "task-resume-thoughtrecipe",
		Type:        "euclo",
		Instruction: "review the auth package",
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}

	firstEnv := contextdata.NewEnvelope(task.ID, "session-resume-thoughtrecipe")
	seedTask(firstEnv, task.Instruction, "thoughtrecipe.go")
	euclostate.SetThoughtRecipeID(firstEnv, "euclo.thoughtrecipe.review")
	runPreIngestion(t, firstEnv, dir, []string{"thoughtrecipe.go"})
	firstEnv.RequestCheckpoint("materialize thoughtrecipe resume", 7, true)
	if err := graph.Execute(ctxWithTrigger(context.Background()), firstEnv); err != nil {
		t.Fatalf("first execute failed: %v", err)
	}

	artifact, err := persistence.LoadLatestCheckpointArtifact(context.Background(), repo, "session-resume-thoughtrecipe", "checkpoint")
	if err != nil {
		t.Fatalf("load latest checkpoint: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected checkpoint artifact to be loaded")
	}

	rehydrated := contextdata.NewEnvelope(task.ID, "session-resume-thoughtrecipe")
	seedTask(rehydrated, task.Instruction, "thoughtrecipe.go")
	var snapshot struct {
		WorkingData map[string]json.RawMessage `json:"working_data"`
	}
	if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
		t.Fatalf("unmarshal checkpoint payload: %v", err)
	}
	if raw, ok := snapshot.WorkingData["euclo.intent_classification"]; ok {
		var classification intake.IntentClassification
		if err := json.Unmarshal(raw, &classification); err != nil {
			t.Fatalf("rehydrate classification: %v", err)
		}
		euclostate.SetIntentClassification(rehydrated, &classification)
	}
	if raw, ok := snapshot.WorkingData["euclo.thoughtrecipe_id"]; ok {
		var thoughtrecipeID string
		if err := json.Unmarshal(raw, &thoughtrecipeID); err != nil {
			t.Fatalf("rehydrate thoughtrecipe id: %v", err)
		}
		euclostate.SetThoughtRecipeID(rehydrated, thoughtrecipeID)
	}

	if err := graph.SetStart("euclo.capability_classify"); err != nil {
		t.Fatalf("set resume start failed: %v", err)
	}
	if err := graph.Execute(ctxWithTrigger(context.Background()), rehydrated); err != nil {
		t.Fatalf("resume execute failed: %v", err)
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("resume execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("resume thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if !mustBoolValue(t, rehydrated, "euclo.execution.completed") {
		t.Fatal("expected resume execution to complete")
	}
}
