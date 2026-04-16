package keys

// Key is the type alias for runtime state key constants.
type Key = string

const (
	KeyEnvelope                   Key = "euclo.envelope"
	KeyClassification             Key = "euclo.classification"
	KeyMode                       Key = "euclo.mode"
	KeyExecutionProfile           Key = "euclo.execution_profile"
	KeyUnitOfWork                 Key = "euclo.unit_of_work"
	KeyModeResolution             Key = "euclo.mode_resolution"
	KeyExecutionProfileSelection  Key = "euclo.execution_profile_selection"
	KeySemanticInputs             Key = "euclo.semantic_inputs"
	KeyResolvedExecutionPolicy    Key = "euclo.resolved_execution_policy"
	KeyExecutorDescriptor         Key = "euclo.executor_descriptor"
	KeyPreClassifiedCapSeq        Key = "euclo.pre_classified_capability_sequence"
	KeyCapabilitySequenceOperator Key = "euclo.capability_sequence_operator"
	KeyClassificationSource       Key = "euclo.capability_classification_source"
	KeyClassificationMeta         Key = "euclo.capability_classification_meta"
	KeyUnitOfWorkHistory          Key = "euclo.unit_of_work_history"
	KeyExecutionStatus            Key = "euclo.execution_status"
	KeyCompiledExecution          Key = "euclo.compiled_execution"
	KeySequenceStepCompleted      Key = "euclo.sequence_step_completed"
	KeyORSelectedCapability       Key = "euclo.or_selected_capability"
	KeyRecoveryTrace              Key = "euclo.recovery_trace"
	KeyPipelineVerify             Key = "pipeline.verify"
)
