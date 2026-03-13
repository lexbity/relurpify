package agenttest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type preparedMemoryStore struct {
	Store   memory.MemoryStore
	Backend string
	cleanup func() error
}

func (p *preparedMemoryStore) Close() error {
	if p == nil || p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

func prepareCaseMemory(workspace string, suite *Suite, c CaseSpec, telemetry core.Telemetry) (*preparedMemoryStore, error) {
	spec := resolveMemorySpec(suite, c)
	paths := config.New(workspace)
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, err
	}
	switch spec.Backend {
	case "", "hybrid":
		store, err := memory.NewHybridMemory(paths.MemoryDir())
		if err != nil {
			return nil, err
		}
		return &preparedMemoryStore{Store: store, Backend: "hybrid"}, nil
	case "sqlite_runtime":
		opts := memorydb.SQLiteRuntimeRetrievalOptions{Telemetry: telemetry}
		if spec.Retrieval.Embedder == "test" {
			opts.Embedder = agenttestRetrievalEmbedder{}
		}
		store, err := memorydb.NewSQLiteRuntimeMemoryStoreWithRetrieval(
			filepath.Join(paths.MemoryDir(), "runtime_memory.db"),
			opts,
		)
		if err != nil {
			return nil, err
		}
		return &preparedMemoryStore{
			Store:   store,
			Backend: "sqlite_runtime",
			cleanup: store.Close,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported memory backend %q", spec.Backend)
	}
}

func resolveMemorySpec(suite *Suite, c CaseSpec) MemorySpec {
	spec := MemorySpec{Backend: "hybrid"}
	if suite != nil {
		if suite.Spec.Memory.Backend != "" {
			spec.Backend = suite.Spec.Memory.Backend
		}
		if suite.Spec.Memory.Retrieval.Embedder != "" {
			spec.Retrieval.Embedder = suite.Spec.Memory.Retrieval.Embedder
		}
	}
	if c.Overrides.Memory != nil {
		if c.Overrides.Memory.Backend != "" {
			spec.Backend = c.Overrides.Memory.Backend
		}
		if c.Overrides.Memory.Retrieval.Embedder != "" {
			spec.Retrieval.Embedder = c.Overrides.Memory.Retrieval.Embedder
		}
	}
	return spec
}

func seedCaseState(ctx context.Context, workspace string, store memory.MemoryStore, setup SetupSpec) error {
	if err := seedRuntimeMemory(ctx, store, setup.Memory); err != nil {
		return err
	}
	if len(setup.Workflows) == 0 {
		return nil
	}
	return seedWorkflowStore(ctx, filepath.Clean(config.New(workspace).WorkflowStateFile()), setup.Workflows)
}

func seedRuntimeMemory(ctx context.Context, store memory.MemoryStore, spec MemorySeedSpec) error {
	if store == nil {
		if len(spec.Declarative) == 0 && len(spec.Procedural) == 0 {
			return nil
		}
		return fmt.Errorf("memory seed requires store")
	}
	for _, record := range spec.Declarative {
		if declarativeStore, ok := store.(memory.DeclarativeMemoryStore); ok {
			if err := declarativeStore.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
				RecordID:    record.RecordID,
				Scope:       parseMemoryScope(record.Scope),
				Kind:        parseDeclarativeKind(record.Kind),
				Title:       record.Title,
				Content:     record.Content,
				Summary:     record.Summary,
				WorkflowID:  record.WorkflowID,
				TaskID:      record.TaskID,
				ProjectID:   record.ProjectID,
				ArtifactRef: record.ArtifactRef,
				Tags:        append([]string{}, record.Tags...),
				Metadata:    cloneContextMap(record.Metadata),
				Verified:    record.Verified,
			}); err != nil {
				return err
			}
			continue
		}
		value := map[string]any{
			"type":         firstNonEmpty(record.Kind, "fact"),
			"title":        record.Title,
			"content":      record.Content,
			"summary":      record.Summary,
			"workflow_id":  record.WorkflowID,
			"task_id":      record.TaskID,
			"project_id":   record.ProjectID,
			"artifact_ref": record.ArtifactRef,
			"tags":         append([]string{}, record.Tags...),
			"verified":     record.Verified,
		}
		for key, val := range record.Metadata {
			value[key] = val
		}
		if err := store.Remember(ctx, record.RecordID, value, parseMemoryScope(record.Scope)); err != nil {
			return err
		}
	}
	if len(spec.Procedural) == 0 {
		return nil
	}
	proceduralStore, ok := store.(memory.ProceduralMemoryStore)
	if !ok {
		return fmt.Errorf("procedural memory seed requires runtime memory backend")
	}
	for _, record := range spec.Procedural {
		if err := proceduralStore.PutProcedural(ctx, memory.ProceduralMemoryRecord{
			RoutineID:              record.RoutineID,
			Scope:                  parseMemoryScope(record.Scope),
			Kind:                   parseProceduralKind(record.Kind),
			Name:                   record.Name,
			Description:            record.Description,
			Summary:                record.Summary,
			WorkflowID:             record.WorkflowID,
			TaskID:                 record.TaskID,
			ProjectID:              record.ProjectID,
			BodyRef:                record.BodyRef,
			InlineBody:             record.InlineBody,
			CapabilityDependencies: append([]core.CapabilitySelector{}, record.CapabilityDependencies...),
			VerificationMetadata:   cloneContextMap(record.VerificationMetadata),
			PolicySnapshotID:       record.PolicySnapshotID,
			Verified:               record.Verified,
			Version:                record.Version,
			ReuseCount:             record.ReuseCount,
		}); err != nil {
			return err
		}
	}
	return nil
}

func seedWorkflowStore(ctx context.Context, path string, workflows []WorkflowSeedSpec) error {
	if len(workflows) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	store, err := memorydb.NewSQLiteWorkflowStateStore(path)
	if err != nil {
		return err
	}
	defer store.Close()
	for _, item := range workflows {
		workflowID := item.Workflow.WorkflowID
		if _, ok, err := store.GetWorkflow(ctx, workflowID); err != nil {
			return err
		} else if !ok {
			if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
				WorkflowID:   workflowID,
				TaskID:       firstNonEmpty(item.Workflow.TaskID, workflowID),
				TaskType:     core.TaskType(firstNonEmpty(item.Workflow.TaskType, string(core.TaskTypeAnalysis))),
				Instruction:  item.Workflow.Instruction,
				Status:       parseWorkflowStatus(item.Workflow.Status),
				CursorStepID: item.Workflow.CursorStepID,
				Metadata:     cloneContextMap(item.Workflow.Metadata),
			}); err != nil {
				return err
			}
		}
		for _, run := range item.Runs {
			if _, ok, err := store.GetRun(ctx, run.RunID); err != nil {
				return err
			} else if !ok {
				if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
					RunID:          run.RunID,
					WorkflowID:     firstNonEmpty(run.WorkflowID, workflowID),
					Status:         parseWorkflowStatus(run.Status),
					AgentName:      run.AgentName,
					AgentMode:      run.AgentMode,
					RuntimeVersion: run.RuntimeVersion,
					Metadata:       cloneContextMap(run.Metadata),
				}); err != nil {
					return err
				}
			}
		}
		for _, knowledge := range item.Knowledge {
			if err := store.PutKnowledge(ctx, memory.KnowledgeRecord{
				RecordID:   knowledge.RecordID,
				WorkflowID: firstNonEmpty(knowledge.WorkflowID, workflowID),
				StepRunID:  knowledge.StepRunID,
				StepID:     knowledge.StepID,
				Kind:       parseKnowledgeKind(knowledge.Kind),
				Title:      knowledge.Title,
				Content:    knowledge.Content,
				Status:     knowledge.Status,
				Metadata:   cloneContextMap(knowledge.Metadata),
			}); err != nil {
				return err
			}
		}
		for _, checkpoint := range item.Checkpoints {
			contextJSON, err := json.Marshal(map[string]any{
				"state": cloneContextMap(checkpoint.ContextState),
			})
			if err != nil {
				return err
			}
			resultJSON, err := json.Marshal(map[string]any{
				"stage_name":     checkpoint.StageName,
				"decoded_output": map[string]any{"type": "json", "value": cloneContextMap(checkpoint.ResultData)},
				"validation_ok":  true,
				"transition": map[string]any{
					"kind": "next",
				},
			})
			if err != nil {
				return err
			}
			if err := store.SavePipelineCheckpoint(ctx, memory.PipelineCheckpointRecord{
				CheckpointID: checkpoint.CheckpointID,
				TaskID:       checkpoint.TaskID,
				WorkflowID:   firstNonEmpty(checkpoint.WorkflowID, workflowID),
				RunID:        firstNonEmpty(checkpoint.RunID, firstWorkflowRunID(item.Runs)),
				StageName:    checkpoint.StageName,
				StageIndex:   checkpoint.StageIndex,
				ContextJSON:  string(contextJSON),
				ResultJSON:   string(resultJSON),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectMemoryOutcome(ctx context.Context, workspace string, store memory.MemoryStore) (MemoryOutcomeReport, error) {
	out := MemoryOutcomeReport{}
	if runtimeStore, ok := store.(memory.RuntimeMemoryStore); ok {
		decl, err := runtimeStore.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{Limit: 10000})
		if err != nil {
			return out, err
		}
		proc, err := runtimeStore.SearchProcedural(ctx, memory.ProceduralMemoryQuery{Limit: 10000})
		if err != nil {
			return out, err
		}
		out.DeclarativeRecordsCreated = len(decl)
		out.ProceduralRecordsCreated = len(proc)
		out.MemoryRecordsCreated = out.DeclarativeRecordsCreated + out.ProceduralRecordsCreated
	} else if store != nil {
		total := 0
		for _, scope := range []memory.MemoryScope{memory.MemoryScopeSession, memory.MemoryScopeProject, memory.MemoryScopeGlobal} {
			records, err := store.Search(ctx, "", scope)
			if err != nil {
				return out, err
			}
			total += len(records)
		}
		out.MemoryRecordsCreated = total
	}

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(config.New(workspace).WorkflowStateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	defer workflowStore.Close()
	workflowRows, err := countWorkflowRows(workflowStore.DB())
	if err != nil {
		return out, err
	}
	out.WorkflowRowsCreated = workflowRows
	out.WorkflowStateUpdated = workflowRows > 0
	return out, nil
}

func diffMemoryOutcome(before, after MemoryOutcomeReport) MemoryOutcomeReport {
	out := MemoryOutcomeReport{
		DeclarativeRecordsCreated: maxInt(after.DeclarativeRecordsCreated-before.DeclarativeRecordsCreated, 0),
		ProceduralRecordsCreated:  maxInt(after.ProceduralRecordsCreated-before.ProceduralRecordsCreated, 0),
		MemoryRecordsCreated:      maxInt(after.MemoryRecordsCreated-before.MemoryRecordsCreated, 0),
		WorkflowRowsCreated:       maxInt(after.WorkflowRowsCreated-before.WorkflowRowsCreated, 0),
	}
	out.WorkflowStateUpdated = out.WorkflowRowsCreated > 0
	return out
}

func countWorkflowRows(db *sql.DB) (int, error) {
	if db == nil {
		return 0, nil
	}
	tables := []string{
		"workflows",
		"workflow_runs",
		"workflow_plans",
		"workflow_steps",
		"step_runs",
		"step_artifacts",
		"workflow_artifacts",
		"workflow_stage_results",
		"pipeline_checkpoints",
		"workflow_knowledge",
		"workflow_events",
	}
	total := 0
	for _, table := range tables {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return 0, err
		}
		total += count
	}
	return total, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstWorkflowRunID(runs []WorkflowRunSeedSpec) string {
	if len(runs) == 0 {
		return ""
	}
	return strings.TrimSpace(runs[0].RunID)
}

func parseMemoryScope(raw string) memory.MemoryScope {
	switch strings.TrimSpace(raw) {
	case string(memory.MemoryScopeSession):
		return memory.MemoryScopeSession
	case string(memory.MemoryScopeGlobal):
		return memory.MemoryScopeGlobal
	default:
		return memory.MemoryScopeProject
	}
}

func parseDeclarativeKind(raw string) memory.DeclarativeMemoryKind {
	switch strings.TrimSpace(raw) {
	case string(memory.DeclarativeMemoryKindDecision):
		return memory.DeclarativeMemoryKindDecision
	case string(memory.DeclarativeMemoryKindConstraint):
		return memory.DeclarativeMemoryKindConstraint
	case string(memory.DeclarativeMemoryKindPreference):
		return memory.DeclarativeMemoryKindPreference
	case string(memory.DeclarativeMemoryKindProjectKnowledge):
		return memory.DeclarativeMemoryKindProjectKnowledge
	default:
		return memory.DeclarativeMemoryKindFact
	}
}

func parseProceduralKind(raw string) memory.ProceduralMemoryKind {
	switch strings.TrimSpace(raw) {
	case string(memory.ProceduralMemoryKindCapabilityComposition):
		return memory.ProceduralMemoryKindCapabilityComposition
	case string(memory.ProceduralMemoryKindRecoveryRoutine):
		return memory.ProceduralMemoryKindRecoveryRoutine
	default:
		return memory.ProceduralMemoryKindRoutine
	}
}

func parseWorkflowStatus(raw string) memory.WorkflowRunStatus {
	switch strings.TrimSpace(raw) {
	case string(memory.WorkflowRunStatusPending):
		return memory.WorkflowRunStatusPending
	case string(memory.WorkflowRunStatusCompleted):
		return memory.WorkflowRunStatusCompleted
	case string(memory.WorkflowRunStatusFailed):
		return memory.WorkflowRunStatusFailed
	case string(memory.WorkflowRunStatusCanceled):
		return memory.WorkflowRunStatusCanceled
	case string(memory.WorkflowRunStatusNeedsReplan):
		return memory.WorkflowRunStatusNeedsReplan
	default:
		return memory.WorkflowRunStatusRunning
	}
}

func parseKnowledgeKind(raw string) memory.KnowledgeKind {
	switch strings.TrimSpace(raw) {
	case string(memory.KnowledgeKindIssue):
		return memory.KnowledgeKindIssue
	case string(memory.KnowledgeKindDecision):
		return memory.KnowledgeKindDecision
	default:
		return memory.KnowledgeKindFact
	}
}

type agenttestRetrievalEmbedder struct{}

func (agenttestRetrievalEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		out = append(out, []float32{float32(len(text)), 1})
	}
	return out, nil
}

func (agenttestRetrievalEmbedder) ModelID() string { return "agenttest-retrieval-v1" }
func (agenttestRetrievalEmbedder) Dims() int       { return 2 }

var _ retrieval.Embedder = agenttestRetrievalEmbedder{}
