package agentgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// CompressedContext holds compressed checkpoint data for history restoration.
type CompressedContext struct {
	Summary string   `json:"summary,omitempty"`
	Chunks  []string `json:"chunks,omitempty"`
}

// CompressionStrategy defines how to compress checkpoint history.
type CompressionStrategy interface {
	ShouldCompress(env *contextdata.Envelope, hint any) bool
	Compress(history []any, llm LanguageModel) (*CompressedContext, error)
}

// NodeTransitionRecord captures a completed node and the next resume boundary.
type NodeTransitionRecord struct {
	FromNodeID       string    `json:"from_node_id,omitempty"`
	CompletedNodeID  string    `json:"completed_node_id,omitempty"`
	NextNodeID       string    `json:"next_node_id,omitempty"`
	TransitionReason string    `json:"transition_reason,omitempty"`
	CompletedAt      time.Time `json:"completed_at"`
}

// CheckpointResultSummary stores lightweight result metadata for safe terminal
// resume without re-running completed work.
type CheckpointResultSummary struct {
	NodeID   string   `json:"node_id,omitempty"`
	Success  bool     `json:"success"`
	Error    string   `json:"error,omitempty"`
	DataKeys []string `json:"data_keys,omitempty"`
}

// GraphCheckpoint captures graph execution state for resumable workflows.
type GraphCheckpoint struct {
	CheckpointID      string                   `json:"checkpoint_id"`
	TaskID            string                   `json:"task_id"`
	CreatedAt         time.Time                `json:"created_at"`
	CurrentNodeID     string                   `json:"current_node_id,omitempty"`
	CompletedNodeID   string                   `json:"completed_node_id,omitempty"`
	NextNodeID        string                   `json:"next_node_id,omitempty"`
	LastTransition    *NodeTransitionRecord    `json:"last_transition,omitempty"`
	LastResultSummary *CheckpointResultSummary `json:"last_result_summary,omitempty"`
	VisitCounts       map[string]int           `json:"visit_counts"`
	ExecutionPath     []string                 `json:"execution_path"`
	Context           *contextdata.Envelope    `json:"context"`
	CompressedContext *CompressedContext       `json:"compressed_context,omitempty"`
	GraphHash         string                   `json:"graph_hash"`
	Metadata          map[string]interface{}   `json:"metadata"`
}

// CreateCheckpoint captures a transition-boundary execution state for later resumption.
func (g *Graph) CreateCheckpoint(taskID, completedNodeID, nextNodeID string, result *Result, transition *NodeTransitionRecord, env *contextdata.Envelope) (*GraphCheckpoint, error) {
	if env == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	ctxClone := checkpointEnvelopeClone(env)
	if transition == nil {
		transition = &NodeTransitionRecord{
			CompletedNodeID: completedNodeID,
			NextNodeID:      nextNodeID,
			CompletedAt:     time.Now().UTC(),
		}
	}
	checkpoint := &GraphCheckpoint{
		CheckpointID:      generateCheckpointID(),
		TaskID:            taskID,
		CreatedAt:         time.Now().UTC(),
		CurrentNodeID:     completedNodeID,
		CompletedNodeID:   completedNodeID,
		NextNodeID:        nextNodeID,
		LastTransition:    cloneTransitionRecord(transition),
		LastResultSummary: summarizeCheckpointResult(result, completedNodeID),
		VisitCounts:       g.copyVisitCounts(),
		ExecutionPath:     g.copyExecutionPath(),
		Context:           ctxClone,
		GraphHash:         g.computeHash(),
		Metadata:          make(map[string]interface{}),
	}
	if telemetry, ok := g.telemetry.(CheckpointTelemetry); ok {
		telemetry.OnCheckpointCreated(taskID, checkpoint.CheckpointID, checkpoint.resumeNodeID())
	}
	return checkpoint, nil
}

// CreateCompressedCheckpoint captures a checkpoint while compressing history.
func (g *Graph) CreateCompressedCheckpoint(taskID, completedNodeID, nextNodeID string, result *Result, transition *NodeTransitionRecord, env *contextdata.Envelope, llm LanguageModel, strategy CompressionStrategy) (*GraphCheckpoint, error) {
	checkpoint, err := g.CreateCheckpoint(taskID, completedNodeID, nextNodeID, result, transition, env)
	if err != nil {
		return nil, err
	}
	if strategy == nil || llm == nil {
		return checkpoint, nil
	}
	if !strategy.ShouldCompress(env, nil) {
		return checkpoint, nil
	}
	// History is stored in working memory under "_history" key as []any
	var historyCopy []any
	if h, ok := env.GetWorkingValue("_history"); ok {
		if hSlice, ok := h.([]any); ok {
			historyCopy = hSlice
		}
	}
	if len(historyCopy) == 0 {
		return checkpoint, nil
	}
	compressed, err := strategy.Compress(historyCopy, llm)
	if err != nil {
		return nil, fmt.Errorf("failed to compress checkpoint: %w", err)
	}
	checkpoint.CompressedContext = compressed
	// Trim history - keep only last 5 entries
	if len(historyCopy) > 5 {
		env.SetWorkingValue("_history", historyCopy[len(historyCopy)-5:], contextdata.MemoryClassEphemeral)
	}
	return checkpoint, nil
}

// ResumeFromCheckpoint validates and resumes from the provided checkpoint.
func (g *Graph) ResumeFromCheckpoint(ctx context.Context, checkpoint *GraphCheckpoint) (*Result, error) {
	if checkpoint == nil {
		return nil, fmt.Errorf("nil checkpoint")
	}
	if g.computeHash() != checkpoint.GraphHash {
		return nil, fmt.Errorf("graph definition has changed since checkpoint")
	}
	state := checkpoint.Context
	if state == nil {
		state = contextdata.NewEnvelope(checkpoint.TaskID, "")
	}
	if checkpoint.CompressedContext != nil {
		// Store compressed context in working memory
		state.SetWorkingValue("_compressed_context", *checkpoint.CompressedContext, contextdata.MemoryClassTask)
	}
	g.execMu.Lock()
	g.visitCounts = make(map[string]int)
	for node, count := range checkpoint.VisitCounts {
		g.visitCounts[node] = count
	}
	g.executionPath = append([]string(nil), checkpoint.ExecutionPath...)
	g.lastCheckpointNode = checkpoint.CompletedNodeID
	g.nodesSinceCheckpoint = 0
	g.execMu.Unlock()
	if telemetry, ok := g.telemetry.(CheckpointTelemetry); ok {
		telemetry.OnCheckpointRestored(checkpoint.TaskID, checkpoint.CheckpointID)
		telemetry.OnGraphResume(checkpoint.TaskID, checkpoint.CheckpointID, checkpoint.resumeNodeID())
	}
	if checkpoint.CompletedNodeID == "" && checkpoint.NextNodeID == "" {
		return nil, fmt.Errorf("checkpoint missing resume boundary")
	}
	if checkpoint.NextNodeID == "" {
		return checkpoint.resultFromSummary(), nil
	}
	return g.run(ctx, state, checkpoint.NextNodeID, false, checkpoint.TaskID)
}

func generateCheckpointID() string {
	return fmt.Sprintf("ckpt_%d", time.Now().UnixNano())
}

func (g *Graph) copyVisitCounts() map[string]int {
	copyMap := make(map[string]int, len(g.visitCounts))
	for k, v := range g.visitCounts {
		copyMap[k] = v
	}
	return copyMap
}

func (g *Graph) copyExecutionPath() []string {
	return append([]string(nil), g.executionPath...)
}

func summarizeCheckpointResult(result *Result, completedNodeID string) *CheckpointResultSummary {
	if result == nil {
		if completedNodeID == "" {
			return nil
		}
		return &CheckpointResultSummary{NodeID: completedNodeID, Success: true}
	}
	keys := make([]string, 0, len(result.Data))
	for key := range result.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return &CheckpointResultSummary{
		NodeID:   result.NodeID,
		Success:  result.Success,
		Error:    result.Error,
		DataKeys: keys,
	}
}

func cloneTransitionRecord(record *NodeTransitionRecord) *NodeTransitionRecord {
	if record == nil {
		return nil
	}
	copy := *record
	return &copy
}

func (c *GraphCheckpoint) resumeNodeID() string {
	if c == nil {
		return ""
	}
	if c.NextNodeID != "" {
		return c.NextNodeID
	}
	if c.CompletedNodeID != "" {
		return c.CompletedNodeID
	}
	return c.CurrentNodeID
}

func (c *GraphCheckpoint) resultFromSummary() *Result {
	if c == nil {
		return &Result{Success: true, Data: map[string]interface{}{}}
	}
	if c.LastResultSummary == nil {
		return &Result{NodeID: c.CompletedNodeID, Success: true, Data: map[string]interface{}{}}
	}
	return &Result{
		NodeID:  c.LastResultSummary.NodeID,
		Success: c.LastResultSummary.Success,
		Data:    map[string]interface{}{},
		Error:   c.LastResultSummary.Error,
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (g *Graph) computeHash() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nodeIDs := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	var sb strings.Builder
	for _, id := range nodeIDs {
		sb.WriteString(id)
	}
	edgeKeys := make([]string, 0, len(g.edges))
	for id := range g.edges {
		edgeKeys = append(edgeKeys, id)
	}
	sort.Strings(edgeKeys)
	for _, from := range edgeKeys {
		for _, edge := range g.edges[from] {
			sb.WriteString(edge.From)
			sb.WriteString(edge.To)
		}
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

func checkpointEnvelopeClone(env *contextdata.Envelope) *contextdata.Envelope {
	if env == nil {
		return nil
	}
	// Use contextdata.CloneEnvelope for proper deep copy
	return contextdata.CloneEnvelope(env, "checkpoint-clone")
}
