package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	frameworkingestion "codeburg.org/lexbit/relurpify/context/knowledge/ingestion"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
)

// IngestionNode runs the framework ingestion pipeline for Euclo tasks.
type IngestionNode struct {
	id   string
	spec IngestionSpec
}

// NewIngestionNode creates a new IngestionNode.
func NewIngestionNode(id string, spec IngestionSpec) *IngestionNode {
	return &IngestionNode{id: id, spec: spec}
}

// ID returns the node ID.
func (n *IngestionNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *IngestionNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeTool
}

// Contract returns the node contract.
func (n *IngestionNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectLocal,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
	}
}

// Execute runs the configured ingestion mode.
func (n *IngestionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	taskEnvelope := taskEnvelopeFromEnv(env)
	if taskEnvelope == nil {
		result := &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(map[string]any{"skipped": true})}
		return result, nil
	}

	mode := n.resolveMode(taskEnvelope)
	store, cleanup, err := n.ensureStore(ctx)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	summary := map[string]any{
		"mode":                  mode,
		"task_id":               taskEnvelope.TaskID,
		"session_id":            taskEnvelope.SessionID,
		"skipped":               false,
		"user_files_ingested":   0,
		"session_pins_ingested": 0,
		"files_scanned":         0,
		"chunks_created":        0,
		"chunks_quarantined":    0,
		"chunks_rejected":       0,
	}
	result := &IngestionResult{Mode: mode}

	switch mode {
	case IngestionModeFilesOnly:
		if err := n.ingestExplicitFiles(ctx, env, taskEnvelope, store, result, summary); err != nil {
			result.Error = err.Error()
			n.writeSummary(env, result, summary, err)
			return &execution.Result{NodeID: n.id, Success: false, Data: execution.NewToolResultPayload(summary), Error: err.Error()}, err
		}
	case IngestionModeIncremental, IngestionModeFull:
		if err := n.ingestWorkspace(ctx, env, taskEnvelope, store, result, summary, mode); err != nil {
			result.Error = err.Error()
			n.writeSummary(env, result, summary, err)
			return &execution.Result{NodeID: n.id, Success: false, Data: execution.NewToolResultPayload(summary), Error: err.Error()}, err
		}
	default:
		err := fmt.Errorf("unknown ingestion mode: %s", mode)
		result.Error = err.Error()
		n.writeSummary(env, result, summary, err)
		return &execution.Result{NodeID: n.id, Success: false, Data: execution.NewToolResultPayload(summary), Error: err.Error()}, err
	}

	if mode == IngestionModeFilesOnly {
		result.FileCount = len(result.Records)
	} else {
		result.FileCount = summaryInt(summary, "files_scanned")
	}
	result.ChunkCount = summaryInt(summary, "chunks_created")
	result.CompletedAt = time.Now().UTC().Unix()
	if since, ok := summary["since_ref"].(string); ok {
		result.SinceRef = since
	}

	n.writeSummary(env, result, summary, nil)
	return &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(summary)}, nil
}

func (n *IngestionNode) resolveMode(task *intake.TaskEnvelope) IngestionMode {
	if task == nil {
		return IngestionModeFilesOnly
	}
	mode := strings.TrimSpace(string(n.spec.Mode))
	if mode == "" {
		mode = strings.TrimSpace(task.IngestPolicy)
	}
	if mode == "" {
		if len(task.ExplicitFiles)+len(task.UserFiles)+len(task.SessionPins) > 0 {
			return IngestionModeFilesOnly
		}
		return IngestionModeFull
	}
	switch IngestionMode(mode) {
	case IngestionModeFilesOnly, IngestionModeIncremental, IngestionModeFull:
		return IngestionMode(mode)
	default:
		return IngestionModeFull
	}
}

func (n *IngestionNode) ingestExplicitFiles(ctx context.Context, env *contextdata.Envelope, task *intake.TaskEnvelope, store *knowledge.ChunkStore, result *IngestionResult, summary map[string]any) error {
	files := append([]string(nil), task.ExplicitFiles...)
	files = append(files, task.UserFiles...)
	files = append(files, task.SessionPins...)
	root := strings.TrimSpace(n.spec.WorkspaceRoot)
	for _, filePath := range files {
		absPath := n.resolvePath(root, filePath)
		pipeline, err := frameworkingestion.AcquireFromFile(ctx, absPath, defaultPrincipal(task), &contextports.PolicyBundle{
			DefaultTrustClass: string(agentspec.TrustClassBuiltinTrusted),
		}, nil, store, nil)
		if err != nil {
			return err
		}
		pipeline.SetQuarantineDir(filepath.Join(os.TempDir(), "euclo-ingestion-quarantine"))
		ingestResult, err := pipeline.Run(ctx)
		if err != nil {
			return err
		}
		record := FileIngestionRecord{
			Path:        absPath,
			ChunkCount:  ingestResult.ChunksCommitted,
			SizeBytes:   fileSize(absPath),
			IngestedAt:  time.Now().UTC().Unix(),
			ContentHash: contentHash(absPath),
		}
		result.Records = append(result.Records, record)
		result.ChunkCount += record.ChunkCount
		if len(task.UserFiles) > 0 && containsPath(task.UserFiles, filePath) {
			summary["user_files_ingested"] = summaryInt(summary, "user_files_ingested") + 1
		}
		if len(task.SessionPins) > 0 && containsPath(task.SessionPins, filePath) {
			summary["session_pins_ingested"] = summaryInt(summary, "session_pins_ingested") + 1
		}
		n.storeFileSummary(env, filePath, ingestResult)
	}
	summary["chunks_created"] = result.ChunkCount
	return nil
}

func (n *IngestionNode) ingestWorkspace(ctx context.Context, env *contextdata.Envelope, task *intake.TaskEnvelope, store *knowledge.ChunkStore, result *IngestionResult, summary map[string]any, mode IngestionMode) error {
	root := strings.TrimSpace(n.spec.WorkspaceRoot)
	if root == "" {
		return fmt.Errorf("%s ingestion requires workspace root", mode)
	}

	scanner := &frameworkingestion.WorkspaceScanner{
		Store:         store,
		Policy:        n.policyBundle(),
		FileScope:     nil,
		IncludeGlobs:  append([]string(nil), n.spec.IncludeGlobs...),
		ExcludeGlobs:  append([]string(nil), n.spec.ExcludeGlobs...),
		QuarantineDir: filepath.Join(os.TempDir(), "euclo-ingestion-quarantine"),
	}
	var scanReport *frameworkingestion.ScanReport
	var err error
	switch mode {
	case IngestionModeIncremental:
		since := strings.TrimSpace(n.spec.SinceRef)
		if since == "" {
			since = strings.TrimSpace(task.IncrementalSince)
		}
		if since == "" {
			since = "HEAD~1"
		}
		scanReport, err = scanner.ScanIncremental(ctx, root, since)
		if err == nil {
			summary["since_ref"] = since
		}
	case IngestionModeFull:
		scanReport, err = scanner.Scan(ctx, root)
	}
	if err != nil {
		return err
	}
	if scanReport == nil {
		scanReport = &frameworkingestion.ScanReport{}
	}
	summary["files_scanned"] = scanReport.FilesScanned
	summary["chunks_created"] = scanReport.ChunksCreated
	summary["chunks_quarantined"] = scanReport.ChunksQuarantined
	summary["chunks_rejected"] = scanReport.ChunksRejected
	result.CompletedAt = time.Now().UTC().Unix()
	return nil
}

func (n *IngestionNode) ensureStore(ctx context.Context) (*knowledge.ChunkStore, func(), error) {
	tempDir, err := os.MkdirTemp("", "euclo-ingestion-*")
	if err != nil {
		return nil, nil, err
	}
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(tempDir))
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, nil, err
	}
	cleanup := func() {
		_ = engine.Close(ctx)
		_ = os.RemoveAll(tempDir)
	}
	return &knowledge.ChunkStore{Graph: engine}, cleanup, nil
}

func (n *IngestionNode) policyBundle() *execctx.ContextPolicyBundle {
	return &execctx.ContextPolicyBundle{
		DefaultTrustClass: agentspec.TrustClassBuiltinTrusted,
	}
}

func (n *IngestionNode) resolvePath(root, filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return filePath
	}
	if filepath.IsAbs(filePath) || root == "" {
		return filePath
	}
	return filepath.Join(root, filePath)
}

func (n *IngestionNode) writeSummary(env *contextdata.Envelope, result *IngestionResult, summary map[string]any, err error) {
	env.SetWorkingValueWithClass("euclo.ingestion_result", result, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("euclo.ingestion.summary", summary, contextdata.MemoryClassTask)
	if err != nil {
		env.SetWorkingValueWithClass("euclo.ingestion.error", err.Error(), contextdata.MemoryClassTask)
	}
}

func (n *IngestionNode) storeFileSummary(env *contextdata.Envelope, path string, ingestResult *frameworkingestion.IngestResult) {
	key := "euclo.ingested.file." + sanitize(path)
	env.SetWorkingValueWithClass(key, map[string]any{
		"chunks_committed":   ingestResult.ChunksCommitted,
		"chunks_quarantined": ingestResult.ChunksQuarantined,
		"chunks_rejected":    ingestResult.ChunksRejected,
	}, contextdata.MemoryClassTask)
}

func taskEnvelopeFromEnv(env *contextdata.Envelope) *intake.TaskEnvelope {
	if task, ok := contextdata.GetTyped[*intake.TaskEnvelope](env, "euclo.task_envelope"); ok {
		return task
	}
	return nil
}

func defaultPrincipal(task *intake.TaskEnvelope) identity.SubjectRef {
	if task == nil {
		return identity.SubjectRef{ID: "euclo"}
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return identity.SubjectRef{ID: "euclo"}
	}
	return identity.SubjectRef{ID: task.TaskID}
}

func summaryInt(summary map[string]any, key string) int {
	if summary == nil {
		return 0
	}
	if value, ok := summary[key]; ok {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case uint64:
			return int(v)
		}
	}
	return 0
}

func containsPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func contentHash(path string) string {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
