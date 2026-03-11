package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

var ErrPipelineCheckpointNotFound = errors.New("pipeline checkpoint not found")

type SQLitePipelineCheckpointStore struct {
	store      *db.SQLiteWorkflowStateStore
	workflowID string
	runID      string
}

func NewSQLitePipelineCheckpointStore(store *db.SQLiteWorkflowStateStore, workflowID, runID string) *SQLitePipelineCheckpointStore {
	return &SQLitePipelineCheckpointStore{
		store:      store,
		workflowID: workflowID,
		runID:      runID,
	}
}

type pipelineContextEnvelope struct {
	State map[string]any `json:"state"`
}

type pipelineResultEnvelope struct {
	StageName       string                   `json:"stage_name"`
	ContractName    string                   `json:"contract_name"`
	ContractVersion string                   `json:"contract_version"`
	Prompt          string                   `json:"prompt"`
	Response        *core.LLMResponse        `json:"response,omitempty"`
	DecodedOutput   map[string]any           `json:"decoded_output,omitempty"`
	DecodedJSON     string                   `json:"decoded_json,omitempty"`
	ValidationOK    bool                     `json:"validation_ok"`
	ErrorText       string                   `json:"error_text,omitempty"`
	RetryAttempt    int                      `json:"retry_attempt"`
	StartedAt       string                   `json:"started_at,omitempty"`
	FinishedAt      string                   `json:"finished_at,omitempty"`
	Transition      pipeline.StageTransition `json:"transition"`
}

func (s *SQLitePipelineCheckpointStore) Save(checkpoint *pipeline.Checkpoint) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("pipeline checkpoint store unavailable")
	}
	if checkpoint == nil {
		return fmt.Errorf("pipeline checkpoint required")
	}
	contextJSON, err := json.Marshal(pipelineContextEnvelope{
		State: checkpoint.Context.StateSnapshot(),
	})
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(pipelineResultEnvelope{
		StageName:       checkpoint.Result.StageName,
		ContractName:    checkpoint.Result.ContractName,
		ContractVersion: checkpoint.Result.ContractVersion,
		Prompt:          checkpoint.Result.Prompt,
		Response:        checkpoint.Result.Response,
		DecodedOutput: map[string]any{
			"type":  "json",
			"value": checkpoint.Result.DecodedOutput,
		},
		DecodedJSON:  checkpoint.Result.DecodedJSON,
		ValidationOK: checkpoint.Result.ValidationOK,
		ErrorText:    checkpoint.Result.ErrorText,
		RetryAttempt: checkpoint.Result.RetryAttempt,
		StartedAt:    checkpoint.Result.StartedAt.UTC().Format(timeLayout),
		FinishedAt:   checkpoint.Result.FinishedAt.UTC().Format(timeLayout),
		Transition:   checkpoint.Result.Transition,
	})
	if err != nil {
		return err
	}
	return s.store.SavePipelineCheckpoint(context.Background(), memory.PipelineCheckpointRecord{
		CheckpointID: checkpoint.CheckpointID,
		TaskID:       checkpoint.TaskID,
		WorkflowID:   s.workflowID,
		RunID:        s.runID,
		StageName:    checkpoint.StageName,
		StageIndex:   checkpoint.StageIndex,
		ContextJSON:  string(contextJSON),
		ResultJSON:   string(resultJSON),
		CreatedAt:    checkpoint.CreatedAt,
	})
}

func (s *SQLitePipelineCheckpointStore) Load(taskID, checkpointID string) (*pipeline.Checkpoint, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("pipeline checkpoint store unavailable")
	}
	record, ok, err := s.store.LoadPipelineCheckpoint(context.Background(), taskID, checkpointID)
	if err != nil {
		return nil, err
	}
	if !ok || record == nil {
		return nil, ErrPipelineCheckpointNotFound
	}
	ctx := core.NewContext()
	if record.ContextJSON != "" {
		var envelope pipelineContextEnvelope
		if err := json.Unmarshal([]byte(record.ContextJSON), &envelope); err != nil {
			return nil, err
		}
		for key, value := range envelope.State {
			ctx.Set(key, value)
		}
	}
	var resultEnvelope pipelineResultEnvelope
	if record.ResultJSON != "" {
		if err := json.Unmarshal([]byte(record.ResultJSON), &resultEnvelope); err != nil {
			return nil, err
		}
	}
	result := pipeline.StageResult{
		StageName:       resultEnvelope.StageName,
		ContractName:    resultEnvelope.ContractName,
		ContractVersion: resultEnvelope.ContractVersion,
		Prompt:          resultEnvelope.Prompt,
		Response:        resultEnvelope.Response,
		DecodedJSON:     resultEnvelope.DecodedJSON,
		ValidationOK:    resultEnvelope.ValidationOK,
		ErrorText:       resultEnvelope.ErrorText,
		RetryAttempt:    resultEnvelope.RetryAttempt,
		Transition:      resultEnvelope.Transition,
	}
	if value, ok := resultEnvelope.DecodedOutput["value"]; ok {
		result.DecodedOutput = value
	}
	result.StartedAt = parsePipelineTime(resultEnvelope.StartedAt)
	result.FinishedAt = parsePipelineTime(resultEnvelope.FinishedAt)
	return &pipeline.Checkpoint{
		CheckpointID: record.CheckpointID,
		TaskID:       record.TaskID,
		StageName:    record.StageName,
		StageIndex:   record.StageIndex,
		CreatedAt:    record.CreatedAt,
		Context:      ctx,
		Result:       result,
	}, nil
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

func parsePipelineTime(value string) (parsedTime time.Time) {
	if value == "" {
		return time.Time{}
	}
	parsedTime, _ = time.Parse(timeLayout, value)
	return parsedTime
}
