package blackboard

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// authored_source.go implements the restored blackboard directive vocabulary
// (Wave 3 D6): authored `source` blocks replace the built-in specialist set for
// that execution and run as a controller loop with declaration-order
// eligibility, per-source episode re-arming, and capture-equivalent writes
// through the run's grounding path.
//
// Authored structure is authoritative (D1): the sources that run are exactly
// the authored ones, evaluated in declaration order, with no model calls beyond
// what a source's own execution performs (the no-`do` specialist). The zero
// value preserves the built-in specialist loop byte-for-byte (FR-9).

// AuthoredSource is one authored `source` block lowered to the runner boundary.
// When is the compiled eligibility predicate (nil means eligible exactly once);
// Read names the state keys injected as the source's read context; Write is the
// required durable state target; Capability optionally pins the invoked
// capability instead of the built-in LLM specialist.
type AuthoredSource struct {
	Name       string
	When       func(*contextdata.Envelope) bool
	WhenExpr   string
	Read       []string
	Write      string
	Capability string
}

// WithAuthoredSources installs the authored source set. Their presence switches
// the agent into authored mode; an empty list is a no-op (FR-9).
func WithAuthoredSources(specs []AuthoredSource) Option {
	return func(agent *BlackboardAgent) {
		if len(specs) == 0 {
			return
		}
		agent.authoredSources = append([]AuthoredSource(nil), specs...)
	}
}

// Envelope keys for per-run episode bookkeeping. All counters live on the
// envelope, so episodes are per-run state with no cross-run leakage.
const (
	episodeGenerationKey  = "blackboard.episode.generation"
	episodeArmedKeyPrefix = "blackboard.episode."
	episodeArmedSuffix    = ".armed"
	episodeRanSuffix      = ".ran"
	episodeDisarmedSuffix = ".disarmed"
)

// authoredNodeID names the authored controller in telemetry and grounding
// provenance.
const authoredNodeID = "blackboard.authored"

// authoredMode reports whether authored sources drive this execution.
func (a *BlackboardAgent) authoredMode() bool {
	return a != nil && len(a.authoredSources) > 0
}

// executeAuthored runs the authored controller loop: each cycle evaluates every
// source in declaration order and executes the eligible ones (D6). A cycle that
// executes nothing ends the loop (goal_satisfied); the cycle cap ends it as
// cycle_cap, which — matching the built-in controller — is an error, not a
// silent success.
func (a *BlackboardAgent) executeAuthored(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if env == nil {
		env = contextdata.NewEnvelope("blackboard", "session")
	}
	goal := taskInstruction(task)
	maxCycles := maxCycles(a.MaxCycles)
	executedOrder := []string{}
	cycles := 0
	termination := ""
	for cycle := 1; cycle <= maxCycles && termination == ""; cycle++ {
		envelopeSet(env, episodeGenerationKey, cycle)
		ran := 0
		for i := range a.authoredSources {
			source := a.authoredSources[i]
			if !a.sourceEligible(env, &source, cycle) {
				continue
			}
			if err := a.executeSource(ctx, env, source, goal, cycle); err != nil {
				return nil, fmt.Errorf("blackboard: source %q failed: %w", source.Name, err)
			}
			setEpisodeCount(env, source.Name, episodeRanSuffix, cycle)
			executedOrder = append(executedOrder, source.Name)
			ran++
		}
		if err := a.closeCycleBarrier(ctx, env); err != nil {
			return nil, err
		}
		if ran == 0 {
			termination = "goal_satisfied"
			break
		}
		cycles = cycle
		if cycle == maxCycles {
			termination = "cycle_cap"
		}
	}
	if termination == "" {
		termination = "goal_satisfied"
	}
	if termination == "cycle_cap" {
		return nil, fmt.Errorf("blackboard: reached cycle limit (%d) without quiescing the authored sources", maxCycles)
	}
	contextdata.SetTyped(env, "blackboard.sources_executed", executedOrder)
	contextdata.SetTyped(env, "blackboard.cycles", cycles)
	contextdata.SetTyped(env, "blackboard.termination", termination)
	return &execution.Result{
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"sources_executed": executedOrder,
			"cycles":           cycles,
			"termination":      termination,
		}),
	}, nil
}

// sourceEligible applies the D6 episode semantics: a `when`-gated source arms
// at the generation its predicate first holds and is eligible while
// armed > ran; execution sets ran = armed, so it runs once per arming. A
// predicate that turns false disarms the source; true again re-arms it at the
// new generation. A continuously-true predicate therefore fires exactly once,
// and a source with no `when` arms once at generation 1 — eligible exactly
// ever.
func (a *BlackboardAgent) sourceEligible(env *contextdata.Envelope, source *AuthoredSource, generation int) bool {
	armed, hasArmed := episodeCount(env, source.Name, episodeArmedSuffix)
	if source.When != nil {
		if !source.When(env) {
			setEpisodeCount(env, source.Name, episodeDisarmedSuffix, 1)
			return false
		}
		disarmed, _ := episodeCount(env, source.Name, episodeDisarmedSuffix)
		if !hasArmed || (disarmed == 1 && armed < generation) {
			armed = generation
			setEpisodeCount(env, source.Name, episodeArmedSuffix, armed)
			setEpisodeCount(env, source.Name, episodeDisarmedSuffix, 0)
		}
	} else if !hasArmed {
		armed = 1
		setEpisodeCount(env, source.Name, episodeArmedSuffix, armed)
	}
	ran, _ := episodeCount(env, source.Name, episodeRanSuffix)
	return armed > ran
}

// executeSource runs one eligible source and lands its product: the envelope
// write is synchronous (read-your-writes within the blackboard), and the
// grounding enqueue is the durable leg that flushes at the cycle barrier.
func (a *BlackboardAgent) executeSource(ctx context.Context, env *contextdata.Envelope, source AuthoredSource, goal string, cycle int) error {
	readContext, readValues := a.readContext(env, source)
	var (
		output any
		origin contextdata.OriginClass
		err    error
	)
	if source.Capability != "" {
		output, err = a.invokeCapability(ctx, env, source, goal, readContext)
		origin = contextdata.OriginTool
	} else {
		output, err = a.runSpecialist(ctx, env, source, goal, readContext)
		origin = contextdata.OriginLLM
	}
	if err != nil {
		return err
	}
	envelopeSet(env, source.Write, output)
	a.emitSourceEvent(ctx, env, source, cycle, readValues)
	a.enqueueWrite(ctx, env, source, output, origin, readValues)
	return nil
}

// readContext snapshots the declared read keys. A key that holds no value is
// recorded as an absent input (nil) — visible in provenance, never an error.
func (a *BlackboardAgent) readContext(env *contextdata.Envelope, source AuthoredSource) (map[string]any, []readInput) {
	contextMap := make(map[string]any, len(source.Read))
	inputs := make([]readInput, 0, len(source.Read))
	for _, key := range source.Read {
		value, ok := envelopeGet(env, key)
		contextMap[key] = value
		inputs = append(inputs, readInput{Key: key, Value: value, Present: ok && value != nil})
	}
	return contextMap, inputs
}

// readInput is one read key's snapshot for provenance resolution.
type readInput struct {
	Key     string
	Value   any
	Present bool
}

// invokeCapability dispatches the source's pinned capability through a registry
// view restricted to exactly that capability (fail-closed inheritance from the
// step's own scoped registry): a capability outside the step's declared scope
// is not invocable. The read context rides the invocation arguments.
func (a *BlackboardAgent) invokeCapability(ctx context.Context, env *contextdata.Envelope, source AuthoredSource, goal string, readContext map[string]any) (any, error) {
	if a.Tools == nil {
		return nil, fmt.Errorf("blackboard: source %q pins capability %q but no registry is configured", source.Name, source.Capability)
	}
	reg := a.Tools
	if scoped := a.Tools.WithAllowlist([]string{source.Capability}); scoped != nil {
		reg = scoped
	}
	result, err := reg.InvokeCapability(ctx, env.State(), source.Capability, map[string]any{
		"goal":                    goal,
		"blackboard.read_context": readContext,
		"blackboard.write_target": source.Write,
	})
	if err != nil {
		return nil, err
	}
	if result != nil && strings.TrimSpace(result.Error) != "" {
		return nil, fmt.Errorf("capability %q: %s", source.Capability, result.Error)
	}
	if result != nil && !result.Success {
		return nil, fmt.Errorf("capability %q failed", source.Capability)
	}
	if result == nil {
		return nil, nil
	}
	return result.Data, nil
}

// runSpecialist is the no-`do` path: one bounded model call over the goal,
// the write target, and the read context. It goes through the agent's
// configured (instrumented) model, so attribution and ingestion apply.
func (a *BlackboardAgent) runSpecialist(ctx context.Context, env *contextdata.Envelope, source AuthoredSource, goal string, readContext map[string]any) (any, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("blackboard: source %q requires a model (no do capability pinned)", source.Name)
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "You are the blackboard knowledge source %q.\n", source.Name)
	if strings.TrimSpace(goal) != "" {
		fmt.Fprintf(&prompt, "Goal: %s\n", goal)
	}
	if len(readContext) > 0 {
		fmt.Fprintf(&prompt, "Read context:\n")
		for _, key := range source.Read {
			fmt.Fprintf(&prompt, "  %s: %v\n", key, readContext[key])
		}
	}
	fmt.Fprintf(&prompt, "Produce the content for %s.", source.Write)
	resp, err := a.Model.Chat(ctx, []model.Message{{Role: "user", Content: prompt.String()}}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Text, nil
}

// enqueueWrite builds the capture-equivalent grounding item for the source's
// write and hands it to the run's capture sink. Read-context inputs resolve to
// derives_from chunk IDs through the grounding service; values that were never
// grounded are recorded as absent inputs in telemetry, never errors. A missing
// sink is surfaced with an explicit event — the envelope write still happened,
// but the durable leg did not, and that is never silent.
func (a *BlackboardAgent) enqueueWrite(ctx context.Context, env *contextdata.Envelope, source AuthoredSource, output any, origin contextdata.OriginClass, inputs []readInput) {
	item := knowledge.GroundingItem{
		Value:          output,
		Epistemics:     knowledge.EpistemicClaimed,
		Origin:         origin,
		StateKey:       source.Write,
		NodeID:         authoredNodeID,
		TaskID:         env.TaskIDSnapshot(),
		SessionID:      env.SessionIDSnapshot(),
		WorkspaceID:    workspaceID(env),
		Kind:           knowledge.ChunkKindCapture,
		SourceChunkIDs: cappedChunkIDs(env.StreamedChunkIDs()),
	}
	resolved, absent := 0, 0
	for _, input := range inputs {
		id, ok, err := a.resolveReadChunkID(input)
		if err != nil {
			// A store failure during provenance resolution must not fail the
			// cycle: the input is recorded as absent instead.
			absent++
			continue
		}
		if !ok {
			absent++
			continue
		}
		item.ForwardedFrom = append(item.ForwardedFrom, id)
		resolved++
	}
	if sink := graph.CaptureSinkFromContext(ctx); sink != nil {
		sink.EnqueueCapture(item)
	} else {
		emitBlackboardEvent(ctx, a.telemetry(), env, telemetry.EventCaptureSinkAbsent, authoredNodeID, env.TaskIDSnapshot(), "capture sink absent", map[string]any{
			"source": source.Name,
			"write":  source.Write,
		})
	}
	emitBlackboardEvent(ctx, a.telemetry(), env, telemetry.EventBlackboardWriteGrounded, authoredNodeID, env.TaskIDSnapshot(), "blackboard write grounded", map[string]any{
		"source":             source.Name,
		"write":              source.Write,
		"read_inputs":        resolved,
		"read_inputs_absent": absent,
	})
}

// resolveReadChunkID resolves one read input to its grounded chunk ID via the
// grounding service's content-addressed lookup. A nil grounding service (no
// durable knowledge runtime) records every input as absent.
func (a *BlackboardAgent) resolveReadChunkID(input readInput) (knowledge.ChunkID, bool, error) {
	if a.Grounder == nil || !input.Present {
		return "", false, nil
	}
	return a.Grounder.ChunkIDForCaptureValue(input.Value, "")
}

// closeCycleBarrier flushes the cycle's grounding batch: the cycle boundary is
// the epoch boundary (D6), so a grounding admission failure surfaces here and
// fails the cycle under the step's on_error policy as grounding_failed.
func (a *BlackboardAgent) closeCycleBarrier(ctx context.Context, env *contextdata.Envelope) error {
	coord := graph.EpochCoordinatorFromContext(ctx)
	if coord == nil {
		return nil
	}
	if err := coord.CloseEpochIfPending(authoredNodeID); err != nil {
		return err
	}
	return nil
}

func (a *BlackboardAgent) emitSourceEvent(ctx context.Context, env *contextdata.Envelope, source AuthoredSource, cycle int, inputs []readInput) {
	keys := make([]string, 0, len(inputs))
	for _, input := range inputs {
		keys = append(keys, input.Key)
	}
	meta := map[string]any{
		"source":    source.Name,
		"cycle":     cycle,
		"write":     source.Write,
		"read_keys": keys,
	}
	if source.Capability != "" {
		meta["capability"] = source.Capability
	}
	if source.WhenExpr != "" {
		meta["when"] = source.WhenExpr
	}
	emitBlackboardEvent(ctx, a.telemetry(), env, telemetry.EventBlackboardSourceExecuted, authoredNodeID, env.TaskIDSnapshot(), "blackboard source executed", meta)
}

func (a *BlackboardAgent) telemetry() telemetry.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

func workspaceID(env *contextdata.Envelope) string {
	if value, ok := contextdata.GetTyped[string](env, "input.workspace"); ok {
		return value
	}
	return ""
}

func cappedChunkIDs(ids []contextdata.ChunkID) []knowledge.ChunkID {
	const cap = 16
	out := make([]knowledge.ChunkID, 0, len(ids))
	seen := make(map[knowledge.ChunkID]struct{}, len(ids))
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

func episodeKey(name, suffix string) string {
	return episodeArmedKeyPrefix + name + suffix
}

func episodeCount(env *contextdata.Envelope, name, suffix string) (int, bool) {
	raw, ok := envelopeGet(env, episodeKey(name, suffix))
	if !ok {
		return 0, false
	}
	value, ok := raw.(int)
	if !ok {
		return 0, false
	}
	return value, true
}

func setEpisodeCount(env *contextdata.Envelope, name, suffix string, value int) {
	envelopeSet(env, episodeKey(name, suffix), value)
}

func taskInstruction(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

// AuthoredSources returns the installed authored source set. An empty result
// means the built-in specialist loop drives the agent (FR-9).
func (a *BlackboardAgent) AuthoredSources() []AuthoredSource {
	if a == nil {
		return nil
	}
	return append([]AuthoredSource(nil), a.authoredSources...)
}
