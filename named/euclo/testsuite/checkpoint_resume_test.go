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
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestEndToEndCheckpointResumeFromPersistedArtifact(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "resume.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"implementation": {Sequence: []string{"euclo:cap.targeted_refactor"}, Operator: "OR"},
		},
	}
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCapabilityClassifier(classifier),
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
	firstEnv.SetWorkingValue("euclo.stream_result", nil, contextdata.MemoryClassTask)
	if err := graph.Execute(context.Background(), firstEnv); err != nil {
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
	if raw, ok := snapshot.WorkingData["euclo.capability_sequence"]; ok {
		var sequence []string
		if err := json.Unmarshal(raw, &sequence); err != nil {
			t.Fatalf("rehydrate capability sequence: %v", err)
		}
		rehydrated.SetWorkingValue("euclo.capability_sequence", sequence, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.capability_operator"]; ok {
		var operator string
		if err := json.Unmarshal(raw, &operator); err != nil {
			t.Fatalf("rehydrate capability operator: %v", err)
		}
		rehydrated.SetWorkingValue("euclo.capability_operator", operator, contextdata.MemoryClassTask)
	}

	if err := graph.SetStart("euclo.capability_classify"); err != nil {
		t.Fatalf("set resume start failed: %v", err)
	}
	if err := graph.Execute(context.Background(), rehydrated); err != nil {
		t.Fatalf("resume execute failed: %v", err)
	}
	if classifier.callCount() != 1 {
		t.Fatalf("expected resume to skip reclassification, call count=%d", classifier.callCount())
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("resume execution kind = %q, want capability", got)
	}
	if !mustBoolValue(t, rehydrated, "euclo.execution.completed") {
		t.Fatal("expected resume execution to complete")
	}
}

func TestEndToEndCheckpointResumeRecipePath(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "recipe.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"review": {Sequence: []string{"euclo:cap.code_review"}, Operator: "OR"},
		},
	}
	recipes := newRecipeRegistry(t, &recipepkg.ThoughtRecipe{
		ID:         "euclo.recipe.review",
		APIVersion: "euclo/v1",
		Metadata:   recipepkg.RecipeMetadata{Name: "review"},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID:           "step-1",
					CapabilityID: "euclo:cap.capture",
					Captures:     map[string]string{"output": "first_output"},
				},
			},
		},
	})
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithRecipeRegistry(recipes),
		orchestrate.WithCapabilityClassifier(classifier),
		orchestrate.WithCheckpointRepository(repo),
		orchestrate.WithPersistenceWriter(writer),
	)

	task := &core.Task{
		ID:          "task-resume-recipe",
		Type:        "euclo",
		Instruction: "review the auth package",
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}

	firstEnv := contextdata.NewEnvelope(task.ID, "session-resume-recipe")
	seedTask(firstEnv, task.Instruction, "recipe.go")
	firstEnv.SetWorkingValue("euclo.recipe_id", "euclo.recipe.review", contextdata.MemoryClassTask)
	runPreIngestion(t, firstEnv, dir, []string{"recipe.go"})
	firstEnv.RequestCheckpoint("materialize recipe resume", 7, true)
	if err := graph.Execute(context.Background(), firstEnv); err != nil {
		t.Fatalf("first execute failed: %v", err)
	}

	artifact, err := persistence.LoadLatestCheckpointArtifact(context.Background(), repo, "session-resume-recipe", "checkpoint")
	if err != nil {
		t.Fatalf("load latest checkpoint: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected checkpoint artifact to be loaded")
	}

	rehydrated := contextdata.NewEnvelope(task.ID, "session-resume-recipe")
	seedTask(rehydrated, task.Instruction, "recipe.go")
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
	if raw, ok := snapshot.WorkingData["euclo.capability_sequence"]; ok {
		var sequence []string
		if err := json.Unmarshal(raw, &sequence); err != nil {
			t.Fatalf("rehydrate capability sequence: %v", err)
		}
		rehydrated.SetWorkingValue("euclo.capability_sequence", sequence, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.capability_operator"]; ok {
		var operator string
		if err := json.Unmarshal(raw, &operator); err != nil {
			t.Fatalf("rehydrate capability operator: %v", err)
		}
		rehydrated.SetWorkingValue("euclo.capability_operator", operator, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.recipe_id"]; ok {
		var recipeID string
		if err := json.Unmarshal(raw, &recipeID); err != nil {
			t.Fatalf("rehydrate recipe id: %v", err)
		}
		rehydrated.SetWorkingValue("euclo.recipe_id", recipeID, contextdata.MemoryClassTask)
	}

	if err := graph.SetStart("euclo.capability_classify"); err != nil {
		t.Fatalf("set resume start failed: %v", err)
	}
	if err := graph.Execute(context.Background(), rehydrated); err != nil {
		t.Fatalf("resume execute failed: %v", err)
	}
	if classifier.callCount() != 1 {
		t.Fatalf("expected resume to skip reclassification, call count=%d", classifier.callCount())
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.kind"); got != "recipe" {
		t.Fatalf("resume execution kind = %q, want recipe", got)
	}
	if got := mustStringValue(t, rehydrated, "euclo.execution.recipe_id"); got != "euclo.recipe.review" {
		t.Fatalf("resume recipe id = %q, want euclo.recipe.review", got)
	}
	if !mustBoolValue(t, rehydrated, "euclo.execution.completed") {
		t.Fatal("expected resume execution to complete")
	}
}
