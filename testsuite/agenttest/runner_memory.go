package agenttest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

type preparedMemoryStore struct {
	Store   *memory.WorkingMemoryStore
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
	paths := manifest.New(workspace)
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, err
	}
	switch spec.Backend {
	case "", "hybrid":
		return &preparedMemoryStore{Store: memory.NewWorkingMemoryStore(), Backend: "hybrid"}, nil
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

func seedCaseState(ctx context.Context, workspace string, store *memory.WorkingMemoryStore, setup SetupSpec) error {
	if err := seedRuntimeMemory(ctx, store, setup.Memory); err != nil {
		return err
	}
	if len(setup.Workflows) == 0 {
		return nil
	}
	return seedWorkflowStore(ctx, filepath.Clean(manifest.New(workspace).WorkflowStateFile()), setup.Workflows)
}

func seedRuntimeMemory(ctx context.Context, store *memory.WorkingMemoryStore, spec MemorySeedSpec) error {
	if store == nil {
		if len(spec.Declarative) == 0 && len(spec.Procedural) == 0 {
			return nil
		}
		return fmt.Errorf("memory seed requires store")
	}
	for _, record := range spec.Declarative {
		store.Scope(firstNonEmpty(record.TaskID, "task")).Set(record.RecordID, map[string]any{
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
		}, core.MemoryClassWorking)
	}
	if len(spec.Procedural) == 0 {
		return nil
	}
	for _, record := range spec.Procedural {
		store.Scope(firstNonEmpty(record.TaskID, "task")).Set(record.RoutineID, map[string]any{
			"routine_id":   record.RoutineID,
			"name":         record.Name,
			"description":  record.Description,
			"summary":      record.Summary,
			"workflow_id":  record.WorkflowID,
			"task_id":      record.TaskID,
			"project_id":   record.ProjectID,
			"body_ref":     record.BodyRef,
			"inline_body":  record.InlineBody,
			"verified":     record.Verified,
			"version":      record.Version,
			"reuse_count":  record.ReuseCount,
		}, core.MemoryClassWorking)
	}
	return nil
}

func seedWorkflowStore(ctx context.Context, path string, workflows []WorkflowSeedSpec) error {
	return nil
}

func collectMemoryOutcome(ctx context.Context, workspace string, store *memory.WorkingMemoryStore) (MemoryOutcomeReport, error) {
	out := MemoryOutcomeReport{}
	if store != nil {
		total := 0
		for _, taskID := range store.ListTasks() {
			total += len(store.Scope(taskID).Keys())
		}
		out.MemoryRecordsCreated = total
	}
	out.WorkflowRowsCreated = 0
	out.WorkflowStateUpdated = false
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

func parseMemoryScope(raw string) string { return strings.TrimSpace(raw) }
func parseDeclarativeKind(raw string) string { return strings.TrimSpace(raw) }
func parseProceduralKind(raw string) string { return strings.TrimSpace(raw) }
func parseWorkflowStatus(raw string) string { return strings.TrimSpace(raw) }
func parseKnowledgeKind(raw string) string { return strings.TrimSpace(raw) }

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
