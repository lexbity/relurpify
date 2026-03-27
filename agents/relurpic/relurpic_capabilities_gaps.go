package relurpic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type gapDetectorDetectCapabilityHandler struct {
	model         core.LanguageModel
	config        *core.Config
	registry      *capability.Registry
	indexManager  *ast.IndexManager
	graphDB       *graphdb.Engine
	retrievalDB   *sql.DB
	planStore     frameworkplan.PlanStore
	guidance      *guidance.GuidanceBroker
	workflowStore memory.WorkflowStateStore
}

func (h gapDetectorDetectCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:gap-detector.detect",
		"gap-detector.detect",
		"Compare declared intent anchors against implementation and surface intent gaps.",
		core.CapabilityKindTool,
		core.CoordinationRoleDomainPack,
		[]string{"analyze", "gap-detect"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredObjectSchema(map[string]*core.Schema{
			"file_path":               {Type: "string"},
			"corpus_scope":            {Type: "string"},
			"anchor_ids":              {Type: "array", Items: &core.Schema{Type: "string"}},
			"workflow_id":             {Type: "string"},
			"exploration_id":          {Type: "string"},
			"exploration_snapshot_id": {Type: "string"},
			"based_on_revision":       {Type: "string"},
		}, "file_path", "corpus_scope"),
		structuredObjectSchema(map[string]*core.Schema{
			"gaps": {
				Type:  "array",
				Items: &core.Schema{Type: "object"},
			},
			"count":                {Type: "integer"},
			"anchor_count_checked": {Type: "integer"},
		}, "gaps", "count", "anchor_count_checked"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "gap-detect",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h gapDetectorDetectCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	filePath := stringArg(args["file_path"])
	if filePath == "" {
		return nil, fmt.Errorf("file_path required")
	}
	corpusScope := stringArg(args["corpus_scope"])
	if corpusScope == "" {
		return nil, fmt.Errorf("corpus_scope required")
	}

	scope, err := resolveSymbolScope(ctx, filePath, h.indexManager, h.registry)
	if err != nil {
		return nil, err
	}
	contextExcerpts := scope.Excerpts
	if h.graphDB != nil {
		contextExcerpts = append(contextExcerpts, h.neighborExcerpts(scope)...)
		contextExcerpts = dedupeResolvedExcerpts(contextExcerpts)
	}

	anchors, err := h.loadAnchors(ctx, corpusScope, stringSliceArg(args["anchor_ids"]))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	gaps, err := h.detectGaps(ctx, scope, contextExcerpts, corpusScope, anchors, now)
	if err != nil {
		return nil, err
	}

	workflowID := stringArg(args["workflow_id"])
	explorationID := stringArg(args["exploration_id"])
	snapshotID := stringArg(args["exploration_snapshot_id"])
	basedOnRevision := stringArg(args["based_on_revision"])
	for _, gap := range gaps {
		invalidatedStepIDs, err := invalidatePlanStepsForAnchor(ctx, h.planStore, workflowID, gap.AnchorID)
		if err != nil {
			return nil, err
		}
		if h.retrievalDB != nil && gap.AnchorID != "" {
			if err := retrieval.RecordAnchorDrift(ctx, h.retrievalDB, gap.AnchorID, string(gap.Severity), gap.Description); err != nil {
				return nil, err
			}
		}
		if h.graphDB != nil && gap.SymbolID != "" && gap.AnchorID != "" {
			props := map[string]any{
				"severity":       gap.Severity,
				"description":    gap.Description,
				"evidence_lines": gap.EvidenceLines,
				"corpus_scope":   gap.CorpusScope,
			}
			kind := ast.EdgeKindDriftsFrom
			weight := float32(0.5)
			if gap.Severity == patterns.GapSeverityCritical || gap.Severity == patterns.GapSeveritySignificant {
				kind = ast.EdgeKindViolatesContract
				weight = 1
			}
			if err := h.graphDB.Link(gap.SymbolID, gap.AnchorID, kind, "", weight, props); err != nil {
				return nil, err
			}
			h.maybeEscalateGap(gap)
		}
		if err := h.persistTension(ctx, workflowID, explorationID, snapshotID, basedOnRevision, gap, invalidatedStepIDs); err != nil {
			return nil, err
		}
	}

	result := &core.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"gaps":                 gapsAsAny(gaps),
			"count":                len(gaps),
			"anchor_count_checked": len(anchors),
		},
		Metadata: map[string]any{},
	}
	chain := core.OriginDerivation("relurpic.gap-detector")
	chain = chain.Derive("load_scope", "relurpic.gap-detector", 0.0, filePath)
	chain = chain.Derive("anchor_lookup", "relurpic.gap-detector", 0.02, corpusScope)
	chain = chain.Derive("llm_analysis", "relurpic.gap-detector", llmLossMagnitude(gaps), fmt.Sprintf("%d gaps", len(gaps)))
	desc := h.Descriptor(ctx, nil)
	envelope := core.NewCapabilityResultEnvelope(desc, result, core.ContentDispositionRaw, nil, nil)
	envelope.Provenance.Derivation = &chain
	result.Metadata["capability_result_envelope"] = envelope
	result.Metadata["derivation_total_loss"] = chain.TotalLoss()
	return result, nil
}

func (h gapDetectorDetectCapabilityHandler) loadAnchors(ctx context.Context, corpusScope string, anchorIDs []string) ([]retrieval.AnchorRecord, error) {
	if h.retrievalDB == nil {
		return nil, nil
	}
	records, err := retrieval.ActiveAnchors(ctx, h.retrievalDB, corpusScope)
	if err != nil {
		return nil, err
	}
	if len(anchorIDs) == 0 {
		return records, nil
	}
	allowed := make(map[string]struct{}, len(anchorIDs))
	for _, id := range anchorIDs {
		allowed[id] = struct{}{}
	}
	filtered := make([]retrieval.AnchorRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[record.AnchorID]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

func (h gapDetectorDetectCapabilityHandler) detectGaps(ctx context.Context, scope resolvedSymbolScope, excerpts []resolvedExcerpt, corpusScope string, anchors []retrieval.AnchorRecord, now time.Time) ([]patterns.IntentGap, error) {
	if len(anchors) == 0 {
		return nil, nil
	}
	gaps := make([]patterns.IntentGap, 0)
	for start := 0; start < len(anchors); start += 5 {
		end := start + 5
		if end > len(anchors) {
			end = len(anchors)
		}
		batch := anchors[start:end]
		resp, err := h.model.Generate(ctx, buildGapDetectionPrompt(batch, excerpts), &core.LLMOptions{
			Model:       modelName(h.config),
			Temperature: 0.1,
			MaxTokens:   1400,
		})
		if err != nil {
			return nil, err
		}
		analyses, err := parseGapDetectorResponse(resp.Text)
		if err != nil {
			return nil, err
		}
		for idx, analysis := range analyses {
			if idx >= len(batch) || analysis == nil {
				continue
			}
			anchor := batch[idx]
			severity := normalizeGapSeverity(analysis.Severity)
			gap := patterns.IntentGap{
				GapID:          gapID(anchor.AnchorID, analysis.Description, idx),
				AnchorID:       anchor.AnchorID,
				AnchorTerm:     anchor.Term,
				FilePath:       scope.PrimaryFile(),
				SymbolID:       firstSymbolID(scope),
				Description:    strings.TrimSpace(analysis.Description),
				Severity:       severity,
				EvidenceLines:  append([]int(nil), analysis.EvidenceLines...),
				CorpusScope:    corpusScope,
				DetectionRunID: fmt.Sprintf("gap-run-%d", now.UnixNano()),
				CreatedAt:      now,
			}
			if gap.Description == "" {
				continue
			}
			gaps = append(gaps, gap)
		}
	}
	return gaps, nil
}

func (h gapDetectorDetectCapabilityHandler) neighborExcerpts(scope resolvedSymbolScope) []resolvedExcerpt {
	if h.graphDB == nil || len(scope.SymbolIDs) == 0 {
		return nil
	}
	impact := h.graphDB.ImpactSet(scope.SymbolIDs, []graphdb.EdgeKind{ast.EdgeKindCalls, ast.EdgeKindImports}, 1)
	out := make([]resolvedExcerpt, 0, len(impact.Affected))
	for _, id := range impact.Affected {
		node, ok := h.graphDB.GetNode(id)
		if !ok || node.SourceID == "" {
			continue
		}
		var props graphNodeProps
		if len(node.Props) > 0 {
			if err := json.Unmarshal(node.Props, &props); err != nil {
				continue
			}
		}
		excerpt, err := excerptForLines(context.Background(), h.registry, node.SourceID, props.StartLine, props.EndLine)
		if err != nil {
			continue
		}
		out = append(out, excerpt)
	}
	return out
}

func (h gapDetectorDetectCapabilityHandler) maybeEscalateGap(gap patterns.IntentGap) {
	if h.guidance == nil || h.graphDB == nil || gap.SymbolID == "" {
		return
	}
	impact := h.graphDB.ImpactSet([]string{gap.SymbolID}, []graphdb.EdgeKind{ast.EdgeKindViolatesContract, ast.EdgeKindDriftsFrom, ast.EdgeKindCalledBy}, 3)
	score := severityScore(gap.Severity) * len(impact.Affected)
	if score <= 6 {
		h.guidance.AddObservation(guidance.EngineeringObservation{
			Source:       gap.GapID,
			GuidanceKind: guidance.GuidanceContradiction,
			Title:        fmt.Sprintf("Behavioral contradiction for %s", gap.AnchorTerm),
			Description:  gap.Description,
			Evidence: map[string]any{
				"anchor_id":      gap.AnchorID,
				"symbol_id":      gap.SymbolID,
				"severity":       gap.Severity,
				"affected_nodes": impact.Affected,
			},
			BlastRadius: len(impact.Affected),
		})
		return
	}
	_, _ = h.guidance.SubmitAsync(guidance.GuidanceRequest{
		Kind:        guidance.GuidanceContradiction,
		Title:       fmt.Sprintf("Behavioral contradiction for %s", gap.AnchorTerm),
		Description: gap.Description,
		Context: map[string]any{
			"anchor_id":      gap.AnchorID,
			"symbol_id":      gap.SymbolID,
			"severity":       gap.Severity,
			"blast_radius":   len(impact.Affected),
			"affected_nodes": impact.Affected,
		},
		Choices: []guidance.GuidanceChoice{
			{ID: "review", Label: "Review now", IsDefault: true},
			{ID: "defer", Label: "Defer"},
		},
		TimeoutBehavior: guidance.GuidanceTimeoutDefer,
		Timeout:         30 * time.Second,
	})
}

type gapAnalysis struct {
	Severity      string `json:"severity"`
	Description   string `json:"description"`
	EvidenceLines []int  `json:"evidence_lines"`
}

func parseGapDetectorResponse(text string) ([]*gapAnalysis, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "[") {
		var direct []*gapAnalysis
		if err := json.Unmarshal([]byte(trimmed), &direct); err != nil {
			return nil, err
		}
		return direct, nil
	}
	extracted := reactpkg.ExtractJSON(text)
	var payload struct {
		Results []*gapAnalysis `json:"results"`
	}
	if err := json.Unmarshal([]byte(extracted), &payload); err == nil && payload.Results != nil {
		return payload.Results, nil
	}
	var direct []*gapAnalysis
	if err := json.Unmarshal([]byte(extracted), &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

func buildGapDetectionPrompt(anchors []retrieval.AnchorRecord, excerpts []resolvedExcerpt) string {
	var b strings.Builder
	b.WriteString("You are an implementation-intent gap detector.\n")
	b.WriteString("For each anchor, decide if the implementation honors the stated intent.\n")
	b.WriteString("Return valid JSON: {\"results\":[null|{\"severity\":\"critical|significant|minor\",\"description\":\"...\",\"evidence_lines\":[1,2]}]}\n")
	b.WriteString("Implementation context:\n")
	for _, excerpt := range excerpts {
		b.WriteString(fmt.Sprintf("FILE %s [%d-%d]\n%s\n", excerpt.FilePath, excerpt.StartLine, excerpt.EndLine, excerpt.Content))
	}
	b.WriteString("Anchors:\n")
	for i, anchor := range anchors {
		b.WriteString(fmt.Sprintf("%d. term=%s\n", i+1, anchor.Term))
		b.WriteString(fmt.Sprintf("definition=%s\n", anchor.Definition))
	}
	return b.String()
}

func normalizeGapSeverity(raw string) patterns.GapSeverity {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(patterns.GapSeverityCritical):
		return patterns.GapSeverityCritical
	case string(patterns.GapSeveritySignificant):
		return patterns.GapSeveritySignificant
	default:
		return patterns.GapSeverityMinor
	}
}

func llmLossMagnitude(gaps []patterns.IntentGap) float64 {
	highest := patterns.GapSeverityMinor
	if len(gaps) == 0 {
		return 0.02
	}
	for _, gap := range gaps {
		if severityScore(gap.Severity) > severityScore(highest) {
			highest = gap.Severity
		}
	}
	switch highest {
	case patterns.GapSeverityCritical:
		return 0.40
	case patterns.GapSeveritySignificant:
		return 0.20
	default:
		return 0.05
	}
}

func severityScore(severity patterns.GapSeverity) int {
	switch severity {
	case patterns.GapSeverityCritical:
		return 3
	case patterns.GapSeveritySignificant:
		return 2
	default:
		return 1
	}
}

func firstSymbolID(scope resolvedSymbolScope) string {
	if len(scope.SymbolIDs) == 0 {
		return ""
	}
	return scope.SymbolIDs[0]
}

func gapsAsAny(gaps []patterns.IntentGap) []any {
	out := make([]any, 0, len(gaps))
	for _, gap := range gaps {
		out = append(out, map[string]any{
			"gap_id":           gap.GapID,
			"anchor_id":        gap.AnchorID,
			"anchor_term":      gap.AnchorTerm,
			"file_path":        gap.FilePath,
			"symbol_id":        gap.SymbolID,
			"description":      gap.Description,
			"severity":         gap.Severity,
			"evidence_lines":   append([]int(nil), gap.EvidenceLines...),
			"corpus_scope":     gap.CorpusScope,
			"detection_run_id": gap.DetectionRunID,
			"created_at":       gap.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func gapID(anchorID, description string, idx int) string {
	return patternProposalID(anchorID, "gap", description, idx)
}

func dedupeResolvedExcerpts(excerpts []resolvedExcerpt) []resolvedExcerpt {
	seen := make(map[string]struct{}, len(excerpts))
	out := make([]resolvedExcerpt, 0, len(excerpts))
	for _, excerpt := range excerpts {
		key := fmt.Sprintf("%s:%d:%d", excerpt.FilePath, excerpt.StartLine, excerpt.EndLine)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, excerpt)
	}
	return out
}

func stringSliceArg(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]string); ok {
			out := make([]string, 0, len(typed))
			for _, value := range typed {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				out = append(out, value)
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func (h gapDetectorDetectCapabilityHandler) persistTension(ctx context.Context, workflowID, explorationID, snapshotID, basedOnRevision string, gap patterns.IntentGap, relatedStepIDs []string) error {
	if h.workflowStore == nil || strings.TrimSpace(workflowID) == "" {
		return nil
	}
	service := archaeotensions.Service{Store: h.workflowStore}
	status := archaeodomain.TensionInferred
	if gap.Severity == patterns.GapSeverityCritical || gap.Severity == patterns.GapSeveritySignificant {
		status = archaeodomain.TensionUnresolved
	}
	var blastRadius []string
	if h.graphDB != nil && gap.SymbolID != "" {
		impact := h.graphDB.ImpactSet([]string{gap.SymbolID}, []graphdb.EdgeKind{ast.EdgeKindViolatesContract, ast.EdgeKindDriftsFrom, ast.EdgeKindCalledBy}, 3)
		blastRadius = append(blastRadius, impact.Affected...)
	}
	_, err := service.CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:         workflowID,
		ExplorationID:      explorationID,
		SnapshotID:         snapshotID,
		SourceRef:          gap.GapID,
		AnchorRefs:         nonEmptyStrings(gap.AnchorID),
		SymbolScope:        nonEmptyStrings(gap.SymbolID),
		Kind:               "intent_gap",
		Description:        gap.Description,
		Severity:           string(gap.Severity),
		Status:             status,
		BlastRadiusNodeIDs: blastRadius,
		RelatedPlanStepIDs: relatedStepIDs,
		BasedOnRevision:    basedOnRevision,
	})
	return err
}

func invalidatePlanStepsForAnchor(ctx context.Context, store frameworkplan.PlanStore, workflowID, anchorID string) ([]string, error) {
	if store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(anchorID) == "" {
		return nil, nil
	}
	livingPlan, err := store.LoadPlanByWorkflow(ctx, workflowID)
	if err != nil || livingPlan == nil {
		return nil, err
	}
	event := frameworkplan.InvalidationEvent{
		Kind:   frameworkplan.InvalidationAnchorDrifted,
		Target: anchorID,
		At:     time.Now().UTC(),
	}
	invalidated := frameworkplan.PropagateInvalidation(livingPlan, event)
	rule := frameworkplan.InvalidationRule{Kind: frameworkplan.InvalidationAnchorDrifted, Target: anchorID}
	for _, stepID := range invalidated {
		if err := store.InvalidateStep(ctx, livingPlan.ID, stepID, rule); err != nil {
			return nil, err
		}
	}
	return invalidated, nil
}

func nonEmptyStrings(values ...string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
