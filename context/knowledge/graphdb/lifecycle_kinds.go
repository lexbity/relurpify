package graphdb

// Lifecycle node kinds for agentlifecycle persistence.
const (
	NodeKindWorkflow             NodeKind = "workflow"
	NodeKindWorkflowRun          NodeKind = "workflow_run"
	NodeKindDelegation           NodeKind = "delegation"
	NodeKindDelegationTransition NodeKind = "delegation_transition"
	NodeKindWorkflowEvent        NodeKind = "workflow_event"
	NodeKindWorkflowArtifact     NodeKind = "workflow_artifact"
	NodeKindLineageBinding       NodeKind = "lineage_binding"
	NodeKindSelectionDecision    NodeKind = "selection_decision"
)

// Compiler node kinds for compiler-specific persistence.
const (
	NodeKindCompilerCompilation NodeKind = "compiler_compilation"
	NodeKindCompilerCache       NodeKind = "compiler_cache"
	NodeKindCompilerArtifact    NodeKind = "compiler_artifact"
)

// Lifecycle edge kinds for agentlifecycle persistence.
const (
	EdgeKindWorkflowHasRun            EdgeKind = "workflow_has_run"
	EdgeKindWorkflowHasDelegation     EdgeKind = "workflow_has_delegation"
	EdgeKindWorkflowHasEvent          EdgeKind = "workflow_has_event"
	EdgeKindWorkflowHasArtifact       EdgeKind = "workflow_has_artifact"
	EdgeKindWorkflowRunHasEvent       EdgeKind = "workflow_run_has_event"
	EdgeKindWorkflowRunHasArtifact    EdgeKind = "workflow_run_has_artifact"
	EdgeKindDelegationHasTransition   EdgeKind = "delegation_has_transition"
	EdgeKindLineageBindingForRun      EdgeKind = "lineage_binding_for_run"
	EdgeKindLineageBindingForWorkflow EdgeKind = "lineage_binding_for_workflow"
	// EdgeKindWorkflowHasSelectionDecision links a workflow to its selection
	// decision records (D11); EdgeKindRunHasSelectionDecision links the run
	// whose dispatch produced the record.
	EdgeKindWorkflowHasSelectionDecision EdgeKind = "workflow_has_selection_decision"
	EdgeKindRunHasSelectionDecision      EdgeKind = "run_has_selection_decision"
)
