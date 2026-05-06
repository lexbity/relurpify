package testsuite

import (
	"context"
	"encoding/json"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

type backgroundContinuationNode struct{}

func (n *backgroundContinuationNode) ID() string { return "euclo.background.resume" }

func (n *backgroundContinuationNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

func (n *backgroundContinuationNode) Execute(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if env == nil {
		return nil, nil
	}
	if _, ok := env.GetWorkingValue("euclo.background.job_id"); !ok {
		return &core.Result{NodeID: n.ID(), Success: false}, nil
	}
	env.SetWorkingValue("euclo.background.resume_completed", true, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.final_response", "background work resumed and completed", contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)
	return &core.Result{
		NodeID:  n.ID(),
		Success: true,
		Data: map[string]any{
			"final_response": "background work resumed and completed",
		},
	}, nil
}

func TestEndToEndBackgroundContinuationFromCheckpoint(t *testing.T) {
	repo := &checkpointArtifactRepo{}
	writer := newPersistenceWriter(t)
	submitter := &recordingSubmitter{}
	backgroundNode := orchestrate.NewBackgroundJobNode("euclo.background").
		WithSubmitter(submitter).
		WithDefaultQueue("background").
		WithDefaultKind("euclo.background.longrun")
	checkpointNode := agentgraph.NewCheckpointNode("euclo.checkpoint").
		WithRepository(repo).
		WithWriter(writer)

	env := contextdata.NewEnvelope("task-background", "session-background")
	env.SetWorkingValue("task.input", &core.Task{ID: "task-background", Instruction: "run background work"}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.background.payload", map[string]any{"mode": "long-run"}, contextdata.MemoryClassTask)

	if result, err := backgroundNode.Execute(context.Background(), env); err != nil {
		t.Fatalf("background submit failed: %v", err)
	} else if result == nil || !result.Success {
		t.Fatalf("expected successful background submission, got %#v", result)
	}

	env.RequestCheckpoint("persist background continuation", 5, true)
	env.SetWorkingValue("euclo.background.job_state", string(jobs.JobStateQueued), contextdata.MemoryClassTask)
	if result, err := checkpointNode.Execute(context.Background(), env); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	} else if result == nil || !result.Success {
		t.Fatalf("expected checkpoint success, got %#v", result)
	}

	artifact, err := persistence.LoadLatestCheckpointArtifact(context.Background(), repo, "session-background", "checkpoint")
	if err != nil {
		t.Fatalf("load latest checkpoint: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected checkpoint artifact to be persisted")
	}

	var snapshot struct {
		WorkingData map[string]json.RawMessage `json:"working_data"`
	}
	if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
		t.Fatalf("unmarshal checkpoint payload: %v", err)
	}

	resumed := contextdata.NewEnvelope("task-background", "session-background")
	if raw, ok := snapshot.WorkingData["euclo.background.job_id"]; ok {
		var jobID string
		if err := json.Unmarshal(raw, &jobID); err != nil {
			t.Fatalf("rehydrate job id: %v", err)
		}
		resumed.SetWorkingValue("euclo.background.job_id", jobID, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.background.job_kind"]; ok {
		var jobKind string
		if err := json.Unmarshal(raw, &jobKind); err != nil {
			t.Fatalf("rehydrate job kind: %v", err)
		}
		resumed.SetWorkingValue("euclo.background.job_kind", jobKind, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.background.job_completed"]; ok {
		var completed bool
		if err := json.Unmarshal(raw, &completed); err != nil {
			t.Fatalf("rehydrate completion flag: %v", err)
		}
		resumed.SetWorkingValue("euclo.background.job_completed", completed, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.background.job_completion"]; ok {
		var completion map[string]any
		if err := json.Unmarshal(raw, &completion); err != nil {
			t.Fatalf("rehydrate completion payload: %v", err)
		}
		resumed.SetWorkingValue("euclo.background.job_completion", completion, contextdata.MemoryClassTask)
	}
	if raw, ok := snapshot.WorkingData["euclo.background.job_submitted"]; ok {
		var submitted bool
		if err := json.Unmarshal(raw, &submitted); err != nil {
			t.Fatalf("rehydrate submission flag: %v", err)
		}
		resumed.SetWorkingValue("euclo.background.job_submitted", submitted, contextdata.MemoryClassTask)
	}
	resumed.AddCheckpointReference(contextdata.CheckpointReference{
		CheckpointID:      artifact.ArtifactID,
		RequestedBy:       "euclo.checkpoint",
		WorkingMemoryKeys: resumed.WorkingMemoryKeys(),
	})

	resumeGraph := agentgraph.NewGraph()
	if err := resumeGraph.AddNode(&backgroundContinuationNode{}); err != nil {
		t.Fatalf("add resume node: %v", err)
	}
	if err := resumeGraph.SetStart("euclo.background.resume"); err != nil {
		t.Fatalf("set resume start: %v", err)
	}

	if _, err := resumeGraph.Execute(context.Background(), resumed); err != nil {
		t.Fatalf("resume graph execute failed: %v", err)
	}
	if got := mustStringValue(t, resumed, "euclo.final_response"); got != "background work resumed and completed" {
		t.Fatalf("final response = %q", got)
	}
	if !mustBoolValue(t, resumed, "euclo.background.resume_completed") {
		t.Fatal("expected resume completion marker")
	}
	if len(resumed.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint reference on resumed envelope, got %d", len(resumed.References.Checkpoints))
	}
}
