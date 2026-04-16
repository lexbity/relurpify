package bkc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	archaeobindings "github.com/lexcodex/relurpify/archaeo/bindings/euclo"
	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

type compileCapability struct {
	env agentenv.AgentEnvironment
}

type streamCapability struct {
	env agentenv.AgentEnvironment
}

type checkpointCapability struct {
	env agentenv.AgentEnvironment
}

type invalidateCapability struct {
	env agentenv.AgentEnvironment
}

func NewCompileCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &compileCapability{env: env}
}

func NewStreamCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &streamCapability{env: env}
}

func NewCheckpointCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &checkpointCapability{env: env}
}

func NewInvalidateCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &invalidateCapability{env: env}
}

func (c *compileCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityBKCCompile,
		Name:          "BKC Compile",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning", "semantic", "bkc"},
	}
}

func (c *compileCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindSemanticCompile,
		},
	}
}

func (c *compileCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read access required for semantic compilation"}
	}
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if strings.Contains(text, "bkc") || strings.Contains(text, "semantic") || strings.Contains(text, "knowledge chunk") || strings.Contains(text, "compile") {
		return euclotypes.EligibilityResult{Eligible: true, Reason: "instruction requests semantic compilation"}
	}
	return euclotypes.EligibilityResult{Eligible: false, Reason: "semantic compilation intent not detected"}
}

func (c *compileCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	workflowStore, ok := env.WorkflowStore.(memory.WorkflowStateStore)
	if !ok || workflowStore == nil {
		return failureResult("bkc_compile_store_missing", "workflow store required for bkc compilation")
	}
	if c.env.IndexManager == nil || c.env.IndexManager.GraphDB == nil {
		return failureResult("bkc_compile_graph_missing", "graphdb required for bkc compilation")
	}
	workflowID := firstNonEmpty(strings.TrimSpace(env.WorkflowID), taskContextString(env.Task, "workflow_id"))
	explorationID := taskContextString(env.Task, "exploration_id")
	workspaceID := firstNonEmpty(taskContextString(env.Task, "workspace_id"), taskContextString(env.Task, "workspace"))
	if workflowID == "" || explorationID == "" || workspaceID == "" {
		return failureResult("bkc_compile_context_missing", "workflow_id, exploration_id, and workspace_id are required")
	}
	runtime := archaeobindings.Runtime{WorkflowStore: workflowStore}
	compiler := &archaeobkc.LLMCompiler{
		Store:         &archaeobkc.ChunkStore{Graph: c.env.IndexManager.GraphDB},
		WorkflowStore: workflowStore,
		Learning:      runtime.LearningService(),
		Deferred:      runtime.DeferredDraftService(),
		Model:         c.env.Model,
	}
	related := chunkIDsFromAny(env.Task.Context["related_chunk_ids"])
	result, err := compiler.Propose(ctx, archaeobkc.LLMCompileInput{
		WorkspaceID:     workspaceID,
		WorkflowID:      workflowID,
		ExplorationID:   explorationID,
		SnapshotID:      taskContextString(env.Task, "snapshot_id"),
		BasedOnRevision: taskContextString(env.Task, "based_on_revision"),
		SubjectRef:      taskContextString(env.Task, "subject_ref"),
		Title:           taskContextString(env.Task, "title"),
		Description:     taskContextString(env.Task, "description"),
		Prompt:          taskInstruction(env.Task),
		RelatedChunkIDs: related,
		SessionID:       taskContextString(env.Task, "session_id"),
		Blocking:        taskContextBool(env.Task, "blocking"),
	})
	if err != nil {
		return failureResult("bkc_compile_failed", err.Error())
	}
	payload := map[string]any{
		"candidate_id":   result.Candidate.ID,
		"interaction_id": result.Interaction.ID,
		"chunk_id":       string(result.Candidate.Chunk.ID),
		"status":         string(result.Candidate.Status),
		"summary":        firstNonEmpty(result.Interaction.Description, result.Candidate.Chunk.Body.Raw),
	}
	artifact := euclotypes.Artifact{
		ID:         "bkc_compile",
		Kind:       euclotypes.ArtifactKindSemanticCompile,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: euclorelurpic.CapabilityBKCCompile,
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "queued BKC chunk candidate for confirmation",
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *streamCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityBKCStream,
		Name:          "BKC Stream",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning", "semantic", "context", "bkc"},
	}
}

func (c *checkpointCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityBKCCheckpoint,
		Name:          "BKC Checkpoint",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"planning", "semantic", "checkpoint", "bkc"},
	}
}

func (c *checkpointCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindCompiledExecution,
		},
	}
}

func (c *checkpointCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read access required for BKC checkpointing"}
	}
	if artifacts.Has(euclotypes.ArtifactKindSemanticContext) || artifacts.Has(euclotypes.ArtifactKindSemanticCompile) {
		return euclotypes.EligibilityResult{Eligible: true, Reason: "semantic context available for checkpointing"}
	}
	return euclotypes.EligibilityResult{Eligible: false, Reason: "no semantic chunk context available"}
}

func (c *checkpointCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	workflowStore, ok := env.WorkflowStore.(memory.WorkflowStateStore)
	if !ok || workflowStore == nil {
		return failureResult("bkc_checkpoint_store_missing", "workflow store required for bkc checkpointing")
	}
	if env.PlanStore == nil {
		return failureResult("bkc_checkpoint_plan_missing", "plan store required for bkc checkpointing")
	}
	workflowID := firstNonEmpty(strings.TrimSpace(env.WorkflowID), taskContextString(env.Task, "workflow_id"))
	if workflowID == "" {
		return failureResult("bkc_checkpoint_workflow_missing", "workflow_id is required")
	}
	version := env.PlanVersion
	if version <= 0 {
		version = taskContextInt(env.Task, "plan_version", 0)
	}
	if version <= 0 {
		version = taskContextInt(env.Task, "active_plan_version", 0)
	}
	rootChunkIDs := append([]string(nil), env.RootChunkIDs...)
	if len(rootChunkIDs) == 0 {
		rootChunkIDs = append(rootChunkIDs, taskContextStringSlice(env.Task, "root_chunk_ids")...)
	}
	if len(rootChunkIDs) == 0 && env.State != nil {
		if chunks, ok := env.State.Get(euclostate.KeyBKCContextChunks); ok && chunks != nil {
			rootChunkIDs = append(rootChunkIDs, contextChunkIDsFromValue(chunks)...)
		}
	}
	if len(rootChunkIDs) == 0 {
		return failureResult("bkc_checkpoint_roots_missing", "root chunk ids required for checkpointing")
	}
	planSvc := archaeoplans.Service{Store: env.PlanStore, WorkflowStore: workflowStore}
	plan, err := planSvc.AnchorChunks(ctx, workflowID, version, rootChunkIDs, firstNonEmpty(env.ChunkStateRef, taskContextString(env.Task, "chunk_state_ref")))
	if err != nil {
		return failureResult("bkc_checkpoint_failed", err.Error())
	}
	checkpointRef := firstNonEmpty(env.ChunkStateRef, fmt.Sprintf("%s:%d", workflowID, version))
	payload := map[string]any{
		"workflow_id":      workflowID,
		"plan_version":     version,
		"root_chunk_ids":   rootChunkIDs,
		"chunk_state_ref":  checkpointRef,
		"anchored_plan_id": firstNonEmpty(env.PlanID, workflowID),
	}
	artifact := euclotypes.Artifact{
		ID:         "bkc_checkpoint",
		Kind:       euclotypes.ArtifactKindCompiledExecution,
		Summary:    fmt.Sprintf("anchored %d root chunks for plan version %d", len(rootChunkIDs), version),
		Payload:    payload,
		ProducerID: euclorelurpic.CapabilityBKCCheckpoint,
		Status:     "produced",
	}
	if env.State != nil {
		env.State.Set(euclostate.KeyBKCRootChunkIDs, rootChunkIDs)
		env.State.Set(euclostate.KeyBKCCheckpointRef, checkpointRef)
		env.State.Set(euclostate.KeyBKCCheckpointVersion, version)
		if plan != nil {
			env.State.Set(euclostate.KeyLivingPlan, plan)
		}
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   artifact.Summary,
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *invalidateCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityBKCInvalidate,
		Name:          "BKC Invalidate",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"planning", "semantic", "invalidate", "bkc"},
	}
}

func (c *invalidateCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindContextCompaction,
		},
	}
}

func (c *invalidateCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read access required for BKC invalidation"}
	}
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if strings.Contains(text, "invalidate") || strings.Contains(text, "stale") || strings.Contains(text, "revision") || strings.Contains(text, "chunk") {
		return euclotypes.EligibilityResult{Eligible: true, Reason: "invalidation intent detected"}
	}
	return euclotypes.EligibilityResult{Eligible: false, Reason: "invalidation intent not detected"}
}

func (c *invalidateCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	workflowStore, ok := env.WorkflowStore.(memory.WorkflowStateStore)
	if !ok || workflowStore == nil {
		return failureResult("bkc_invalidate_store_missing", "workflow store required for bkc invalidation")
	}
	if c.env.IndexManager == nil || c.env.IndexManager.GraphDB == nil {
		return failureResult("bkc_invalidate_graph_missing", "graphdb required for bkc invalidation")
	}
	workflowID := firstNonEmpty(strings.TrimSpace(env.WorkflowID), taskContextString(env.Task, "workflow_id"))
	if workflowID == "" {
		return failureResult("bkc_invalidate_workflow_missing", "workflow_id is required")
	}
	store := &archaeobkc.ChunkStore{Graph: c.env.IndexManager.GraphDB}
	pass := &archaeobkc.InvalidationPass{
		Store:    store,
		Tensions: archaeotensions.Service{Store: workflowStore},
	}
	paths := append(taskContextStringSlice(env.Task, "changed_paths"), taskContextStringSlice(env.Task, "files")...)
	revision := firstNonEmpty(taskContextString(env.Task, "based_on_revision"), taskContextString(env.Task, "code_revision"))
	if env.State != nil {
		if text := env.State.GetString(euclostate.KeyCodeRevision); text != "" {
			revision = text
		}
	}
	if len(paths) > 0 {
		if err := pass.HandleRevisionChanged(ctx, archaeobkc.CodeRevisionChangedPayload{
			WorkspaceRoot: taskContextString(env.Task, "workspace"),
			NewRevision:   revision,
			AffectedPaths: uniqueStrings(paths),
		}); err != nil {
			return failureResult("bkc_invalidate_failed", err.Error())
		}
	} else {
		chunkIDs := append([]string(nil), env.RootChunkIDs...)
		if len(chunkIDs) == 0 {
			chunkIDs = append(chunkIDs, taskContextStringSlice(env.Task, "stale_chunk_ids")...)
		}
		if len(chunkIDs) == 0 && env.State != nil {
			chunkIDs = append(chunkIDs, stringSlice(env.State.GetString(euclostate.KeyBKCRootChunkIDs))...)
		}
		if len(chunkIDs) == 0 {
			return failureResult("bkc_invalidate_targets_missing", "affected paths or chunk ids required")
		}
		if err := pass.SurfaceStaleChunks(ctx, uniqueStrings(chunkIDs), nil, "manual_invalidation"); err != nil {
			return failureResult("bkc_invalidate_failed", err.Error())
		}
		if env.State != nil {
			env.State.Set(euclostate.KeyBKCStaleChunkIDs, uniqueStrings(chunkIDs))
		}
	}
	payload := map[string]any{
		"workflow_id":    workflowID,
		"affected_paths": uniqueStrings(paths),
		"revision":       revision,
		"root_chunk_ids": append([]string(nil), env.RootChunkIDs...),
	}
	artifact := euclotypes.Artifact{
		ID:         "bkc_invalidate",
		Kind:       euclotypes.ArtifactKindContextCompaction,
		Summary:    "surfaces stale BKC chunks and tensions",
		Payload:    payload,
		ProducerID: euclorelurpic.CapabilityBKCInvalidate,
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   artifact.Summary,
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (c *streamCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindSemanticContext,
		},
	}
}

func (c *streamCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read access required for BKC streaming"}
	}
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if strings.Contains(text, "stream") || strings.Contains(text, "context") || strings.Contains(text, "semantic") || strings.Contains(text, "bkc") {
		return euclotypes.EligibilityResult{Eligible: true, Reason: "instruction requests semantic context streaming"}
	}
	return euclotypes.EligibilityResult{Eligible: false, Reason: "semantic streaming intent not detected"}
}

func (c *streamCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if c.env.IndexManager == nil || c.env.IndexManager.GraphDB == nil {
		return failureResult("bkc_stream_graph_missing", "graphdb required for bkc streaming")
	}
	store := &archaeobkc.ChunkStore{Graph: c.env.IndexManager.GraphDB}
	streamer := &archaeobkc.Streamer{Store: store}
	seed, err := streamSeedForTask(streamer, env.Task)
	if err != nil {
		return failureResult("bkc_stream_seed_failed", err.Error())
	}
	budget := taskContextInt(env.Task, "max_tokens", 1200)
	result, err := streamer.Stream(ctx, seed, budget)
	if err != nil {
		return failureResult("bkc_stream_failed", err.Error())
	}
	if len(result.StaleDuringStream) > 0 && env.State != nil {
		env.State.Set(euclostate.KeyBKCStaleChunkIDs, chunkIDsToStrings(result.StaleDuringStream))
		env.State.Set(euclostate.KeyBKCStaleGapMessages, append([]string(nil), result.StaleGapMessages...))
	}
	if len(result.StaleDuringStream) > 0 {
		pass := &archaeobkc.InvalidationPass{
			Store:    store,
			Tensions: archaeotensions.Service{Store: workflowStoreFromEnvelope(env)},
		}
		if err := pass.SurfaceStaleDuringStream(ctx, result); err != nil {
			_ = err
		}
	}
	contextChunks := archaeobkc.ToContextChunks(result.Chunks)
	payload := map[string]any{
		"chunk_count":         len(result.Chunks),
		"token_total":         result.TokenTotal,
		"stale_during_stream": chunkIDsToStrings(result.StaleDuringStream),
		"stale_gap_messages":  append([]string(nil), result.StaleGapMessages...),
		"context_chunks":      contextChunks,
	}
	artifact := euclotypes.Artifact{
		ID:         "bkc_stream",
		Kind:       euclotypes.ArtifactKindSemanticContext,
		Summary:    fmt.Sprintf("streamed %d BKC chunks into context", len(result.Chunks)),
		Payload:    payload,
		ProducerID: euclorelurpic.CapabilityBKCStream,
		Status:     "produced",
	}
	if env.State != nil {
		env.State.Set(euclostate.KeyBKCContextChunks, contextChunks)
	}
	artifacts := []euclotypes.Artifact{artifact}
	if len(result.StaleDuringStream) > 0 {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "bkc_stream_gap",
			Kind:       euclotypes.ArtifactKindTension,
			Summary:    fmt.Sprintf("%d BKC chunks were stale and excluded from context", len(result.StaleDuringStream)),
			Payload:    map[string]any{"gap_messages": append([]string(nil), result.StaleGapMessages...)},
			ProducerID: euclorelurpic.CapabilityBKCStream,
			Status:     "gap",
		})
	}
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   artifact.Summary,
		Artifacts: artifacts,
	}
}

func streamSeedForTask(streamer *archaeobkc.Streamer, task *core.Task) (archaeobkc.StreamSeed, error) {
	if task == nil {
		return archaeobkc.StreamSeed{}, nil
	}
	modeID := strings.ToLower(taskContextString(task, "mode_id"))
	rootChunkIDs := uniqueStrings(append(stringSlice(task.Context["root_chunk_ids"]), stringSlice(task.Context["active_plan_root_chunk_ids"])...))
	files := uniqueStrings(stringSlice(task.Context["files"]))
	tensions := uniqueStrings(stringSlice(task.Context["tension_refs"]))
	learningRefs := uniqueStrings(stringSlice(task.Context["learning_interaction_refs"]))
	if len(rootChunkIDs) > 0 {
		return streamer.PlanningSeed(rootChunkIDs), nil
	}
	switch modeID {
	case "planning", "review":
		if len(tensions) > 0 || len(learningRefs) > 0 {
			return streamer.ArchaeologySeed(uniqueStrings(append(append([]string(nil), tensions...), learningRefs...))), nil
		}
		if len(files) > 0 {
			seed, err := streamer.ChatSeed(files)
			return seed, err
		}
	case "debug":
		if len(tensions) > 0 {
			seed, err := streamer.DebugSeed(files, tensions)
			return seed, err
		}
	default:
		if len(learningRefs) > 0 {
			return streamer.ArchaeologySeed(learningRefs), nil
		}
		if len(tensions) > 0 {
			seed, err := streamer.DebugSeed(files, tensions)
			return seed, err
		}
		if len(files) > 0 {
			seed, err := streamer.ChatSeed(files)
			return seed, err
		}
	}
	return archaeobkc.StreamSeed{}, nil
}

func failureResult(code, message string) euclotypes.ExecutionResult {
	return euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusFailed,
		Summary: message,
		FailureInfo: &euclotypes.CapabilityFailure{
			Code:        code,
			Message:     message,
			Recoverable: true,
			FailedPhase: "semantic",
		},
	}
}

func workflowStoreFromEnvelope(env euclotypes.ExecutionEnvelope) memory.WorkflowStateStore {
	workflowStore, _ := env.WorkflowStore.(memory.WorkflowStateStore)
	return workflowStore
}

func instructionFromArtifacts(artifacts euclotypes.ArtifactState) string {
	for _, artifact := range artifacts.All() {
		if artifact.Kind != euclotypes.ArtifactKindIntake {
			continue
		}
		if summary := strings.TrimSpace(artifact.Summary); summary != "" {
			return summary
		}
		if payload, ok := artifact.Payload.(map[string]any); ok {
			if instruction := strings.TrimSpace(stringValue(payload["instruction"])); instruction != "" {
				return instruction
			}
		}
	}
	return ""
}

func taskInstruction(task *core.Task) string {
	if task == nil || strings.TrimSpace(task.Instruction) == "" {
		return "the requested change"
	}
	return strings.TrimSpace(task.Instruction)
}

func taskContextString(task *core.Task, key string) string {
	if task == nil || task.Context == nil {
		return ""
	}
	value, ok := task.Context[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func taskContextBool(task *core.Task, key string) bool {
	if task == nil || task.Context == nil {
		return false
	}
	value, ok := task.Context[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func taskContextInt(task *core.Task, key string, fallback int) int {
	if task == nil || task.Context == nil {
		return fallback
	}
	switch typed := task.Context[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var value int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &value); err == nil {
			return value
		}
	}
	return fallback
}

func taskContextStringSlice(task *core.Task, key string) []string {
	if task == nil || task.Context == nil {
		return nil
	}
	return stringSlice(task.Context[key])
}

func mergeStateArtifactsToContext(state *core.Context, artifacts []euclotypes.Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	existing := euclotypes.ArtifactStateFromContext(state).All()
	merged := append(existing, artifacts...)
	euclostate.SetArtifacts(state, merged)
	for _, artifact := range artifacts {
		if key := euclotypes.StateKeyForArtifactKind(artifact.Kind); key != "" && artifact.Payload != nil {
			state.Set(key, artifact.Payload)
		}
	}
}

func summarizePayload(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return encodePayload(value)
	}
}

func encodePayload(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if text := strings.TrimSpace(typed); text != "" && text != "<nil>" {
			return []string{text}
		}
		return nil
	default:
		return nil
	}
}

func chunkIDsFromAny(raw any) []archaeobkc.ChunkID {
	values := stringSlice(raw)
	if len(values) == 0 {
		return nil
	}
	out := make([]archaeobkc.ChunkID, 0, len(values))
	for _, value := range values {
		out = append(out, archaeobkc.ChunkID(value))
	}
	return out
}

func chunkIDsToStrings(ids []archaeobkc.ChunkID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(string(id)) != "" {
			out = append(out, string(id))
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func contextChunkIDsFromValue(raw any) []string {
	switch typed := raw.(type) {
	case []contextmgr.ContextChunk:
		out := make([]string, 0, len(typed))
		for _, chunk := range typed {
			if id := strings.TrimSpace(chunk.ID); id != "" {
				out = append(out, id)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			switch chunk := item.(type) {
			case contextmgr.ContextChunk:
				if id := strings.TrimSpace(chunk.ID); id != "" {
					out = append(out, id)
				}
			case map[string]any:
				if id := strings.TrimSpace(fmt.Sprint(chunk["id"])); id != "" && id != "<nil>" {
					out = append(out, id)
				}
			}
		}
		return out
	case map[string]any:
		if id := strings.TrimSpace(fmt.Sprint(typed["id"])); id != "" && id != "<nil>" {
			return []string{id}
		}
		return nil
	default:
		return nil
	}
}
