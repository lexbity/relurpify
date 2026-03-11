package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// MemoryClass identifies the lifecycle and reuse contract for stored memory.
type MemoryClass string

const (
	MemoryClassWorking     MemoryClass = "working"
	MemoryClassDeclarative MemoryClass = "declarative"
	MemoryClassProcedural  MemoryClass = "procedural"
)

// StateDataClass categorizes data written into graph-visible state.
type StateDataClass string

const (
	StateDataClassTaskMetadata    StateDataClass = "task-metadata"
	StateDataClassStepMetadata    StateDataClass = "step-metadata"
	StateDataClassRoutingFlag     StateDataClass = "routing-flag"
	StateDataClassArtifactRef     StateDataClass = "artifact-ref"
	StateDataClassMemoryRef       StateDataClass = "memory-ref"
	StateDataClassStructuredState StateDataClass = "structured-state"
	StateDataClassTranscript      StateDataClass = "transcript"
	StateDataClassRawPayload      StateDataClass = "raw-payload"
	StateDataClassRetrievalDump   StateDataClass = "retrieval-dump"
	StateDataClassSubagentHistory StateDataClass = "subagent-history"
)

// ArtifactReference is the preferred graph-state representation for large raw
// outputs. State should typically carry this reference instead of inline payload.
type ArtifactReference struct {
	ArtifactID   string            `json:"artifact_id,omitempty" yaml:"artifact_id,omitempty"`
	WorkflowID   string            `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	RunID        string            `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	StepRunID    string            `json:"step_run_id,omitempty" yaml:"step_run_id,omitempty"`
	Kind         string            `json:"kind,omitempty" yaml:"kind,omitempty"`
	ContentType  string            `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	StorageKind  string            `json:"storage_kind,omitempty" yaml:"storage_kind,omitempty"`
	URI          string            `json:"uri,omitempty" yaml:"uri,omitempty"`
	Summary      string            `json:"summary,omitempty" yaml:"summary,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	RawSizeBytes int64             `json:"raw_size_bytes,omitempty" yaml:"raw_size_bytes,omitempty"`
}

// MemoryReference points graph state at durable declarative or procedural memory.
type MemoryReference struct {
	MemoryClass MemoryClass       `json:"memory_class,omitempty" yaml:"memory_class,omitempty"`
	Scope       string            `json:"scope,omitempty" yaml:"scope,omitempty"`
	RecordKey   string            `json:"record_key,omitempty" yaml:"record_key,omitempty"`
	Store       string            `json:"store,omitempty" yaml:"store,omitempty"`
	Summary     string            `json:"summary,omitempty" yaml:"summary,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// StateBoundaryPolicy declares what a node may read and write in graph state.
// Phase 3 uses this as a vocabulary and lint surface; later phases may enforce it.
type StateBoundaryPolicy struct {
	ReadKeys                 []string         `json:"read_keys,omitempty" yaml:"read_keys,omitempty"`
	WriteKeys                []string         `json:"write_keys,omitempty" yaml:"write_keys,omitempty"`
	AllowHistoryAccess       bool             `json:"allow_history_access,omitempty" yaml:"allow_history_access,omitempty"`
	AllowRetrieval           bool             `json:"allow_retrieval,omitempty" yaml:"allow_retrieval,omitempty"`
	AllowedMemoryClasses     []MemoryClass    `json:"allowed_memory_classes,omitempty" yaml:"allowed_memory_classes,omitempty"`
	AllowedDataClasses       []StateDataClass `json:"allowed_data_classes,omitempty" yaml:"allowed_data_classes,omitempty"`
	MaxStateEntryBytes       int              `json:"max_state_entry_bytes,omitempty" yaml:"max_state_entry_bytes,omitempty"`
	MaxInlineCollectionItems int              `json:"max_inline_collection_items,omitempty" yaml:"max_inline_collection_items,omitempty"`
	PreferArtifactReferences bool             `json:"prefer_artifact_references,omitempty" yaml:"prefer_artifact_references,omitempty"`
}

// StateBoundaryViolation reports a state-boundary lint failure.
type StateBoundaryViolation struct {
	Key       string         `json:"key,omitempty" yaml:"key,omitempty"`
	Code      string         `json:"code,omitempty" yaml:"code,omitempty"`
	Message   string         `json:"message,omitempty" yaml:"message,omitempty"`
	DataClass StateDataClass `json:"data_class,omitempty" yaml:"data_class,omitempty"`
	SizeBytes int            `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
}

// ValidateStateBoundaryPolicy checks that a node's declared boundary policy is coherent.
func ValidateStateBoundaryPolicy(policy StateBoundaryPolicy) error {
	for _, key := range append(append([]string{}, policy.ReadKeys...), policy.WriteKeys...) {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("state boundary contains empty key")
		}
	}
	for _, class := range policy.AllowedMemoryClasses {
		switch class {
		case MemoryClassWorking, MemoryClassDeclarative, MemoryClassProcedural:
		default:
			return fmt.Errorf("memory class %s invalid", class)
		}
	}
	for _, class := range policy.AllowedDataClasses {
		switch class {
		case StateDataClassTaskMetadata, StateDataClassStepMetadata, StateDataClassRoutingFlag, StateDataClassArtifactRef, StateDataClassMemoryRef, StateDataClassStructuredState, StateDataClassTranscript, StateDataClassRawPayload, StateDataClassRetrievalDump, StateDataClassSubagentHistory:
		default:
			return fmt.Errorf("state data class %s invalid", class)
		}
	}
	if policy.MaxStateEntryBytes < 0 {
		return fmt.Errorf("max_state_entry_bytes must be >= 0")
	}
	if policy.MaxInlineCollectionItems < 0 {
		return fmt.Errorf("max_inline_collection_items must be >= 0")
	}
	return nil
}

// LintStateMap flags state payloads that violate a declared boundary policy.
func LintStateMap(state map[string]interface{}, policy StateBoundaryPolicy) []StateBoundaryViolation {
	if len(state) == 0 {
		return nil
	}
	allowedData := make(map[StateDataClass]struct{}, len(policy.AllowedDataClasses))
	for _, class := range policy.AllowedDataClasses {
		allowedData[class] = struct{}{}
	}
	allowedMemory := make(map[MemoryClass]struct{}, len(policy.AllowedMemoryClasses))
	for _, class := range policy.AllowedMemoryClasses {
		allowedMemory[class] = struct{}{}
	}

	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var violations []StateBoundaryViolation
	for _, key := range keys {
		value := state[key]
		dataClass := classifyStateValue(key, value)
		sizeBytes := estimateStateValueBytes(value)
		if len(allowedData) > 0 {
			if _, ok := allowedData[dataClass]; !ok {
				violations = append(violations, StateBoundaryViolation{
					Key:       key,
					Code:      "disallowed_data_class",
					Message:   fmt.Sprintf("state key %s stores %s outside the declared boundary", key, dataClass),
					DataClass: dataClass,
					SizeBytes: sizeBytes,
				})
			}
		}
		if !policy.AllowHistoryAccess && dataClass == StateDataClassTranscript {
			violations = append(violations, StateBoundaryViolation{
				Key:       key,
				Code:      "history_access_disallowed",
				Message:   fmt.Sprintf("state key %s stores transcript/history outside the declared boundary", key),
				DataClass: dataClass,
				SizeBytes: sizeBytes,
			})
		}
		if !policy.AllowRetrieval && dataClass == StateDataClassRetrievalDump {
			violations = append(violations, StateBoundaryViolation{
				Key:       key,
				Code:      "retrieval_access_disallowed",
				Message:   fmt.Sprintf("state key %s stores retrieval output outside the declared boundary", key),
				DataClass: dataClass,
				SizeBytes: sizeBytes,
			})
		}
		if policy.MaxStateEntryBytes > 0 && sizeBytes > policy.MaxStateEntryBytes {
			violations = append(violations, StateBoundaryViolation{
				Key:       key,
				Code:      "oversize_state_entry",
				Message:   fmt.Sprintf("state key %s is %d bytes; limit is %d", key, sizeBytes, policy.MaxStateEntryBytes),
				DataClass: dataClass,
				SizeBytes: sizeBytes,
			})
		}
		if policy.MaxInlineCollectionItems > 0 {
			if items := inlineCollectionItems(value); items > policy.MaxInlineCollectionItems {
				violations = append(violations, StateBoundaryViolation{
					Key:       key,
					Code:      "oversize_collection",
					Message:   fmt.Sprintf("state key %s stores %d inline items; limit is %d", key, items, policy.MaxInlineCollectionItems),
					DataClass: dataClass,
					SizeBytes: sizeBytes,
				})
			}
		}
		if policy.PreferArtifactReferences && dataClass == StateDataClassRawPayload {
			violations = append(violations, StateBoundaryViolation{
				Key:       key,
				Code:      "prefer_artifact_reference",
				Message:   fmt.Sprintf("state key %s stores raw payload inline; prefer ArtifactReference", key),
				DataClass: dataClass,
				SizeBytes: sizeBytes,
			})
		}
		if memoryRef, ok := value.(MemoryReference); ok && len(allowedMemory) > 0 {
			if _, allowed := allowedMemory[memoryRef.MemoryClass]; !allowed {
				violations = append(violations, StateBoundaryViolation{
					Key:       key,
					Code:      "disallowed_memory_class",
					Message:   fmt.Sprintf("state key %s references %s memory outside the declared boundary", key, memoryRef.MemoryClass),
					DataClass: StateDataClassMemoryRef,
					SizeBytes: sizeBytes,
				})
			}
		}
	}
	return violations
}

func classifyStateValue(key string, value interface{}) StateDataClass {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(lowerKey, "task"):
		return StateDataClassTaskMetadata
	case strings.Contains(lowerKey, "step"):
		return StateDataClassStepMetadata
	case strings.Contains(lowerKey, "route"), strings.Contains(lowerKey, "next"), strings.Contains(lowerKey, "flag"):
		return StateDataClassRoutingFlag
	case strings.Contains(lowerKey, "history") || strings.Contains(lowerKey, "transcript"):
		return StateDataClassTranscript
	case strings.Contains(lowerKey, "retriev"):
		return StateDataClassRetrievalDump
	}

	switch typed := value.(type) {
	case ArtifactReference:
		return StateDataClassArtifactRef
	case *ArtifactReference:
		return StateDataClassArtifactRef
	case MemoryReference:
		return StateDataClassMemoryRef
	case *MemoryReference:
		return StateDataClassMemoryRef
	case []Interaction:
		return StateDataClassTranscript
	case []MemoryRecordEnvelope:
		return StateDataClassRetrievalDump
	case string:
		if strings.HasPrefix(strings.TrimSpace(typed), "workflow://artifact/") {
			return StateDataClassArtifactRef
		}
		if strings.HasPrefix(strings.TrimSpace(typed), "memory://") {
			return StateDataClassMemoryRef
		}
		if len(typed) > 512 {
			return StateDataClassRawPayload
		}
	}
	if looksLikeArtifactReference(value) {
		return StateDataClassArtifactRef
	}
	if looksLikeMemoryReference(value) {
		return StateDataClassMemoryRef
	}
	if looksLikeTranscript(value) {
		return StateDataClassTranscript
	}
	if looksLikeRetrievalDump(value) {
		return StateDataClassRetrievalDump
	}
	if estimateStateValueBytes(value) > 1024 {
		return StateDataClassRawPayload
	}
	return StateDataClassStructuredState
}

// MemoryRecordEnvelope is a compact retrieval-shaped wrapper used by state linting.
type MemoryRecordEnvelope struct {
	Key         string      `json:"key,omitempty" yaml:"key,omitempty"`
	MemoryClass MemoryClass `json:"memory_class,omitempty" yaml:"memory_class,omitempty"`
	Scope       string      `json:"scope,omitempty" yaml:"scope,omitempty"`
	Summary     string      `json:"summary,omitempty" yaml:"summary,omitempty"`
}

func estimateStateValueBytes(value interface{}) int {
	if value == nil {
		return 0
	}
	data, err := json.Marshal(value)
	if err != nil {
		return len(fmt.Sprint(value))
	}
	return len(data)
}

func inlineCollectionItems(value interface{}) int {
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []string:
		return len(typed)
	case []map[string]interface{}:
		return len(typed)
	case []Interaction:
		return len(typed)
	case []MemoryRecordEnvelope:
		return len(typed)
	}
	return 0
}

func looksLikeArtifactReference(value interface{}) bool {
	values, ok := value.(map[string]interface{})
	if !ok {
		return false
	}
	_, hasArtifactID := values["artifact_id"]
	_, hasRawRef := values["raw_ref"]
	_, hasURI := values["uri"]
	return hasArtifactID || hasRawRef || hasURI
}

func looksLikeMemoryReference(value interface{}) bool {
	values, ok := value.(map[string]interface{})
	if !ok {
		return false
	}
	_, hasClass := values["memory_class"]
	_, hasKey := values["record_key"]
	return hasClass || hasKey
}

func looksLikeTranscript(value interface{}) bool {
	switch typed := value.(type) {
	case []map[string]interface{}:
		if len(typed) == 0 {
			return false
		}
		_, hasRole := typed[0]["role"]
		_, hasContent := typed[0]["content"]
		return hasRole && hasContent
	}
	return false
}

func looksLikeRetrievalDump(value interface{}) bool {
	switch typed := value.(type) {
	case []map[string]interface{}:
		if len(typed) == 0 {
			return false
		}
		if _, ok := typed[0]["record_key"]; ok {
			return true
		}
		if _, ok := typed[0]["memory_class"]; ok {
			return true
		}
	}
	return false
}
