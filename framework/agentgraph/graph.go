// Package graph provides a deterministic state-machine workflow runtime for agents.
// It executes directed graphs of typed nodes (LLM, Tool, Conditional, Human, Terminal,
// System, Observation) connected by conditional or unconditional edges, recording
// telemetry at each step and enforcing cycle guards.
package agentgraph

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/perfstats"
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
)

// Node describes the unit of work executed inside a graph.
type Node interface {
	ID() string
	Type() NodeType
	Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error)
}

// ConditionFunc determines whether an edge should be followed.
type ConditionFunc func(result *Result, env *contextdata.Envelope) bool

// Edge describes a transition between nodes.
type Edge struct {
	From      string
	To        string
	Condition ConditionFunc
	Parallel  bool
}

type parallelBranchResult struct {
	edge  Edge
	env   *contextdata.Envelope
	delta contextdata.BranchDelta
	err   error
}

// Graph orchestrates a workflow of nodes. It behaves like a tiny, deterministic
// state machine: nodes are registered ahead of time, edges describe transitions,
// and Execute walks the graph while recording telemetry plus enforcing invariants
// such as bounded node visits (to guard against accidental cycles).
type Graph struct {
	mu                   sync.RWMutex
	nodes                map[string]Node
	nodeContracts        map[string]NodeContract
	edges                map[string][]Edge
	startNodeID          string
	maxNodeVisits        int
	telemetry            Telemetry
	execMu               sync.Mutex
	visitCounts          map[string]int
	executionPath        []string
	checkpointInterval   int
	checkpointCallback   CheckpointCallback
	lastCheckpointNode   string
	nodesSinceCheckpoint int
	capabilityCatalog    CapabilityCatalog
	lastPreflight        *PreflightReport
	lastPreflightErr     error
	preflightDirty       bool
	lastValidationErr    error
	validationDirty      bool
	graphHash            string
	hashDirty            bool
}

// CheckpointCallback receives checkpoints generated during execution.
type CheckpointCallback func(checkpoint *GraphCheckpoint) error

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
		hashDirty:       true,
	}
}

// WithCheckpointing configures automatic checkpointing for the graph.
func (g *Graph) WithCheckpointing(interval int, callback CheckpointCallback) *Graph {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkpointInterval = interval
	g.checkpointCallback = callback
	g.invalidatePreflightLocked()
	return g
}

// SetTelemetry wires a telemetry sink for execution traces.
func (g *Graph) SetTelemetry(t Telemetry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.telemetry = t
}

func (g *Graph) invalidateStructureLocked() {
	g.validationDirty = true
	g.invalidatePreflightLocked()
	g.hashDirty = true
}

func (g *Graph) invalidatePreflightLocked() {
	g.preflightDirty = true
	g.lastPreflight = nil
	g.lastPreflightErr = nil
}

// SetMaxNodeVisits updates the cycle-guard visit cap.
func (g *Graph) SetMaxNodeVisits(limit int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit > 0 {
		g.maxNodeVisits = limit
	}
}

// emit sends telemetry events when a sink is configured; a no-op otherwise.
func (g *Graph) emit(event Event) {
	g.mu.RLock()
	telemetry := g.telemetry
	g.mu.RUnlock()
	if telemetry == nil {
		return
	}
	telemetry.Emit(event)
}

// extractTaskID fetches the current task identifier from the execution state so
// telemetry has stable correlation identifiers even across node boundaries.
func (g *Graph) extractTaskID(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.TaskID
}

// SetStart marks the starting node.
func (g *Graph) SetStart(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.nodes[id]; !ok {
		return fmt.Errorf("start node %s not found", id)
	}
	g.startNodeID = id
	g.invalidateStructureLocked()
	return nil
}

// AddNode registers a node.
func (g *Graph) AddNode(node Node) error {
	g.mu.Lock()
	defer g.mu.Unlock()
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

// GraphSnapshot stores enough state to resume an execution.
type GraphSnapshot struct {
	NextNodeID string
	State      *ContextSnapshot
}

// Execute runs the graph from its start node.
func (g *Graph) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	if _, err := g.Preflight(); err != nil {
		return nil, err
	}

	taskID := g.extractTaskID(env)
	taskMeta := g.extractTaskMeta(env)
	g.emit(Event{
		Type:      EventGraphStart,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  taskMeta,
	})
	var execErr error
	defer func() {
		status := "success"
		if execErr != nil {
			status = "error"
		}
		g.emit(Event{
			Type:      EventGraphFinish,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]interface{}{
				"status": status,
			},
		})
	}()

	if g.startNodeID == "" {
		execErr = errors.New("graph has no start node")
		return nil, execErr
	}

	lastResult, err := g.run(ctx, env, g.startNodeID, true, taskID)
	execErr = err
	return lastResult, err
}

func (g *Graph) run(ctx context.Context, env *contextdata.Envelope, current string, reset bool, taskID string) (*Result, error) {
	g.execMu.Lock()
	defer g.execMu.Unlock()
	if reset {
		g.visitCounts = make(map[string]int)
		g.executionPath = make([]string, 0)
		g.lastCheckpointNode = ""
		g.nodesSinceCheckpoint = 0
	}
	// NOTE: We intentionally do NOT hold g.mu.RLock across the entire loop.
	// Nodes may mutate the graph during execution (e.g. MaterializePlanGraph
	// adds step nodes/edges dynamically). Holding a read lock here would
	// deadlock against the write lock those mutations require.

	var lastResult *Result
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
		g.nodesSinceCheckpoint++
		g.emit(Event{
			Type:      EventNodeStart,
			NodeID:    current,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
		})
		taskType := TaskType(fmt.Sprint(taskMetaValue(env, "task.type")))
		instruction := fmt.Sprint(taskMetaValue(env, "task.instruction"))
		nodeCtx := WithTaskContext(ctx, TaskContext{ID: taskID, Type: taskType, Instruction: instruction})
		result, err := node.Execute(nodeCtx, env)
		if err != nil {
			err = fmt.Errorf("node %s execution failed: %w", current, err)
			g.emit(Event{
				Type:      EventNodeError,
				NodeID:    current,
				TaskID:    taskID,
				Timestamp: time.Now().UTC(),
				Message:   err.Error(),
			})
			return nil, err
		}
		if result == nil {
			result = &Result{NodeID: current, Success: true, Data: map[string]interface{}{}}
		}
		result.NodeID = current
		lastResult = result
		for key, value := range result.Data {
			env.SetWorkingValue(fmt.Sprintf("%s.%s", current, key), value, contextdata.MemoryClassTask)
		}
		g.emit(Event{
			Type:      EventNodeFinish,
			NodeID:    current,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]interface{}{
				"success": result.Success,
			},
		})
		next, reason, err := g.nextNodes(ctx, env, node, result)
		if err != nil {
			return nil, err
		}
		g.maybeCheckpoint(taskID, current, next, reason, result, env)
		current = next
	}
	return lastResult, nil
}

func taskMetaValue(env *contextdata.Envelope, key string) interface{} {
	if env == nil {
		return nil
	}
	if v, ok := env.GetWorkingValue(key); ok {
		return v
	}
	return nil
}

func (g *Graph) extractTaskMeta(env *contextdata.Envelope) map[string]interface{} {
	if env == nil {
		return nil
	}
	meta := map[string]interface{}{}
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

func (g *Graph) maybeCheckpoint(taskID, completedNode, nextNode, transitionReason string, result *Result, env *contextdata.Envelope) {
	if g.checkpointInterval == 0 || g.checkpointCallback == nil {
		return
	}
	if !g.shouldCheckpoint() {
		return
	}
	checkpoint, err := g.CreateCheckpoint(taskID, completedNode, nextNode, result, &NodeTransitionRecord{
		FromNodeID:       g.previousNodeID(),
		CompletedNodeID:  completedNode,
		NextNodeID:       nextNode,
		TransitionReason: transitionReason,
		CompletedAt:      time.Now().UTC(),
	}, env)
	if err != nil {
		g.emit(Event{
			Type:      EventNodeError,
			NodeID:    completedNode,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("checkpoint creation failed: %v", err),
		})
		return
	}
	if err := g.checkpointCallback(checkpoint); err != nil {
		g.emit(Event{
			Type:      EventNodeError,
			NodeID:    completedNode,
			TaskID:    taskID,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("checkpoint callback failed: %v", err),
		})
		return
	}
	g.lastCheckpointNode = completedNode
	g.nodesSinceCheckpoint = 0
}

func (g *Graph) shouldCheckpoint() bool {
	if g.checkpointInterval == 0 {
		return false
	}
	return g.nodesSinceCheckpoint >= g.checkpointInterval
}

func (g *Graph) previousNodeID() string {
	if len(g.executionPath) < 2 {
		return ""
	}
	return g.executionPath[len(g.executionPath)-2]
}

// nextNodes evaluates the outgoing edges for a node. Parallel edges are
// executed optimistically on cloned contexts while serial edges behave like a
// traditional state machine transition. Returning a single node ID keeps the
// main Execute loop simple and debuggable.
func (g *Graph) nextNodes(ctx context.Context, env *contextdata.Envelope, node Node, result *Result) (string, string, error) {
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
	// Launch parallel branches, merging their updates into the shared state.
	if len(parallelEdges) > 0 {
		var wg sync.WaitGroup
		results := make(chan parallelBranchResult, len(parallelEdges))
		for _, edge := range parallelEdges {
			wg.Add(1)
			edge := edge
			go func() {
				defer wg.Done()
				perfstats.IncBranchClone()
				branchID := fmt.Sprintf("branch-%s", edge.To)
				branchEnv := contextdata.CloneEnvelope(env, branchID)
				_, err := g.executeBranch(ctx, edge.To, branchEnv)
				results <- parallelBranchResult{
					edge:  edge,
					env:   branchEnv,
					delta: contextdata.ComputeBranchDelta(env, branchEnv),
					err:   err,
				}
			}()
		}
		wg.Wait()
		close(results)
		branches := make([]parallelBranchResult, 0, len(parallelEdges))
		for result := range results {
			if result.err != nil {
				return "", "", result.err
			}
			branches = append(branches, result)
		}
		mergeStarted := time.Now()
		if err := mergeParallelBranchEnvelopes(env, branches); err != nil {
			return "", "", err
		}
		perfstats.ObserveBranchMerge(time.Since(mergeStarted))
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

func mergeParallelBranchEnvelopes(parent *contextdata.Envelope, branches []parallelBranchResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	// Collect branch envelopes for merge
	branchEnvelopes := make([]*contextdata.Envelope, 0, len(branches))
	for _, branch := range branches {
		if branch.env != nil {
			branchEnvelopes = append(branchEnvelopes, branch.env)
		}
	}
	if len(branchEnvelopes) == 0 {
		return nil
	}
	// Validate before merge
	if err := contextdata.ValidateBranchMerge(branchEnvelopes); err != nil {
		return err
	}
	// Merge envelopes
	merged, err := contextdata.MergeBranchEnvelopes(parent.TaskID, parent.SessionID, branchEnvelopes)
	if err != nil {
		return err
	}
	// Update parent with merged state
	parent.WorkingData = merged.WorkingData
	parent.References = merged.References
	return nil
}

// executeBranch runs a detached sub-graph that starts at the provided node.
// The parent graph shares the node/edge definitions but each branch receives a
// cloned Envelope, which preserves determinism until Merge recombines updates.
func (g *Graph) executeBranch(ctx context.Context, start string, env *contextdata.Envelope) (*Result, error) {
	// We reuse the same node/edge maps because branch graphs are read-only. The
	// only mutable data lives inside the cloned Envelope passed to this function.
	subGraph := &Graph{
		nodes:           g.nodes,
		nodeContracts:   g.nodeContracts,
		edges:           g.edges,
		startNodeID:     start,
		maxNodeVisits:   g.maxNodeVisits,
		telemetry:       g.telemetry,
		preflightDirty:  g.preflightDirty,
		validationDirty: g.validationDirty,
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
func (g *Graph) Pause(currentNode string, env *contextdata.Envelope) *GraphSnapshot {
	snapshot := env.Snapshot()
	return &GraphSnapshot{
		NextNodeID: currentNode,
		State:      &ContextSnapshot{State: snapshot},
	}
}

// ToolNode executes a tool by name.
type ToolNode struct {
	id       string
	Tool     Tool
	Args     map[string]interface{}
	Registry CapabilityInvoker
}

// CapabilityInvoker is the narrow registry contract ToolNode needs for
// capability-routed execution without importing the concrete registry package.
type CapabilityInvoker interface {
	InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]interface{}) (*core.ToolResult, error)
	CapturePolicySnapshot() *core.PolicySnapshot
	GetCapability(idOrName string) (core.CapabilityDescriptor, bool)
}

// NewToolNode constructs a tool node with a required capability invoker.
func NewToolNode(id string, tool Tool, args map[string]interface{}, registry CapabilityInvoker) *ToolNode {
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
func (n *ToolNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	if n.Tool == nil {
		return nil, errors.New("tool node missing tool")
	}
	if n.Registry == nil {
		return nil, fmt.Errorf("tool node %q missing capability registry", n.id)
	}
	res, err := n.Registry.InvokeCapability(ctx, env, n.Tool.Name(), n.Args)
	if err != nil {
		return nil, err
	}
	envelope := attachCapabilityEnvelope(n.Registry, n.Tool, env, res, n.Args)
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
func (n *ConditionalNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	to, err := n.Condition(env)
	if err != nil {
		return nil, err
	}
	return &Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"next": to,
		},
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
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "approval.*"},
			WriteKeys:                []string{"approval.*"},
			AllowHistoryAccess:       true,
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
		},
	}
}

// Execute pauses execution until callback completes.
func (n *HumanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	if n.Callback != nil {
		if err := n.Callback(env); err != nil {
			return nil, err
		}
	}
	return &Result{NodeID: n.id, Success: true}, nil
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
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "plan.*", "react.*", "architect.*"},
			WriteKeys:                []string{},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassRoutingFlag, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 32,
		},
	}
}

// Execute completes immediately.
func (n *TerminalNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	return &Result{NodeID: n.id, Success: true}, nil
}

// errorFromString reconstructs an error from a stored message, enabling tool
// results that only record strings to participate in graph error handling.
func errorFromString(err string) error {
	if err == "" {
		return nil
	}
	return errors.New(err)
}

func resultFromToolExecution(nodeID string, res *core.ToolResult) *Result {
	if res == nil {
		return &Result{NodeID: nodeID, Success: true, Data: map[string]interface{}{}}
	}
	data := res.Data
	if data == nil {
		data = map[string]interface{}{}
	}
	return &Result{
		NodeID:   nodeID,
		Success:  res.Success,
		Data:     data,
		Metadata: res.Metadata,
		Error:    res.Error,
	}
}

func attachCapabilityEnvelope(registry CapabilityInvoker, tool Tool, env *contextdata.Envelope, res *core.ToolResult, args map[string]interface{}) *core.CapabilityResultEnvelope {
	if registry == nil || tool == nil || res == nil {
		return nil
	}
	if res.Metadata == nil {
		res.Metadata = map[string]interface{}{}
	}
	if res.Metadata["capability_envelope_created"] == true {
		return nil
	}

	desc, ok := res.Metadata["capability_descriptor"].(core.CapabilityDescriptor)
	if !ok || desc.ID == "" {
		desc, ok = registry.GetCapability(tool.Name())
		if !ok || desc.ID == "" {
			desc = core.ToolDescriptor(context.Background(), tool)
		}
	}

	var approval *core.ApprovalBinding
	if raw := res.Metadata["approval_binding"]; raw != nil {
		if typed, ok := raw.(*core.ApprovalBinding); ok {
			approval = typed
		}
	}
	if approval == nil {
		approval = core.ApprovalBindingFromCapability(desc, env.Snapshot(), args)
	}

	envelope := core.NewCapabilityResultEnvelope(desc, res, core.ContentDispositionRaw, registry.CapturePolicySnapshot(), approval)
	if decision, ok := res.Metadata["insertion_decision"].(core.InsertionDecision); ok {
		envelope = core.ApplyInsertionDecision(envelope, decision)
	}
	res.Metadata["insertion_decision"] = envelope.Insertion
	res.Metadata["capability_envelope_created"] = true
	return envelope
}
