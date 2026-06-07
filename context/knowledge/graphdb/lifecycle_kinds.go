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
)
