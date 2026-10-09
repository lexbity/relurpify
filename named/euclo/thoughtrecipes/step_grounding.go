package thoughtrecipe

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// groundingContextCap bounds the streamed-context sources carried into a
// grounding item.
const groundingContextCap = 16

// enqueueCaptureItems builds grounding items for finished capture bindings and
// hands them to the run's capture sink. Items targeting scratch.* are never
// grounded (FR-1). When no sink is present in the node context the batch is
// dropped with an explicit event — never silently.
func (c *stepCore) enqueueCaptureItems(ctx context.Context, env *contextdata.Envelope, bindings []CaptureBinding, resultData map[string]any) {
	items, downgrades := buildCaptureItems(c.step, env, bindings, resultData)
	for _, downgrade := range downgrades {
		c.emitGroundingEvent(fwtelemetry.EventCaptureEpistemicsDowngraded, "capture epistemics downgraded", map[string]any{
			"node_id":   c.step.ID,
			"state_key": downgrade,
		})
	}
	if len(items) == 0 {
		return
	}
	sink := agentgraph.CaptureSinkFromContext(ctx)
	if sink == nil {
		c.emitGroundingEvent(fwtelemetry.EventCaptureSinkAbsent, "capture sink absent", map[string]any{
			"node_id":  c.step.ID,
			"captures": len(items),
		})
		return
	}
	for _, item := range items {
		sink.EnqueueCapture(item)
	}
}

// enqueueToolResult builds a tool-result grounding item (origin tool, kind
// tool) and enqueues it through the capture sink. It replaces the
// fire-and-forget async ingestion path.
func (c *stepCore) enqueueToolResult(ctx context.Context, env *contextdata.Envelope, data map[string]any) {
	item := buildToolResultItem(c.step, env, data)
	sink := agentgraph.CaptureSinkFromContext(ctx)
	if sink == nil {
		c.emitGroundingEvent(fwtelemetry.EventCaptureSinkAbsent, "capture sink absent", map[string]any{
			"node_id": c.step.ID,
			"kind":    "tool",
		})
		return
	}
	sink.EnqueueCapture(item)
}

// buildCaptureItems resolves one grounding item per finished grounding-capable
// binding, plus the list of state keys whose `given` claim was downgraded.
func buildCaptureItems(step ExecutionStep, env *contextdata.Envelope, bindings []CaptureBinding, resultData map[string]any) ([]knowledge.GroundingItem, []string) {
	if env == nil || len(bindings) == 0 {
		return nil, nil
	}
	items := make([]knowledge.GroundingItem, 0, len(bindings))
	var downgrades []string
	sourceData := env.Snapshot()
	for _, binding := range bindings {
		dest := CaptureDestinationKey(binding)
		if !groundingDestination(dest) {
			continue // scratch.* never grounds
		}
		value, ok := CaptureSourceValue(binding, resultData, sourceData)
		if !ok {
			value = resultData
		}
		epistemics, downgraded := resolveCaptureEpistemics(env, binding)
		if downgraded {
			downgrades = append(downgrades, dest)
		}
		item := knowledge.GroundingItem{
			Value:          value,
			TypeAnnotation: typeAnnotationName(binding.Annotation),
			Epistemics:     knowledge.Epistemics(epistemics),
			Origin:         captureOriginFloor(env, step.Sources, binding),
			StateKey:       dest,
			NodeID:         step.ID,
			TaskID:         env.TaskID,
			SessionID:      env.SessionID,
			WorkspaceID:    workspaceIDFromEnvelope(env),
			RecipeID:       recipeIDFromEnvelope(env),
			Kind:           knowledge.ChunkKindCapture,
			SourceChunkIDs: cappedChunkIDs(env.StreamedChunkIDs(), groundingContextCap),
		}
		items = append(items, item)
	}
	return items, downgrades
}

func buildToolResultItem(step ExecutionStep, env *contextdata.Envelope, data map[string]any) knowledge.GroundingItem {
	if env == nil {
		env = contextdata.NewEnvelope("", "")
	}
	return knowledge.GroundingItem{
		Value:          data,
		Epistemics:     knowledge.EpistemicClaimed,
		Origin:         contextdata.OriginTool,
		NodeID:         step.ID,
		TaskID:         env.TaskID,
		SessionID:      env.SessionID,
		WorkspaceID:    workspaceIDFromEnvelope(env),
		RecipeID:       recipeIDFromEnvelope(env),
		Kind:           knowledge.ChunkKindTool,
		SourceChunkIDs: cappedChunkIDs(env.StreamedChunkIDs(), groundingContextCap),
	}
}

// resolveCaptureEpistemics applies the epistemic annotation at runtime: `given`
// is honored only when the captured value's dataflow origin is user; otherwise
// it downgrades to claimed.
func resolveCaptureEpistemics(env *contextdata.Envelope, binding CaptureBinding) (string, bool) {
	if binding.Epistemics == nil || binding.Epistemics.Value != "given" {
		return "claimed", false
	}
	if captureSourceHasUserOrigin(env, binding) {
		return "given", false
	}
	return "claimed", true
}

// captureSourceHasUserOrigin reports whether the source value's recorded
// origin class is user.
func captureSourceHasUserOrigin(env *contextdata.Envelope, binding CaptureBinding) bool {
	path, ok := valueExprPath(binding.Source)
	if !ok {
		return false
	}
	if env != nil {
		if env.OriginOf(path.Raw) == contextdata.OriginUser {
			return true
		}
		if len(path.Parts) > 0 && env.OriginOf(path.Parts[len(path.Parts)-1].Value) == contextdata.OriginUser {
			return true
		}
	}
	if len(path.Parts) > 0 && path.Parts[0].Value == "user" {
		return true
	}
	return false
}

// captureOriginFloor computes the most restrictive origin among the step's
// from-sources and the capture's own source (§D4). Unrecorded sources resolve
// to the llm default, so undifferentiated dataflow never elevates.
func captureOriginFloor(env *contextdata.Envelope, stepSources []string, binding CaptureBinding) contextdata.OriginClass {
	floor := contextdata.OriginClass("")
	consider := func(origin contextdata.OriginClass) {
		if floor == "" {
			floor = origin
			return
		}
		floor = contextdata.MostRestrictive(floor, origin)
	}
	for _, source := range stepSources {
		consider(env.OriginOf(source))
	}
	path, ok := valueExprPath(binding.Source)
	if ok && env != nil {
		consider(env.OriginOf(path.Raw))
		if len(path.Parts) > 0 {
			consider(env.OriginOf(path.Parts[len(path.Parts)-1].Value))
		}
	}
	if floor == "" {
		return contextdata.OriginLLM
	}
	return floor
}

// groundingDestination reports whether a capture destination grounds: only
// state.* and output.* writes are durable memory (FR-1).
func groundingDestination(dest string) bool {
	dest = strings.TrimSpace(dest)
	return strings.HasPrefix(dest, "state.") || strings.HasPrefix(dest, "output.")
}

func workspaceIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if value, ok := contextdata.GetTyped[string](env, "input.workspace"); ok {
		return value
	}
	return ""
}

func recipeIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if value, ok := contextdata.GetTyped[string](env, euclokeys.KeyExecutionThoughtRecipe); ok {
		return value
	}
	return ""
}

func cappedChunkIDs(ids []contextdata.ChunkID, cap int) []knowledge.ChunkID {
	if cap <= 0 {
		return nil
	}
	out := make([]knowledge.ChunkID, 0, cap)
	seen := make(map[knowledge.ChunkID]struct{}, cap)
	for _, id := range ids {
		if id == "" {
			continue
		}
		chunkID := knowledge.ChunkID(id)
		if _, ok := seen[chunkID]; ok {
			continue
		}
		seen[chunkID] = struct{}{}
		out = append(out, chunkID)
		if len(out) >= cap {
			break
		}
	}
	return out
}

func (c *stepCore) emitGroundingEvent(eventType fwtelemetry.EventType, message string, metadata map[string]any) {
	if c == nil || c.deps == nil || c.deps.Telemetry == nil {
		return
	}
	c.deps.Telemetry.Emit(fwtelemetry.Event{
		Type:     eventType,
		Message:  message,
		Metadata: metadata,
	})
}
