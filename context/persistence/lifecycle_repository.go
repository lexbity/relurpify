package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// LifecycleRepository implements contextports.LifecycleRepository using graphdb as the backend.
type LifecycleRepository struct {
	db *graphdb.Engine
}

// NewLifecycleRepository creates a new lifecycle repository backed by graphdb.
func NewLifecycleRepository(db *graphdb.Engine) *LifecycleRepository {
	return &LifecycleRepository{db: db}
}

// Close closes the repository and the underlying graphdb engine.
func (r *LifecycleRepository) Close() error {
	return r.db.Close(context.Background())
}

// Workflow operations

func (r *LifecycleRepository) CreateWorkflow(workflow contextports.WorkflowRecord) error {
	if workflow.WorkflowID == "" {
		workflow.WorkflowID = graphdb.GenerateID("wf")
	}
	if workflow.StartedAt.IsZero() {
		workflow.StartedAt = time.Now().UTC()
	}

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
	return r.db.UpsertNode(context.TODO(), node)
}

func (r *LifecycleRepository) GetWorkflow(workflowID string) (*contextports.WorkflowRecord, error) {
	node, ok := r.db.GetNode(workflowID)
	if !ok {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	return r.unmarshalWorkflow(node)
}

func (r *LifecycleRepository) ListWorkflows(agentID string) ([]contextports.WorkflowRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindWorkflow)
	workflows := make([]contextports.WorkflowRecord, 0, len(nodes))
	for _, node := range nodes {
		workflow, err := r.unmarshalWorkflow(node)
		if err != nil {
			continue
		}
		if agentID != "" && workflow.AgentID != agentID {
			continue
		}
		workflows = append(workflows, *workflow)
	}
	return workflows, nil
}

// Run operations

func (r *LifecycleRepository) CreateRun(run contextports.WorkflowRunRecord) error {
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

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	// Link to workflow
	if run.WorkflowID != "" {
		return r.db.Link(context.TODO(), run.WorkflowID, run.RunID, graphdb.EdgeKindWorkflowHasRun, "", 0, nil)
	}
	return nil
}

func (r *LifecycleRepository) GetRun(runID string) (*contextports.WorkflowRunRecord, error) {
	node, ok := r.db.GetNode(runID)
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}
	return r.unmarshalRun(node)
}

func (r *LifecycleRepository) ListRuns(workflowID string) ([]contextports.WorkflowRunRecord, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindWorkflowRun)
		runs := make([]contextports.WorkflowRunRecord, 0, len(nodes))
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
	runs := make([]contextports.WorkflowRunRecord, 0, len(edges))
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

func (r *LifecycleRepository) UpdateRunStatus(runID string, status string) error {
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
	return r.db.UpsertNode(context.TODO(), node)
}

// Delegation operations

func (r *LifecycleRepository) UpsertDelegation(entry contextports.DelegationEntry) error {
	if entry.DelegationID == "" {
		entry.DelegationID = graphdb.GenerateID("del")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
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

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	// Link to workflow
	if entry.WorkflowID != "" {
		if err := r.db.Link(context.TODO(), entry.WorkflowID, entry.DelegationID, graphdb.EdgeKindWorkflowHasDelegation, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if entry.RunID != "" {
		if err := r.db.Link(context.TODO(), entry.RunID, entry.DelegationID, graphdb.EdgeKindWorkflowHasDelegation, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetDelegation(delegationID string) (*contextports.DelegationEntry, error) {
	node, ok := r.db.GetNode(delegationID)
	if !ok {
		return nil, fmt.Errorf("delegation not found: %s", delegationID)
	}
	return r.unmarshalDelegation(node)
}

func (r *LifecycleRepository) ListDelegations(workflowID string) ([]contextports.DelegationEntry, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindDelegation)
		delegations := make([]contextports.DelegationEntry, 0, len(nodes))
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
	delegations := make([]contextports.DelegationEntry, 0, len(edges))
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

func (r *LifecycleRepository) ListDelegationsByRun(runID string) ([]contextports.DelegationEntry, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindWorkflowHasDelegation)
	delegations := make([]contextports.DelegationEntry, 0, len(edges))
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

func (r *LifecycleRepository) AppendDelegationTransition(transition contextports.DelegationTransitionEntry) error {
	transID := graphdb.GenerateID("trans")
	if transition.Timestamp.IsZero() {
		transition.Timestamp = time.Now().UTC()
	}

	props, err := r.marshalDelegationTransition(transition)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     transID,
		Kind:   graphdb.NodeKindDelegationTransition,
		Props:  props,
		Labels: []string{"delegation_transition"},
	}

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	if transition.DelegationID != "" {
		return r.db.Link(context.TODO(), transition.DelegationID, transID, graphdb.EdgeKindDelegationHasTransition, "", 0, nil)
	}
	return nil
}

func (r *LifecycleRepository) ListDelegationTransitions(delegationID string) ([]contextports.DelegationTransitionEntry, error) {
	edges := r.db.GetOutEdges(delegationID, graphdb.EdgeKindDelegationHasTransition)
	transitions := make([]contextports.DelegationTransitionEntry, 0, len(edges))
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

func (r *LifecycleRepository) AppendEvent(event contextports.WorkflowEventRecord) error {
	if event.EventID == "" {
		event.EventID = graphdb.GenerateSequenceID("evt", uint64(event.Sequence))
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
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

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	// Link to workflow
	if event.WorkflowID != "" {
		if err := r.db.Link(context.TODO(), event.WorkflowID, event.EventID, graphdb.EdgeKindWorkflowHasEvent, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if event.RunID != "" {
		if err := r.db.Link(context.TODO(), event.RunID, event.EventID, graphdb.EdgeKindWorkflowRunHasEvent, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) ListEvents(workflowID string, limit int) ([]contextports.WorkflowEventRecord, error) {
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

func (r *LifecycleRepository) ListEventsByRun(runID string, limit int) ([]contextports.WorkflowEventRecord, error) {
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

func (r *LifecycleRepository) UpsertArtifact(artifact contextports.WorkflowArtifactRecord) error {
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

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	// Link to workflow
	if artifact.WorkflowID != "" {
		if err := r.db.Link(context.TODO(), artifact.WorkflowID, artifact.ArtifactID, graphdb.EdgeKindWorkflowHasArtifact, "", 0, nil); err != nil {
			return err
		}
	}

	// Link to run if provided
	if artifact.RunID != "" {
		if err := r.db.Link(context.TODO(), artifact.RunID, artifact.ArtifactID, graphdb.EdgeKindWorkflowRunHasArtifact, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetArtifact(artifactID string) (*contextports.WorkflowArtifactRecord, error) {
	node, ok := r.db.GetNode(artifactID)
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", artifactID)
	}
	return r.unmarshalArtifact(node)
}

func (r *LifecycleRepository) ListArtifacts(workflowID string) ([]contextports.WorkflowArtifactRecord, error) {
	if workflowID == "" {
		nodes := r.db.ListNodes(graphdb.NodeKindWorkflowArtifact)
		artifacts := make([]contextports.WorkflowArtifactRecord, 0, len(nodes))
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
	artifacts := make([]contextports.WorkflowArtifactRecord, 0, len(edges))
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

func (r *LifecycleRepository) ListArtifactsByRun(runID string) ([]contextports.WorkflowArtifactRecord, error) {
	edges := r.db.GetOutEdges(runID, graphdb.EdgeKindWorkflowRunHasArtifact)
	artifacts := make([]contextports.WorkflowArtifactRecord, 0, len(edges))
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

func (r *LifecycleRepository) UpsertLineageBinding(binding contextports.LineageBindingRecord) error {
	if binding.BindingID == "" {
		binding.BindingID = graphdb.GenerateID("lb")
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
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

	if err := r.db.UpsertNode(context.TODO(), node); err != nil {
		return err
	}

	if binding.WorkflowID != "" {
		if err := r.db.Link(context.TODO(), binding.WorkflowID, binding.BindingID, graphdb.EdgeKindLineageBindingForWorkflow, "", 0, nil); err != nil {
			return err
		}
	}

	if binding.FromRunID != "" {
		if err := r.db.Link(context.TODO(), binding.FromRunID, binding.BindingID, graphdb.EdgeKindLineageBindingForRun, "", 0, nil); err != nil {
			return err
		}
	}

	return nil
}

func (r *LifecycleRepository) GetLineageBinding(bindingID string) (*contextports.LineageBindingRecord, error) {
	node, ok := r.db.GetNode(bindingID)
	if !ok {
		return nil, fmt.Errorf("lineage binding not found: %s", bindingID)
	}
	return r.unmarshalLineageBinding(node)
}

func (r *LifecycleRepository) FindLineageBinding(fromEntityID, toEntityID string) (*contextports.LineageBindingRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindLineageBinding)
	for _, node := range nodes {
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		if binding.FromEntityID == fromEntityID && binding.ToEntityID == toEntityID {
			return binding, nil
		}
	}
	return nil, fmt.Errorf("lineage binding not found for from=%s to=%s", fromEntityID, toEntityID)
}

func (r *LifecycleRepository) FindLineageBindingsByFrom(fromEntityID string) ([]contextports.LineageBindingRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindLineageBinding)
	var bindings []contextports.LineageBindingRecord
	for _, node := range nodes {
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		if binding.FromEntityID == fromEntityID {
			bindings = append(bindings, *binding)
		}
	}
	return bindings, nil
}

func (r *LifecycleRepository) FindLineageBindingsByTo(toEntityID string) ([]contextports.LineageBindingRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindLineageBinding)
	var bindings []contextports.LineageBindingRecord
	for _, node := range nodes {
		binding, err := r.unmarshalLineageBinding(node)
		if err != nil {
			continue
		}
		if binding.ToEntityID == toEntityID {
			bindings = append(bindings, *binding)
		}
	}
	return bindings, nil
}

// Marshal/unmarshal helpers

func (r *LifecycleRepository) marshalWorkflow(w contextports.WorkflowRecord) (json.RawMessage, error) {
	return json.Marshal(w)
}

func (r *LifecycleRepository) unmarshalWorkflow(node graphdb.NodeRecord) (*contextports.WorkflowRecord, error) {
	var w contextports.WorkflowRecord
	if err := json.Unmarshal(node.Props, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *LifecycleRepository) marshalRun(run contextports.WorkflowRunRecord) (json.RawMessage, error) {
	return json.Marshal(run)
}

func (r *LifecycleRepository) unmarshalRun(node graphdb.NodeRecord) (*contextports.WorkflowRunRecord, error) {
	var run contextports.WorkflowRunRecord
	if err := json.Unmarshal(node.Props, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *LifecycleRepository) marshalDelegation(d contextports.DelegationEntry) (json.RawMessage, error) {
	return json.Marshal(d)
}

func (r *LifecycleRepository) unmarshalDelegation(node graphdb.NodeRecord) (*contextports.DelegationEntry, error) {
	var d contextports.DelegationEntry
	if err := json.Unmarshal(node.Props, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *LifecycleRepository) marshalDelegationTransition(t contextports.DelegationTransitionEntry) (json.RawMessage, error) {
	return json.Marshal(t)
}

func (r *LifecycleRepository) unmarshalDelegationTransition(node graphdb.NodeRecord) (*contextports.DelegationTransitionEntry, error) {
	var t contextports.DelegationTransitionEntry
	if err := json.Unmarshal(node.Props, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *LifecycleRepository) marshalEvent(e contextports.WorkflowEventRecord) (json.RawMessage, error) {
	return json.Marshal(e)
}

func (r *LifecycleRepository) unmarshalEvent(node graphdb.NodeRecord) (*contextports.WorkflowEventRecord, error) {
	var e contextports.WorkflowEventRecord
	if err := json.Unmarshal(node.Props, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *LifecycleRepository) marshalArtifact(a contextports.WorkflowArtifactRecord) (json.RawMessage, error) {
	return json.Marshal(a)
}

func (r *LifecycleRepository) unmarshalArtifact(node graphdb.NodeRecord) (*contextports.WorkflowArtifactRecord, error) {
	var a contextports.WorkflowArtifactRecord
	if err := json.Unmarshal(node.Props, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *LifecycleRepository) marshalLineageBinding(lb contextports.LineageBindingRecord) (json.RawMessage, error) {
	return json.Marshal(lb)
}

func (r *LifecycleRepository) unmarshalLineageBinding(node graphdb.NodeRecord) (*contextports.LineageBindingRecord, error) {
	var lb contextports.LineageBindingRecord
	if err := json.Unmarshal(node.Props, &lb); err != nil {
		return nil, err
	}
	return &lb, nil
}

func (r *LifecycleRepository) limitEvents(nodes []graphdb.NodeRecord, limit int) ([]contextports.WorkflowEventRecord, error) {
	events := make([]contextports.WorkflowEventRecord, 0, len(nodes))
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
