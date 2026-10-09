// Package graph provides a deterministic state-machine workflow runtime for agents.
// It executes directed graphs of typed nodes (LLM, Tool, Conditional, Human, Terminal,
// System, Observation) connected by conditional or unconditional edges, recording
// telemetry at each step and enforcing cycle guards.
package agentgraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"codeburg.org/lexbit/relurpify/platform/observability"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/ports"
	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
)

// NodeType enumerates supported node categories.
type NodeType string

const (
	NodeTypeTool        NodeType = "tool"
	NodeTypeConditional NodeType = "conditional"
	NodeTypeHuman       NodeType = "human"
	NodeTypeTerminal    NodeType = "terminal"
	NodeTypeSystem      NodeType = "system"
	NodeTypeObservation NodeType = "observation"
	NodeTypeStream      NodeType = "stream"
)

// Node describes the unit of work executed inside a graph.
type Node interface {
	ID() string
	Type() NodeType
	Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error)
}

// ConditionFunc determines whether an edge should be followed.
type ConditionFunc func(result *execution.Result, env *contextdata.Envelope) bool

// Edge describes a transition between nodes.
type Edge struct {
	From      string
	To        string
	Condition ConditionFunc
	Parallel  bool
}

// parallelBranchResult is the outcome of one parallel branch. index is the
// branch's edge-declaration position and therefore the merge order; delta is
// the branch's change set relative to the fork-time parent and is the merge
// input; env is the branch-final envelope supplying the values and references.
type parallelBranchResult struct {
	index int
	edge  Edge
	env   *contextdata.Envelope
	delta contextdata.BranchDelta
	err   error
}

// ErrGraphSealed is returned by Graph mutators once Execute has begun. A Graph
// has two phases: build (mutators legal) and sealed (Execute has started and the
// structure is immutable). Sealing governs mutation, not execution: Execute may
// be called again on a sealed graph, which re-runs it with reset visit counts.
// Mid-run structure materialization is deliberately unsupported; a mutator on a
// sealed graph fails loudly here instead of racing the run loop.
var ErrGraphSealed = errors.New("agentgraph: graph is sealed (execution in progress or complete)")

// Graph orchestrates a workflow of nodes. It behaves like a tiny, deterministic
// state machine: nodes are registered ahead of time, edges describe transitions,
// and Execute walks the graph while recording telemetry plus enforcing invariants
// such as bounded node visits (to guard against accidental cycles).
//
// Graph is a two-phase artifact. AddNode/AddEdge/SetStart/SetTelemetry/
// SetMaxNodeVisits/SetCapabilityCatalog are build-phase operations; once Execute
// seals the graph they return ErrGraphSealed. Parallel branch subgraphs run
// against an immutable structural snapshot, so no goroutine ever shares a
// mutable map with the parent.
type Graph struct {
	mu                sync.RWMutex
	sealed            bool
	nodes             map[string]Node
	nodeContracts     map[string]NodeContract
	edges             map[string][]Edge
	startNodeID       string
	maxNodeVisits     int
	telemetry         telemetry.Telemetry
	grounder          Grounder
	execMu            sync.Mutex
	visitCounts       map[string]int
	executionPath     []string
	capabilityCatalog CapabilityCatalog
	lastPreflight     *PreflightReport
	lastPreflightErr  error
	preflightDirty    bool
	lastValidationErr error
	validationDirty   bool
}

// NewGraph creates a graph with sane defaults.
func NewGraph() *Graph {
	return &Graph{
		nodes:           make(map[string]Node),
		nodeContracts:   make(map[string]NodeContract),
		edges:           make(map[string][]Edge),
		maxNodeVisits:   1024,
		visitCounts:     make(map[string]int),
		executionPath:   make([]string, 0),
		preflightDirty:  true,
		validationDirty: true,
	}
}

// SetTelemetry wires a telemetry sink for execution traces.
func (g *Graph) SetTelemetry(t telemetry.Telemetry) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return ErrGraphSealed
	}
	g.telemetry = t
	return nil
}

// SetGrounder wires the durable grounding boundary the epoch coordinator flushes
// through. Without a grounder the run carries no coordinator and recipe captures
// degrade to the explicit capture.sink_absent event.
func (g *Graph) SetGrounder(grounder Grounder) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.grounder = grounder
}

func (g *Graph) invalidateStructureLocked() {
	g.validationDirty = true
	g.invalidatePreflightLocked()
}

func (g *Graph) invalidatePreflightLocked() {
	g.preflightDirty = true
	g.lastPreflight = nil
	g.lastPreflightErr = nil
}

// SetMaxNodeVisits updates the cycle-guard visit cap.
//
// The cap is enforced per traversal: every parallel branch subgraph keeps its
// own visit counters, so a node visited once by the parent and once per branch
// counts width+1 in total. The cap exists to catch accidental cycles, not to
// budget aggregate work; per-branch counters are the intended semantics.
func (g *Graph) SetMaxNodeVisits(limit int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return ErrGraphSealed
	}
	if limit > 0 {
		g.maxNodeVisits = limit
	}
	return nil
}

// emit sends telemetry events when a sink is configured; a no-op otherwise.
// ctx carries the turn's correlation identifiers, which are stamped onto the
// event before it reaches the sink.
func (g *Graph) emit(ctx context.Context, event telemetry.Event) {
	g.mu.RLock()
	sink := g.telemetry
	g.mu.RUnlock()
	if sink == nil {
		return
	}
	telemetry.StampCorrelation(ctx, &event)
	sink.Emit(event)
}

// extractTaskID fetches the current task identifier from the execution state so
// telemetry has stable correlation identifiers even across node boundaries.
func (g *Graph) extractTaskID(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.TaskIDSnapshot()
}

// SetStart marks the starting node.
func (g *Graph) SetStart(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return ErrGraphSealed
	}
	if _, ok := g.nodes[id]; !ok {
		return fmt.Errorf("start node %s not found", id)
	}
	g.startNodeID = id
	g.invalidateStructureLocked()
	return nil
}

// HasNode reports whether the graph contains a node with the given ID.
func (g *Graph) HasNode(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.nodes[id]
	return ok
}

// NodeIDs returns a sorted copy of all registered node IDs.
func (g *Graph) NodeIDs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// NodeType returns the type of a registered node.
func (g *Graph) NodeType(id string) (NodeType, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[id]
	if !ok {
		return "", false
	}
	return node.Type(), true
}

// StartNodeID returns the configured start node ID.
func (g *Graph) StartNodeID() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.startNodeID
}

// OutgoingEdges returns a copy of the outgoing edges for the given node.
func (g *Graph) OutgoingEdges(id string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := g.edges[id]
	out := make([]Edge, len(edges))
	copy(out, edges)
	return out
}

// AddNode registers a node.
func (g *Graph) AddNode(node Node) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return ErrGraphSealed
	}
	if _, exists := g.nodes[node.ID()]; exists {
		return fmt.Errorf("node %s already exists", node.ID())
	}
	g.nodes[node.ID()] = node
	g.nodeContracts[node.ID()] = ResolveNodeContract(node)
	g.invalidateStructureLocked()
	return nil
}

// AddEdge wires two nodes together.
func (g *Graph) AddEdge(from, to string, condition ConditionFunc, parallel bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return ErrGraphSealed
	}
	if _, ok := g.nodes[from]; !ok {
		return fmt.Errorf("node %s not defined", from)
	}
	if _, ok := g.nodes[to]; !ok {
		return fmt.Errorf("node %s not defined", to)
	}
	g.edges[from] = append(g.edges[from], Edge{
		From:      from,
		To:        to,
		Condition: condition,
		Parallel:  parallel,
	})
	g.invalidateStructureLocked()
	return nil
}

// Execute runs the graph from its start node. The first call seals the graph:
// subsequent build-phase mutations return ErrGraphSealed. Execute itself may be
// called again; each call resets visit counts and runs the sealed structure.
func (g *Graph) Execute(ctx context.Context, env *contextdata.Envelope) (result *execution.Result, err error) {
	g.mu.Lock()
	g.sealed = true
	g.mu.Unlock()
	if err := g.Validate(); err != nil {
		return nil, err
	}
	if _, err := g.Preflight(); err != nil {
		return nil, err
	}

	taskID := g.extractTaskID(env)
	taskMeta := g.extractTaskMeta(env)
	g.emit(ctx, telemetry.Event{
		Type:      telemetry.EventGraphStart,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  taskMeta,
	})
	defer func() {
		g.emit(ctx, telemetry.Event{
			Type:      telemetry.EventGraphFinish,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"status": statusFor(err),
			},
		})
	}()

	// Epoch lifecycle: the top-level Execute owns the coordinator. Nested
	// executions (parallel branches) share the parent coordinator from ctx and
	// never finalize it.
	coord := EpochCoordinatorFromContext(ctx)
	ownsCoordinator := false
	if coord == nil && g.grounder != nil {
		coord = NewEpochCoordinator(contextdata.WithEnvelope(ctx, env), g.grounder, g.telemetry)
		ctx = WithEpochCoordinator(ctx, coord)
		ownsCoordinator = true
	}
	if ownsCoordinator {
		// Safety net: flush what exists even on panic paths (idempotent).
		defer func() { _ = coord.FinalEpoch("") }()
	}

	if g.startNodeID == "" {
		return nil, errors.New("graph has no start node")
	}

	result, err = g.run(ctx, env, g.startNodeID, true, taskID)
	if err == nil && ownsCoordinator {
		if finalErr := coord.FinalEpoch(""); finalErr != nil {
			err = finalErr
		}
	}
	return result, err
}

func statusFor(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func (g *Graph) run(ctx context.Context, env *contextdata.Envelope, current string, reset bool, taskID string) (*execution.Result, error) {
	g.execMu.Lock()
	defer g.execMu.Unlock()
	if reset {
		g.visitCounts = make(map[string]int)
		g.executionPath = make([]string, 0)
	}
	// The graph is sealed by Execute before this loop begins, so the structure
	// is immutable for the duration of the run. We therefore take g.mu only for
	// short map reads and never hold it across node.Execute. Mid-run structure
	// materialization is unsupported: a mutator called here returns
	// ErrGraphSealed rather than racing this loop.

	var lastResult *execution.Result
	for current != "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		g.mu.RLock()
		node, ok := g.nodes[current]
		g.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("node %s missing", current)
		}
		g.visitCounts[current]++
		if g.visitCounts[current] > g.maxNodeVisits {
			return nil, fmt.Errorf("potential cycle detected at node %s", current)
		}
		g.executionPath = append(g.executionPath, current)
		g.emit(ctx, telemetry.Event{
			Type:      telemetry.EventNodeStart,
			NodeID:    current,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
		})

		taskType := execution.TaskType(fmt.Sprint(taskMetaValue(env, "task.type")))
		instruction := fmt.Sprint(taskMetaValue(env, "task.instruction"))
		// Attach the envelope and the active node ID to the context so
		// downstream emitters — above all the LLM instrumentation — can
		// attribute their events to this task and node without importing
		// the execution packages (telemetry correlation, FR-3).
		nodeCtx := contextdata.WithEnvelope(ctx, env)
		nodeCtx = observability.WithNodeContext(nodeCtx, current)
		nodeCtx = execution.WithTaskContext(nodeCtx, execution.TaskContext{ID: taskID, Type: taskType, Instruction: instruction})
		if g.telemetry != nil {
			nodeCtx = telemetry.WithTelemetry(nodeCtx, g.telemetry)
		}
		result, err := node.Execute(nodeCtx, env)
		if err != nil {
			err = fmt.Errorf("node %s execution failed: %w", current, err)
			g.emit(ctx, telemetry.Event{
				Type:      telemetry.EventNodeError,
				NodeID:    current,
				TaskID:    taskID,
				Timestamp: time.Now().UTC(),
				Message:   err.Error(),
			})
			return nil, err
		}
		if result == nil {
			result = &execution.Result{NodeID: current, Success: true}
		}
		result.NodeID = current
		lastResult = result
		for key, value := range execution.ResultFields(result.Data) {
			env.SetWorkingValueWithClass(fmt.Sprintf("%s.%s", current, key), value, contextdata.MemoryClassTask)
		}
		// Epoch barrier: flush the node's grounding and land background stream
		// jobs so the next node's context compilation sees read-your-writes.
		if coord := EpochCoordinatorFromContext(ctx); coord != nil {
			if err := coord.CloseEpochIfPending(current); err != nil {
				return nil, fmt.Errorf("agentgraph: %w", err)
			}
		}
		g.emit(ctx, telemetry.Event{
			Type:      telemetry.EventNodeFinish,
			NodeID:    current,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"success": result.Success,
			},
		})
		if result.Metadata != nil {
			if pause, _ := result.Metadata["euclo.interaction.pause"].(bool); pause {
				return lastResult, nil
			}
		}
		next, _, err := g.nextNodes(ctx, env, node, result)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return lastResult, nil
}

func taskMetaValue(env *contextdata.Envelope, key string) any {
	if env == nil {
		return nil
	}
	if v, ok := contextdata.GetTyped[any](env, key); ok {
		return v
	}
	return nil
}

func (g *Graph) extractTaskMeta(env *contextdata.Envelope) map[string]any {
	if env == nil {
		return nil
	}
	meta := map[string]any{}
	if v := taskMetaValue(env, "task.type"); v != nil {
		meta["task_type"] = v
	}
	if v := taskMetaValue(env, "task.instruction"); v != nil {
		meta["instruction"] = v
	}
	if v := taskMetaValue(env, "task.source"); v != nil {
		meta["source"] = v
	}
	return meta
}

// nextNodes evaluates the outgoing edges for a node. Conditions are evaluated
// against the pre-branch parent state before any fork. Parallel edges execute
// on independent structural snapshots with cloned envelopes, collect results by
// edge index, and merge in declaration order; serial edges behave like a
// traditional state machine transition. Returning a single node ID keeps the
// main Execute loop simple and debuggable.
func (g *Graph) nextNodes(ctx context.Context, env *contextdata.Envelope, node Node, result *execution.Result) (string, string, error) {
	g.mu.RLock()
	outEdges := make([]Edge, len(g.edges[node.ID()]))
	copy(outEdges, g.edges[node.ID()])
	g.mu.RUnlock()
	if len(outEdges) == 0 || node.Type() == NodeTypeTerminal {
		return "", "terminal", nil
	}
	var serialEdges []Edge
	var parallelEdges []Edge
	for _, edge := range outEdges {
		if edge.Condition != nil && !edge.Condition(result, env) {
			continue
		}
		if edge.Parallel {
			parallelEdges = append(parallelEdges, edge)
		} else {
			serialEdges = append(serialEdges, edge)
		}
	}
	// Launch parallel branches. Condition evaluation above already ran against
	// the pre-branch parent state (predicates route, branches compute). Each
	// branch executes on a private structural snapshot of the sealed graph and
	// its own cloned envelope; results land in a pre-sized slice by edge index
	// so collection order is never goroutine completion order.
	if len(parallelEdges) > 0 {
		// The measurement spans the whole parallel section (snapshot + branch
		// execution + index-ordered merge), not just the merge call.
		branchSectionStarted := time.Now()
		snapshot := g.snapshotGraph()
		results := make([]parallelBranchResult, len(parallelEdges))
		var wg sync.WaitGroup
		for edgeIndex, edge := range parallelEdges {
			wg.Add(1)
			go func(index int, edge Edge) {
				defer wg.Done()
				perfstats.IncBranchClone()
				branchEnv := contextdata.CloneEnvelope(env)
				_, err := g.executeBranch(ctx, snapshot, edge.To, branchEnv)
				results[index] = parallelBranchResult{
					index: index,
					edge:  edge,
					env:   branchEnv,
					delta: contextdata.ComputeBranchDelta(env, branchEnv),
					err:   err,
				}
			}(edgeIndex, edge)
		}
		wg.Wait()
		// Scan in edge-declaration order: the lowest-index error aborts before
		// any merge, so failure handling is deterministic too.
		for i := range results {
			if results[i].err != nil {
				return "", "", results[i].err
			}
		}
		if err := g.mergeParallelBranchEnvelopes(ctx, env, results); err != nil {
			return "", "", err
		}
		perfstats.ObserveBranchMerge(time.Since(branchSectionStarted))
	}
	if len(serialEdges) == 0 {
		if len(parallelEdges) > 0 {
			return "", "parallel-complete", nil
		}
		return "", "no-transition", nil
	}
	if len(serialEdges) > 1 {
		return "", "", fmt.Errorf("ambiguous transitions from %s", node.ID())
	}
	reason := "serial"
	if node.Type() == NodeTypeConditional {
		reason = "conditional"
	} else if len(parallelEdges) > 0 {
		reason = "parallel-serial"
	}
	return serialEdges[0].To, reason, nil
}

// mergeParallelBranchEnvelopes applies the branch deltas to the parent in edge
// declaration order and emits graph.branch_merged. branches[i] must belong to
// edge index i (the fan-out assigns results by index before waiting), so the
// slice is already the declaration order the merge requires.
func (g *Graph) mergeParallelBranchEnvelopes(ctx context.Context, parent *contextdata.Envelope, branches []parallelBranchResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	envs := make([]*contextdata.Envelope, 0, len(branches))
	units := make([]contextdata.BranchMergeUnit, 0, len(branches))
	for _, branch := range branches {
		if branch.env == nil {
			continue
		}
		envs = append(envs, branch.env)
		units = append(units, contextdata.BranchMergeUnit{
			Index: branch.index,
			ID:    branch.edge.To,
			Delta: branch.delta,
			Env:   branch.env,
		})
	}
	if len(units) == 0 {
		return nil
	}
	if err := contextdata.ValidateBranchMerge(envs); err != nil {
		return err
	}
	stats, err := parent.ApplyBranchMerges(units)
	if err != nil {
		return err
	}
	g.emit(ctx, telemetry.Event{
		Type:      telemetry.EventGraphBranchMerged,
		TaskID:    g.extractTaskID(parent),
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"units_applied":   stats.UnitsApplied,
			"branches":        mergeUnitIDs(units),
			"keys_written":    stats.KeysWritten,
			"keys_deleted":    stats.KeysDeleted,
			"keys_skipped":    stats.KeysSkipped,
			"conflicted_keys": stats.Conflicts,
			"winner_index":    conflictWinnerIndices(units, stats.Conflicts),
			"refs_streamed":   stats.RefsStreamed,
			"refs_retrieval":  stats.RefsRetrieval,
		},
	})
	return nil
}

// mergeUnitIDs returns the branch identifiers that participated in the merge,
// in declaration order.
func mergeUnitIDs(units []contextdata.BranchMergeUnit) []string {
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		ids = append(ids, unit.ID)
	}
	return ids
}

// conflictWinnerIndices maps each conflicted key to the index of the last unit
// that mentioned it — the declaration-order winner. Units are ascending by
// Index, so the final match is the winner.
func conflictWinnerIndices(units []contextdata.BranchMergeUnit, conflicts []string) map[string]int {
	if len(conflicts) == 0 {
		return nil
	}
	winners := make(map[string]int, len(conflicts))
	for _, key := range conflicts {
		winner := -1
		for _, unit := range units {
			if branchDeltaMentions(unit.Delta, key) {
				winner = unit.Index
			}
		}
		if winner >= 0 {
			winners[key] = winner
		}
	}
	return winners
}

func branchDeltaMentions(delta contextdata.BranchDelta, key string) bool {
	return stringSliceContains(delta.WorkingMemoryAdded, key) ||
		stringSliceContains(delta.WorkingMemoryModified, key) ||
		stringSliceContains(delta.WorkingMemoryDeleted, key)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// graphSnapshot is an immutable structural view of a sealed graph, taken under
// the parent's lock and consumed by branch subgraphs. nodes and nodeContracts
// are shallow-copied: Node and NodeContract are treated as immutable after
// build (NodeContract's slices are shared read-only), and the subgraph's
// Validate takes the cached-result fast path, so nodeContracts is never
// written. edges are deep-copied per key so no subgraph aliases a parent edge
// slice.
type graphSnapshot struct {
	nodes             map[string]Node
	nodeContracts     map[string]NodeContract
	edges             map[string][]Edge
	maxNodeVisits     int
	telemetry         telemetry.Telemetry
	capabilityCatalog CapabilityCatalog
	lastPreflight     *PreflightReport
	lastPreflightErr  error
	preflightDirty    bool
	lastValidationErr error
}

// snapshotGraphLocked copies the graph structure into a snapshot. The caller
// must hold g.mu for reading.
func (g *Graph) snapshotGraphLocked() graphSnapshot {
	snapshot := graphSnapshot{
		nodes:             make(map[string]Node, len(g.nodes)),
		nodeContracts:     make(map[string]NodeContract, len(g.nodeContracts)),
		edges:             make(map[string][]Edge, len(g.edges)),
		maxNodeVisits:     g.maxNodeVisits,
		telemetry:         g.telemetry,
		capabilityCatalog: g.capabilityCatalog,
		lastPreflight:     g.lastPreflight,
		lastPreflightErr:  g.lastPreflightErr,
		preflightDirty:    g.preflightDirty,
		lastValidationErr: g.lastValidationErr,
	}
	for id, node := range g.nodes {
		snapshot.nodes[id] = node
	}
	for id, contract := range g.nodeContracts {
		snapshot.nodeContracts[id] = contract
	}
	for id, edges := range g.edges {
		copied := make([]Edge, len(edges))
		copy(copied, edges)
		snapshot.edges[id] = copied
	}
	return snapshot
}

// snapshotGraph takes a structural snapshot under the read lock. It is called
// once per fan-out, before the branch goroutines are spawned.
func (g *Graph) snapshotGraph() graphSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.snapshotGraphLocked()
}

// executeBranch runs a detached subgraph starting at the provided node. The
// subgraph executes against the immutable snapshot taken before the fan-out,
// so it shares no mutable map with the parent and takes no parent lock while
// running (its own g.mu guards only its private copies). Each branch receives
// its own cloned envelope; the branch-final envelope is merged back in
// edge-declaration order by mergeParallelBranchEnvelopes.
func (g *Graph) executeBranch(ctx context.Context, snapshot graphSnapshot, start string, env *contextdata.Envelope) (*execution.Result, error) {
	subGraph := &Graph{
		nodes:             snapshot.nodes,
		nodeContracts:     snapshot.nodeContracts,
		edges:             snapshot.edges,
		startNodeID:       start,
		maxNodeVisits:     snapshot.maxNodeVisits,
		telemetry:         snapshot.telemetry,
		capabilityCatalog: snapshot.capabilityCatalog,
		lastPreflight:     snapshot.lastPreflight,
		lastPreflightErr:  snapshot.lastPreflightErr,
		preflightDirty:    snapshot.preflightDirty,
		lastValidationErr: snapshot.lastValidationErr,
		// The parent validated before the fan-out; the snapshot carries that
		// result, so the subgraph never writes its nodeContracts map.
		validationDirty: false,
	}
	return subGraph.Execute(ctx, env)
}

// Validate ensures the graph is well-formed (start node present, edges reference known nodes).
func (g *Graph) Validate() error {
	g.mu.RLock()
	if !g.validationDirty {
		err := g.lastValidationErr
		g.mu.RUnlock()
		return err
	}
	g.mu.RUnlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.validationDirty {
		return g.lastValidationErr
	}
	if len(g.nodes) == 0 {
		g.lastValidationErr = errors.New("graph has no nodes")
		g.validationDirty = false
		return g.lastValidationErr
	}
	if g.startNodeID == "" {
		g.lastValidationErr = errors.New("graph has no start node")
		g.validationDirty = false
		return g.lastValidationErr
	}
	for from, edges := range g.edges {
		if _, ok := g.nodes[from]; !ok {
			g.lastValidationErr = fmt.Errorf("edge references missing node %s", from)
			g.validationDirty = false
			return g.lastValidationErr
		}
		for _, edge := range edges {
			if _, ok := g.nodes[edge.To]; !ok {
				g.lastValidationErr = fmt.Errorf("edge references missing node %s", edge.To)
				g.validationDirty = false
				return g.lastValidationErr
			}
		}
	}
	for _, node := range g.nodes {
		contract, ok := g.nodeContracts[node.ID()]
		if !ok {
			contract = ResolveNodeContract(node)
			g.nodeContracts[node.ID()] = contract
		}
		if err := validateNodeContract(node, contract); err != nil {
			g.lastValidationErr = err
			g.validationDirty = false
			return err
		}
	}
	g.lastValidationErr = nil
	g.validationDirty = false
	return nil
}

// Pause builds a snapshot at the given node.
// ToolNode executes a tool by name.
type ToolNode struct {
	id       string
	Tool     ports.Tool
	Args     map[string]any
	Registry CapabilityInvoker
	traceID  atomic.Pointer[string] // set by SetTraceID
}

// SetTraceID assigns a trace ID for child span generation. Parallel branches
// share ToolNode instances, so the field is stored atomically.
func (n *ToolNode) SetTraceID(traceID string) {
	n.traceID.Store(&traceID)
}

// traceIDValue returns the current trace ID, or "" when unset.
func (n *ToolNode) traceIDValue() string {
	if p := n.traceID.Load(); p != nil {
		return *p
	}
	return ""
}

// nextSpanID generates a unique child span ID for each tool call.
func (n *ToolNode) nextSpanID() string {
	return observability.NewSpanID()
}

// nextTraceContext derives a child trace context for a tool invocation.
func (n *ToolNode) nextTraceContext() observability.TraceContext {
	return observability.TraceContext{
		TraceID: n.traceIDValue(),
		SpanID:  n.nextSpanID(),
	}
}

// CapabilityInvoker is the narrow registry contract ToolNode needs for
// capability-routed execution without importing the concrete registry package.
type CapabilityInvoker interface {
	InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]any) (*ports.ToolResult, error)
	CapturePolicySnapshot() *capresult.PolicySnapshot
	GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool)
}

// NewToolNode constructs a tool node with a required capability invoker.
func NewToolNode(id string, tool ports.Tool, args map[string]any, registry CapabilityInvoker) *ToolNode {
	if registry == nil {
		panic("graph.NewToolNode requires a capability registry")
	}
	return &ToolNode{
		id:       id,
		Tool:     tool,
		Args:     args,
		Registry: registry,
	}
}

// ID implements Node.
func (n *ToolNode) ID() string { return n.id }

// Type implements Node.
func (n *ToolNode) Type() NodeType { return NodeTypeTool }

// Contract describes the capability requirement and replay characteristics for
// tool-backed nodes.
func (n *ToolNode) Contract() NodeContract {
	return toolNodeContract(n.Tool)
}

// Execute calls the tool through the capability registry.
func (n *ToolNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.Tool == nil {
		return nil, errors.New("tool node missing tool")
	}
	if n.Registry == nil {
		return nil, fmt.Errorf("tool node %q missing capability registry", n.id)
	}
	// Attach trace context for child span generation in instrumentedTool.
	if n.traceID.Load() == nil {
		traceID := observability.NewTraceID()
		n.traceID.Store(&traceID)
	}
	tc := n.nextTraceContext()
	ctx = observability.WithTraceContext(ctx, tc)
	res, err := n.Registry.InvokeCapability(ctx, env, n.Tool.Name(), n.Args)
	if err != nil {
		return nil, err
	}
	envelope := attachCapabilityEnvelope(ctx, n.Registry, n.Tool, env, res, n.Args)
	result := resultFromToolExecution(n.id, res)
	if envelope != nil {
		// Build a fresh metadata map so the envelope pointer is not written back
		// into res.Metadata (which would recreate the ToolResult → Envelope cycle).
		meta := make(map[string]any, len(res.Metadata)+1)
		for k, v := range res.Metadata {
			meta[k] = v
		}
		meta["capability_result_envelope"] = envelope
		result.Metadata = meta
	}
	return result, nil
}

// ConditionalNode computes the next branch dynamically.
type ConditionalNode struct {
	id        string
	Condition func(*contextdata.Envelope) (string, error)
}

// ID implements Node.
func (n *ConditionalNode) ID() string { return n.id }

// Type implements Node.
func (n *ConditionalNode) Type() NodeType { return NodeTypeConditional }

// Execute just evaluates the condition and stores the decision.
func (n *ConditionalNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	to, err := n.Condition(env)
	if err != nil {
		return nil, err
	}
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"next": to}),
	}, nil
}

// HumanNode represents a pause waiting for user approval.
type HumanNode struct {
	id       string
	Prompt   string
	Callback func(*contextdata.Envelope) error
}

// ID implements Node.
func (n *HumanNode) ID() string { return n.id }

// Type implements Node.
func (n *HumanNode) Type() NodeType { return NodeTypeHuman }

// Contract describes human-gated execution semantics.
func (n *HumanNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectHuman,
		Idempotency:     IdempotencySingleShot,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "approval.*"},
			WriteKeys:                []string{"approval.*"},
			AllowHistoryAccess:       true,
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
		},
	}
}

// Execute pauses execution until callback completes.
func (n *HumanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.Callback != nil {
		if err := n.Callback(env); err != nil {
			return nil, err
		}
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// TerminalNode marks the end of the workflow.
type TerminalNode struct {
	id string
}

// NewTerminalNode creates a terminal node.
func NewTerminalNode(id string) *TerminalNode {
	return &TerminalNode{id: id}
}

// ID implements Node.
func (n *TerminalNode) ID() string { return n.id }

// Type implements Node.
func (n *TerminalNode) Type() NodeType { return NodeTypeTerminal }

// Contract describes terminal nodes as replay-safe control flow only.
func (n *TerminalNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectNone,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "plan.*", "react.*", "architect.*"},
			WriteKeys:                []string{},
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStepMetadata, relurpctx.StateDataClassRoutingFlag, relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 32,
		},
	}
}

// Execute completes immediately.
func (n *TerminalNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func resultFromToolExecution(nodeID string, res *ports.ToolResult) *execution.Result {
	if res == nil {
		return &execution.Result{NodeID: nodeID, Success: true}
	}
	return &execution.Result{
		NodeID:   nodeID,
		Success:  res.Success,
		Data:     execution.NewToolResultPayload(res.Data),
		Metadata: res.Metadata,
		Error:    res.Error,
	}
}

func attachCapabilityEnvelope(ctx context.Context, registry CapabilityInvoker, tool ports.Tool, env *contextdata.Envelope, res *ports.ToolResult, args map[string]any) *capresult.CapabilityResultEnvelope {
	if registry == nil || tool == nil || res == nil {
		return nil
	}
	if res.Metadata == nil {
		res.Metadata = map[string]any{}
	}
	if res.Metadata["capability_envelope_created"] == true {
		return nil
	}

	desc, ok := res.Metadata["capability_descriptor"].(descriptor.CapabilityDescriptor)
	if !ok || desc.ID == "" {
		desc, ok = registry.GetCapability(tool.Name())
		if !ok || desc.ID == "" {
			desc = descriptor.ToolDescriptor(ctx, tool)
		}
	}

	var approval *capresult.ApprovalBinding
	if raw := res.Metadata["approval_binding"]; raw != nil {
		if typed, ok := raw.(*capresult.ApprovalBinding); ok {
			approval = typed
		}
	}
	if approval == nil {
		approval = capresult.ApprovalBindingFromCapability(desc, env.Snapshot(), args)
	}

	envelope := capresult.NewCapabilityResultEnvelope(desc, res, capresult.ContentDispositionRaw, registry.CapturePolicySnapshot(), approval)
	if decision, ok := res.Metadata["insertion_decision"].(capresult.InsertionDecision); ok {
		envelope = capresult.ApplyInsertionDecision(envelope, decision)
	}
	res.Metadata["insertion_decision"] = envelope.Insertion
	res.Metadata["capability_envelope_created"] = true
	return envelope
}
