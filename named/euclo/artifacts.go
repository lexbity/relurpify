package euclo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

type ArtifactKind string

const (
	ArtifactKindIntake             ArtifactKind = "euclo.intake"
	ArtifactKindClassification     ArtifactKind = "euclo.classification"
	ArtifactKindModeResolution     ArtifactKind = "euclo.mode_resolution"
	ArtifactKindExecutionProfile   ArtifactKind = "euclo.execution_profile"
	ArtifactKindRetrievalPolicy    ArtifactKind = "euclo.retrieval_policy"
	ArtifactKindContextExpansion   ArtifactKind = "euclo.context_expansion"
	ArtifactKindCapabilityRouting  ArtifactKind = "euclo.capability_routing"
	ArtifactKindVerificationPolicy ArtifactKind = "euclo.verification_policy"
	ArtifactKindSuccessGate        ArtifactKind = "euclo.success_gate"
	ArtifactKindActionLog          ArtifactKind = "euclo.action_log"
	ArtifactKindProofSurface       ArtifactKind = "euclo.proof_surface"
	ArtifactKindWorkflowRetrieval  ArtifactKind = "euclo.workflow_retrieval"
	ArtifactKindExplore            ArtifactKind = "euclo.explore"
	ArtifactKindAnalyze            ArtifactKind = "euclo.analyze"
	ArtifactKindPlan               ArtifactKind = "euclo.plan"
	ArtifactKindEditIntent         ArtifactKind = "euclo.edit_intent"
	ArtifactKindEditExecution      ArtifactKind = "euclo.edit_execution"
	ArtifactKindVerification       ArtifactKind = "euclo.verification"
	ArtifactKindFinalReport        ArtifactKind = "euclo.final_report"
)

// Artifact is Euclo's normalized runtime view over workflow and state outputs.
type Artifact struct {
	ID       string
	Kind     ArtifactKind
	Summary  string
	Metadata map[string]any
	Payload  any
}

type WorkflowArtifactWriter interface {
	UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error
}

type WorkflowArtifactReader interface {
	ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error)
}

// CollectArtifactsFromState adapts legacy pipeline/workflow state into a small,
// typed Euclo artifact set.
func CollectArtifactsFromState(state *core.Context) []Artifact {
	if state == nil {
		return nil
	}
	defs := []struct {
		Key       string
		Kind      ArtifactKind
		Normalize func(any) (any, map[string]any)
	}{
		{Key: "euclo.envelope", Kind: ArtifactKindIntake},
		{Key: "euclo.classification", Kind: ArtifactKindClassification},
		{Key: "euclo.mode_resolution", Kind: ArtifactKindModeResolution},
		{Key: "euclo.execution_profile_selection", Kind: ArtifactKindExecutionProfile},
		{Key: "euclo.retrieval_policy", Kind: ArtifactKindRetrievalPolicy},
		{Key: "euclo.context_expansion", Kind: ArtifactKindContextExpansion},
		{Key: "euclo.capability_family_routing", Kind: ArtifactKindCapabilityRouting},
		{Key: "euclo.verification_policy", Kind: ArtifactKindVerificationPolicy},
		{Key: "euclo.success_gate", Kind: ArtifactKindSuccessGate},
		{Key: "euclo.action_log", Kind: ArtifactKindActionLog},
		{Key: "euclo.proof_surface", Kind: ArtifactKindProofSurface},
		{Key: "pipeline.workflow_retrieval", Kind: ArtifactKindWorkflowRetrieval, Normalize: normalizeWorkflowRetrieval},
		{Key: "pipeline.explore", Kind: ArtifactKindExplore},
		{Key: "pipeline.analyze", Kind: ArtifactKindAnalyze},
		{Key: "pipeline.plan", Kind: ArtifactKindPlan},
		{Key: "pipeline.code", Kind: ArtifactKindEditIntent, Normalize: normalizeEditIntent},
		{Key: "euclo.edit_execution", Kind: ArtifactKindEditExecution},
		{Key: "pipeline.verify", Kind: ArtifactKindVerification},
		{Key: "pipeline.final_output", Kind: ArtifactKindFinalReport},
	}

	out := make([]Artifact, 0, len(defs))
	for _, def := range defs {
		raw, ok := state.Get(def.Key)
		if !ok || raw == nil {
			continue
		}
		payload := raw
		metadata := map[string]any{"source_key": def.Key}
		if def.Normalize != nil {
			payload, metadata = def.Normalize(raw)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["source_key"] = def.Key
		}
		out = append(out, Artifact{
			ID:       strings.ReplaceAll(string(def.Kind), ".", "_"),
			Kind:     def.Kind,
			Summary:  artifactSummary(payload),
			Metadata: metadata,
			Payload:  payload,
		})
	}
	return out
}

func PersistWorkflowArtifacts(ctx context.Context, store WorkflowArtifactWriter, workflowID, runID string, artifacts []Artifact) error {
	if store == nil || strings.TrimSpace(workflowID) == "" || len(artifacts) == 0 {
		return nil
	}
	for _, artifact := range artifacts {
		payload, err := json.Marshal(artifact.Payload)
		if err != nil {
			return fmt.Errorf("marshal artifact %s: %w", artifact.Kind, err)
		}
		record := memory.WorkflowArtifactRecord{
			ArtifactID:      firstNonEmpty(strings.TrimSpace(artifact.ID), strings.ReplaceAll(string(artifact.Kind), ".", "_")),
			WorkflowID:      strings.TrimSpace(workflowID),
			RunID:           strings.TrimSpace(runID),
			Kind:            string(artifact.Kind),
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     artifact.Summary,
			SummaryMetadata: cloneMap(artifact.Metadata),
			InlineRawText:   string(payload),
			RawSizeBytes:    int64(len(payload)),
		}
		if err := store.UpsertWorkflowArtifact(ctx, record); err != nil {
			return fmt.Errorf("persist artifact %s: %w", artifact.Kind, err)
		}
	}
	return nil
}

func LoadPersistedArtifacts(ctx context.Context, store WorkflowArtifactReader, workflowID, runID string) ([]Artifact, error) {
	if store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	records, err := store.ListWorkflowArtifacts(ctx, strings.TrimSpace(workflowID), strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(records))
	for _, record := range records {
		payload := decodeArtifactPayload(record.InlineRawText)
		out = append(out, Artifact{
			ID:       strings.TrimSpace(record.ArtifactID),
			Kind:     ArtifactKind(strings.TrimSpace(record.Kind)),
			Summary:  strings.TrimSpace(record.SummaryText),
			Metadata: cloneMap(record.SummaryMetadata),
			Payload:  payload,
		})
	}
	return out, nil
}

func RestoreStateFromArtifacts(state *core.Context, artifacts []Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	for _, artifact := range artifacts {
		key := stateKeyForArtifactKind(artifact.Kind)
		if key == "" {
			continue
		}
		state.Set(key, artifact.Payload)
	}
	state.Set("euclo.artifacts", append([]Artifact{}, artifacts...))
}

func AssembleFinalReport(artifacts []Artifact) map[string]any {
	report := map[string]any{
		"artifacts": len(artifacts),
	}
	if len(artifacts) == 0 {
		return report
	}
	order := []ArtifactKind{
		ArtifactKindIntake,
		ArtifactKindClassification,
		ArtifactKindModeResolution,
		ArtifactKindExecutionProfile,
		ArtifactKindRetrievalPolicy,
		ArtifactKindContextExpansion,
		ArtifactKindCapabilityRouting,
		ArtifactKindVerificationPolicy,
		ArtifactKindActionLog,
		ArtifactKindProofSurface,
		ArtifactKindEditIntent,
		ArtifactKindEditExecution,
		ArtifactKindVerification,
		ArtifactKindSuccessGate,
		ArtifactKindFinalReport,
	}
	for _, kind := range order {
		if artifact, ok := firstArtifactOfKind(artifacts, kind); ok {
			switch kind {
			case ArtifactKindIntake:
				report["task"] = artifact.Payload
			case ArtifactKindClassification:
				report["classification"] = artifact.Payload
			case ArtifactKindModeResolution:
				report["mode"] = artifact.Payload
			case ArtifactKindExecutionProfile:
				report["execution_profile"] = artifact.Payload
			case ArtifactKindRetrievalPolicy:
				report["retrieval_policy"] = artifact.Payload
			case ArtifactKindContextExpansion:
				report["context_expansion"] = artifact.Payload
			case ArtifactKindCapabilityRouting:
				report["capability_routing"] = artifact.Payload
			case ArtifactKindVerificationPolicy:
				report["verification_policy"] = artifact.Payload
			case ArtifactKindActionLog:
				report["action_log"] = artifact.Payload
			case ArtifactKindProofSurface:
				report["proof_surface"] = artifact.Payload
			case ArtifactKindEditIntent:
				report["edit_intent"] = artifact.Payload
			case ArtifactKindEditExecution:
				report["edit_execution"] = artifact.Payload
			case ArtifactKindVerification:
				report["verification"] = artifact.Payload
			case ArtifactKindSuccessGate:
				report["success_gate"] = artifact.Payload
			case ArtifactKindFinalReport:
				if payload, ok := artifact.Payload.(map[string]any); ok {
					for key, value := range payload {
						report[key] = value
					}
				}
			}
		}
	}
	summaries := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, map[string]any{
			"kind":    artifact.Kind,
			"summary": artifact.Summary,
		})
	}
	report["artifact_summaries"] = summaries
	return report
}

func normalizeWorkflowRetrieval(raw any) (any, map[string]any) {
	switch typed := raw.(type) {
	case map[string]any:
		metadata := map[string]any{}
		for _, key := range []string{"query", "scope", "cache_tier", "query_id", "citation_count", "result_size"} {
			if value, ok := typed[key]; ok && value != nil {
				metadata[key] = value
			}
		}
		return cloneMap(typed), metadata
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}, nil
		}
		return map[string]any{"summary": text, "texts": []string{text}}, map[string]any{"result_size": 1}
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return map[string]any{}, nil
		}
		return map[string]any{"summary": text}, nil
	}
}

func normalizeEditIntent(raw any) (any, map[string]any) {
	payload := raw
	metadata := map[string]any{"intent_only": true}
	switch typed := raw.(type) {
	case map[string]any:
		payload = cloneMap(typed)
		if value, ok := typed["intent_only"]; ok {
			metadata["intent_only"] = value
		}
	}
	return payload, metadata
}

func artifactSummary(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"summary", "text", "status", "strategy"} {
			if text := strings.TrimSpace(fmt.Sprint(typed[key])); text != "" && text != "<nil>" {
				return text
			}
		}
		if texts, ok := typed["texts"].([]string); ok && len(texts) > 0 {
			return strings.TrimSpace(texts[0])
		}
	}
	if marshaled, err := json.Marshal(raw); err == nil {
		text := string(marshaled)
		if len(text) > 240 {
			return text[:240]
		}
		return text
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = input[key]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decodeArtifactPayload(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return raw
}

func stateKeyForArtifactKind(kind ArtifactKind) string {
	switch kind {
	case ArtifactKindIntake:
		return "euclo.envelope"
	case ArtifactKindClassification:
		return "euclo.classification"
	case ArtifactKindModeResolution:
		return "euclo.mode_resolution"
	case ArtifactKindExecutionProfile:
		return "euclo.execution_profile_selection"
	case ArtifactKindRetrievalPolicy:
		return "euclo.retrieval_policy"
	case ArtifactKindContextExpansion:
		return "euclo.context_expansion"
	case ArtifactKindCapabilityRouting:
		return "euclo.capability_family_routing"
	case ArtifactKindVerificationPolicy:
		return "euclo.verification_policy"
	case ArtifactKindSuccessGate:
		return "euclo.success_gate"
	case ArtifactKindActionLog:
		return "euclo.action_log"
	case ArtifactKindProofSurface:
		return "euclo.proof_surface"
	case ArtifactKindWorkflowRetrieval:
		return "pipeline.workflow_retrieval"
	case ArtifactKindExplore:
		return "pipeline.explore"
	case ArtifactKindAnalyze:
		return "pipeline.analyze"
	case ArtifactKindPlan:
		return "pipeline.plan"
	case ArtifactKindEditIntent:
		return "pipeline.code"
	case ArtifactKindEditExecution:
		return "euclo.edit_execution"
	case ArtifactKindVerification:
		return "pipeline.verify"
	case ArtifactKindFinalReport:
		return "pipeline.final_output"
	default:
		return ""
	}
}

func firstArtifactOfKind(artifacts []Artifact, kind ArtifactKind) (Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact, true
		}
	}
	return Artifact{}, false
}
