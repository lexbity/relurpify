package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
)

// LifecycleRepository implements agentlifecycle.Repository using graphdb as the backend.
type LifecycleRepository struct {
	db *graphdb.Engine
}

// NewLifecycleRepository creates a new lifecycle repository backed by graphdb.
func NewLifecycleRepository(db *graphdb.Engine) *LifecycleRepository {
	return &LifecycleRepository{db: db}
}

// Close closes the repository and the underlying graphdb engine.
func (r *LifecycleRepository) Close() error {
	return r.db.Close()
}

// Workflow operations

func (r *LifecycleRepository) CreateWorkflow(ctx context.Context, workflow agentlifecycle.WorkflowRecord) error {
	if workflow.WorkflowID == "" {
		workflow.WorkflowID = graphdb.GenerateID("wf")
	}
	if workflow.CreatedAt.IsZero() {
		workflow.CreatedAt = time.Now().UTC()
	}
	workflow.UpdatedAt = time.Now().UTC()

	props, err := r.marshalWorkflow(workflow)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     workflow.WorkflowID,
		Kind:   graphdb.NodeKindWorkflow,
		Props:  props,
		Labels: []string{"workflow"},
	}
	return r.db.UpsertNode(node)
}

func (r *LifecycleRepository) GetWorkflow(ctx context.Context, workflowID string) (*agentlifecycle.WorkflowRecord, error) {
	node, ok := r.db.GetNode(workflowID)
	if !ok {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	return r.unmarshalWorkflow(node)
}

func (r *LifecycleRepository) ListWorkflows(ctx context.Context) ([]agentlifecycle.WorkflowRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindWorkflow)
	workflows := make([]agentlifecycle.WorkflowRecord, 0, len(nodes))
	for _, node := range nodes {
		workflow, err := r.unmarshalWorkflow(node)
		if err != nil {
			continue // Skip malformed records
		}
		workflows = append(workflows, *workflow)
	}
	return workflows, nil
}

// Run operations

func (r *LifecycleRepository) CreateRun(ctx context.Context, run agentlifecycle.WorkflowRunRecord) error {
	if run.RunID == "" {
		run.RunID = graphdb.GenerateID("run")
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}

	props, err := r.marshalRun(run)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     run.RunID,
		Kind:   graphdb.NodeKindWorkflowRun,
		Props:  props,
		Labels: []string{"workflow_run"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to workflow
	if run.WorkflowID != "" {
		return r.db.Link(run.WorkflowID, run.RunID, graphdb.EdgeKindWorkflowHasRun, "", 0, nil)
	}
	return nil
}

func (r *LifecycleRepository) GetRun(ctx context.Context, runID string) (*agentlifecycle.WorkflowRunRecord, error) {
	node, ok := r.db.GetNode(runID)
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	return r.unmarshalRun(node)
}

func (r *LifecycleRepository) ListRuns(ctx context.Context, workflowID string) ([]agentlifecycle.WorkflowRunRecord, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindWorkflowRun)
		runs := make([]agentlifecycle.WorkflowRunRecord, 0, len(nodes))
		for _, node := range nodes {
			run, err := r.unmarshalRun(node)
			if err != nil {
				continue
			}
			runs = append(runs, *run)
		}
		return runs, nil
	}

	// List runs for a specific workflow via edges
	edges := r.db.GetOutEdges(workflowID, graphdb.EdgeKindWorkflowHasRun)
	runs := make([]agentlifecycle.WorkflowRunRecord, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		run, err := r.unmarshalRun(node)
		if err != nil {
			continue
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

func (r *LifecycleRepository) UpdateRunStatus(ctx context.Context, runID string, status string) error {
	node, ok := r.db.GetNode(runID)
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	run, err := r.unmarshalRun(node)
	if err != nil {
		return err
	}

	run.Status = status
	if status == "completed" || status == "failed" || status == "cancelled" {
		now := time.Now().UTC()
		run.CompletedAt = &now
	}

	props, err := r.marshalRun(*run)
	if err != nil {
		return err
	}

	node.Props = props
	return r.db.UpsertNode(node)
}

// Delegation operations

func (r *LifecycleRepository) UpsertDelegation(ctx context.Context, entry agentlifecycle.DelegationEntry) error {
	if entry.DelegationID == "" {
		entry.DelegationID = graphdb.GenerateID("del")
	}
	if entry.StartedAt.IsZero() {
		entry.StartedAt = time.Now().UTC()
	}
	entry.UpdatedAt = time.Now().UTC()

	props, err := r.marshalDelegation(entry)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     entry.DelegationID,
		Kind:   graphdb.NodeKindDelegation,
		Props:  props,
		Labels: []string{"delegation"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to workflow
	if entry.WorkflowID != "" {
		if err := r.db.Link(entry.WorkflowID, entry.DelegationID, graphdb.EdgeKindWorkflowHasDelegation, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if entry.RunID != "" {
		if err := r.db.Link(entry.RunID, entry.DelegationID, graphdb.EdgeKindWorkflowHasDelegation, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetDelegation(ctx context.Context, delegationID string) (*agentlifecycle.DelegationEntry, error) {
	node, ok := r.db.GetNode(delegationID)
	if !ok {
		return nil, fmt.Errorf("delegation not found: %s", delegationID)
	}
	return r.unmarshalDelegation(node)
}

func (r *LifecycleRepository) ListDelegations(ctx context.Context, workflowID string) ([]agentlifecycle.DelegationEntry, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindDelegation)
		delegations := make([]agentlifecycle.DelegationEntry, 0, len(nodes))
		for _, node := range nodes {
			delegate, err := r.unmarshalDelegation(node)
			if err != nil {
				continue
			}
			delegations = append(delegations, *delegate)
		}
		return delegations, nil
	}

	edges := r.db.GetOutEdges(workflowID, graphdb.EdgeKindWorkflowHasDelegation)
	delegations := make([]agentlifecycle.DelegationEntry, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		delegate, err := r.unmarshalDelegation(node)
		if err != nil {
			continue
		}
		delegations = append(delegations, *delegate)
	}
	return delegations, nil
}

func (r *LifecycleRepository) ListDelegationsByRun(ctx context.Context, runID string) ([]agentlifecycle.DelegationEntry, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindWorkflowHasDelegation)
	delegations := make([]agentlifecycle.DelegationEntry, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		delegate, err := r.unmarshalDelegation(node)
		if err != nil {
			continue
		}
		delegations = append(delegations, *delegate)
	}
	return delegations, nil
}

func (r *LifecycleRepository) AppendDelegationTransition(ctx context.Context, transition agentlifecycle.DelegationTransitionEntry) error {
	if transition.TransitionID == "" {
		transition.TransitionID = graphdb.GenerateID("trans")
	}
	if transition.CreatedAt.IsZero() {
		transition.CreatedAt = time.Now().UTC()
	}

	props, err := r.marshalDelegationTransition(transition)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     transition.TransitionID,
		Kind:   graphdb.NodeKindDelegationTransition,
		Props:  props,
		Labels: []string{"delegation_transition"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to delegation
	if transition.DelegationID != "" {
		return r.db.Link(transition.DelegationID, transition.TransitionID, graphdb.EdgeKindDelegationHasTransition, "", 0, nil)
	}
	return nil
}

func (r *LifecycleRepository) ListDelegationTransitions(ctx context.Context, delegationID string) ([]agentlifecycle.DelegationTransitionEntry, error) {
	edges := r.db.GetOutEdges(delegationID, graphdb.EdgeKindDelegationHasTransition)
	transitions := make([]agentlifecycle.DelegationTransitionEntry, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		transition, err := r.unmarshalDelegationTransition(node)
		if err != nil {
			continue
		}
		transitions = append(transitions, *transition)
	}
	return transitions, nil
}

// Event operations

func (r *LifecycleRepository) AppendEvent(ctx context.Context, event agentlifecycle.WorkflowEventRecord) error {
	if event.EventID == "" {
		event.EventID = graphdb.GenerateSequenceID("evt", event.Sequence)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	props, err := r.marshalEvent(event)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     event.EventID,
		Kind:   graphdb.NodeKindWorkflowEvent,
		Props:  props,
		Labels: []string{"workflow_event"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to workflow
	if event.WorkflowID != "" {
		if err := r.db.Link(event.WorkflowID, event.EventID, graphdb.EdgeKindWorkflowHasEvent, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if event.RunID != "" {
		if err := r.db.Link(event.RunID, event.EventID, graphdb.EdgeKindWorkflowRunHasEvent, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) ListEvents(ctx context.Context, workflowID string, limit int) ([]agentlifecycle.WorkflowEventRecord, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindWorkflowEvent)
		return r.limitEvents(nodes, limit)
	}

	edges := r.db.GetOutEdges(workflowID, graphdb.EdgeKindWorkflowHasEvent)
	nodes := make([]graphdb.NodeRecord, 0, len(edges))
	for _, edge := range edges {
		if node, ok := r.db.GetNode(edge.TargetID); ok {
			nodes = append(nodes, node)
		}
	}
	return r.limitEvents(nodes, limit)
}

func (r *LifecycleRepository) ListEventsByRun(ctx context.Context, runID string, limit int) ([]agentlifecycle.WorkflowEventRecord, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindWorkflowRunHasEvent)
	nodes := make([]graphdb.NodeRecord, 0, len(edges))
	for _, edge := range edges {
		if node, ok := r.db.GetNode(edge.TargetID); ok {
			nodes = append(nodes, node)
		}
	}
	return r.limitEvents(nodes, limit)
}

// Artifact operations

func (r *LifecycleRepository) UpsertArtifact(ctx context.Context, artifact agentlifecycle.WorkflowArtifactRecord) error {
	if artifact.ArtifactID == "" {
		artifact.ArtifactID = graphdb.GenerateID("art")
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}

	props, err := r.marshalArtifact(artifact)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     artifact.ArtifactID,
		Kind:   graphdb.NodeKindWorkflowArtifact,
		Props:  props,
		Labels: []string{"workflow_artifact"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to workflow
	if artifact.WorkflowID != "" {
		if err := r.db.Link(artifact.WorkflowID, artifact.ArtifactID, graphdb.EdgeKindWorkflowHasArtifact, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if artifact.RunID != "" {
		if err := r.db.Link(artifact.RunID, artifact.ArtifactID, graphdb.EdgeKindWorkflowRunHasArtifact, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetArtifact(ctx context.Context, artifactID string) (*agentlifecycle.WorkflowArtifactRecord, error) {
	node, ok := r.db.GetNode(artifactID)
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", artifactID)
	}
	return r.unmarshalArtifact(node)
}

func (r *LifecycleRepository) ListArtifacts(ctx context.Context, workflowID string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindWorkflowArtifact)
		artifacts := make([]agentlifecycle.WorkflowArtifactRecord, 0, len(nodes))
		for _, node := range nodes {
			artifact, err := r.unmarshalArtifact(node)
			if err != nil {
				continue
			}
			artifacts = append(artifacts, *artifact)
		}
		return artifacts, nil
	}

	edges := r.db.GetOutEdges(workflowID, graphdb.EdgeKindWorkflowHasArtifact)
	artifacts := make([]agentlifecycle.WorkflowArtifactRecord, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		artifact, err := r.unmarshalArtifact(node)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, *artifact)
	}
	return artifacts, nil
}

func (r *LifecycleRepository) ListArtifactsByRun(ctx context.Context, runID string) ([]agentlifecycle.WorkflowArtifactRecord, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindWorkflowRunHasArtifact)
	artifacts := make([]agentlifecycle.WorkflowArtifactRecord, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		artifact, err := r.unmarshalArtifact(node)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, *artifact)
	}
	return artifacts, nil
}

// Lineage binding operations

func (r *LifecycleRepository) UpsertLineageBinding(ctx context.Context, binding agentlifecycle.LineageBindingRecord) error {
	if binding.BindingID == "" {
		binding.BindingID = graphdb.GenerateID("lb")
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	binding.UpdatedAt = time.Now().UTC()

	props, err := r.marshalLineageBinding(binding)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     binding.BindingID,
		Kind:   graphdb.NodeKindLineageBinding,
		Props:  props,
		Labels: []string{"lineage_binding"},
	}

	if err := r.db.UpsertNode(node); err != nil {
		return err
	}

	// Link to workflow
	if binding.WorkflowID != "" {
		if err := r.db.Link(binding.WorkflowID, binding.BindingID, graphdb.EdgeKindLineageBindingForWorkflow, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if binding.RunID != "" {
		if err := r.db.Link(binding.RunID, binding.BindingID, graphdb.EdgeKindLineageBindingForRun, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetLineageBinding(ctx context.Context, bindingID string) (*agentlifecycle.LineageBindingRecord, error) {
	node, ok := r.db.GetNode(bindingID)
	if !ok {
		return nil, fmt.Errorf("lineage binding not found: %s", bindingID)
	}
	return r.unmarshalLineageBinding(node)
}

func (r *LifecycleRepository) FindLineageBindingByWorkflow(ctx context.Context, workflowID string) ([]agentlifecycle.LineageBindingRecord, error) {
	edges := r.db.GetOutEdges(workflowID, graphdb.EdgeKindLineageBindingForWorkflow)
	bindings := make([]agentlifecycle.LineageBindingRecord, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		bindings = append(bindings, *binding)
	}
	return bindings, nil
}

func (r *LifecycleRepository) FindLineageBindingByRun(ctx context.Context, runID string) ([]agentlifecycle.LineageBindingRecord, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindLineageBindingForRun)
	bindings := make([]agentlifecycle.LineageBindingRecord, 0, len(edges))
	for _, edge := range edges {
		node, ok := r.db.GetNode(edge.TargetID)
		if !ok {
			continue
		}
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		bindings = append(bindings, *binding)
	}
	return bindings, nil
}

func (r *LifecycleRepository) FindLineageBindingByLineageID(ctx context.Context, lineageID string) (*agentlifecycle.LineageBindingRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindLineageBinding)
	for _, node := range nodes {
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		if binding.LineageID == lineageID {
			return binding, nil
		}
	}
	return nil, fmt.Errorf("lineage binding not found for lineage ID: %s", lineageID)
}

func (r *LifecycleRepository) FindLineageBindingByAttemptID(ctx context.Context, attemptID string) (*agentlifecycle.LineageBindingRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindLineageBinding)
	for _, node := range nodes {
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		if binding.AttemptID == attemptID {
			return binding, nil
		}
	}
	return nil, fmt.Errorf("lineage binding not found for attempt ID: %s", attemptID)
}

// Marshal/unmarshal helpers

func (r *LifecycleRepository) marshalWorkflow(w agentlifecycle.WorkflowRecord) (json.RawMessage, error) {
	return json.Marshal(w)
}

func (r *LifecycleRepository) unmarshalWorkflow(node graphdb.NodeRecord) (*agentlifecycle.WorkflowRecord, error) {
	var w agentlifecycle.WorkflowRecord
	if err := json.Unmarshal(node.Props, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *LifecycleRepository) marshalRun(run agentlifecycle.WorkflowRunRecord) (json.RawMessage, error) {
	return json.Marshal(run)
}

func (r *LifecycleRepository) unmarshalRun(node graphdb.NodeRecord) (*agentlifecycle.WorkflowRunRecord, error) {
	var run agentlifecycle.WorkflowRunRecord
	if err := json.Unmarshal(node.Props, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *LifecycleRepository) marshalDelegation(d agentlifecycle.DelegationEntry) (json.RawMessage, error) {
	return json.Marshal(d)
}

func (r *LifecycleRepository) unmarshalDelegation(node graphdb.NodeRecord) (*agentlifecycle.DelegationEntry, error) {
	var d agentlifecycle.DelegationEntry
	if err := json.Unmarshal(node.Props, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *LifecycleRepository) marshalDelegationTransition(t agentlifecycle.DelegationTransitionEntry) (json.RawMessage, error) {
	return json.Marshal(t)
}

func (r *LifecycleRepository) unmarshalDelegationTransition(node graphdb.NodeRecord) (*agentlifecycle.DelegationTransitionEntry, error) {
	var t agentlifecycle.DelegationTransitionEntry
	if err := json.Unmarshal(node.Props, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *LifecycleRepository) marshalEvent(e agentlifecycle.WorkflowEventRecord) (json.RawMessage, error) {
	return json.Marshal(e)
}

func (r *LifecycleRepository) unmarshalEvent(node graphdb.NodeRecord) (*agentlifecycle.WorkflowEventRecord, error) {
	var e agentlifecycle.WorkflowEventRecord
	if err := json.Unmarshal(node.Props, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *LifecycleRepository) marshalArtifact(a agentlifecycle.WorkflowArtifactRecord) (json.RawMessage, error) {
	return json.Marshal(a)
}

func (r *LifecycleRepository) unmarshalArtifact(node graphdb.NodeRecord) (*agentlifecycle.WorkflowArtifactRecord, error) {
	var a agentlifecycle.WorkflowArtifactRecord
	if err := json.Unmarshal(node.Props, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *LifecycleRepository) marshalLineageBinding(lb agentlifecycle.LineageBindingRecord) (json.RawMessage, error) {
	return json.Marshal(lb)
}

func (r *LifecycleRepository) unmarshalLineageBinding(node graphdb.NodeRecord) (*agentlifecycle.LineageBindingRecord, error) {
	var lb agentlifecycle.LineageBindingRecord
	if err := json.Unmarshal(node.Props, &lb); err != nil {
		return nil, err
	}
	return &lb, nil
}

func (r *LifecycleRepository) limitEvents(nodes []graphdb.NodeRecord, limit int) ([]agentlifecycle.WorkflowEventRecord, error) {
	events := make([]agentlifecycle.WorkflowEventRecord, 0, len(nodes))
	for _, node := range nodes {
		event, err := r.unmarshalEvent(node)
		if err != nil {
			continue
		}
		events = append(events, *event)
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}
