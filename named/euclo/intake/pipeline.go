package intake

import (
	"context"
	"errors"
	"fmt"
	"strings"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// StreamTrigger captures the subset of contextstream.Trigger behavior required
// by the intake node. Intake is a BLOCKING producer only (D5): the initial
// context must land before the first prompt composes, so background semantics
// — and their request interface surface — do not exist here.
type StreamTrigger interface {
	RequestBlocking(ctx context.Context, req contextstream.Request) (*contextstream.Result, error)
}

// IntakePipelineNode coordinates normalization, evidence extraction,
// interpretation, stream seeding, and clarification gating.
type IntakePipelineNode struct {
	id              string
	registry        *families.KeywordFamilyRegistry
	maxStreamTokens int
	streamTrigger   StreamTrigger
	// tier2Model is the bounded family tie-break model (option A): consulted
	// ONLY when the deterministic classifier marks the classification
	// ambiguous with at least two tied candidates. Nil (or model failure)
	// keeps the deterministic result and its clarification posture.
	tier2Model model.LanguageModel
}

// NewIntakePipelineNode creates a new intake pipeline node.
func NewIntakePipelineNode(id string, registry *families.KeywordFamilyRegistry, maxStreamTokens int, trigger StreamTrigger) *IntakePipelineNode {
	return &IntakePipelineNode{
		id:              id,
		registry:        registry,
		maxStreamTokens: maxStreamTokens,
		streamTrigger:   trigger,
	}
}

// WithTier2Model installs the family tie-break model.
func (n *IntakePipelineNode) WithTier2Model(lm model.LanguageModel) *IntakePipelineNode {
	if n != nil {
		n.tier2Model = lm
	}
	return n
}

// ID returns the node ID.
func (n *IntakePipelineNode) ID() string { return n.id }

// Type returns the node type.
func (n *IntakePipelineNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Contract returns the node contract. The intake compile is a first-class
// context producer (D5): its slice lands on the envelope, so the contract
// declares the context side effect and bounds its working-state entries.
func (n *IntakePipelineNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectContext,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "contextstream.*"},
			WriteKeys:                []string{"contextstream.*"},
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
		},
	}
}

// Execute performs the intake pipeline as a coordinator-only stage.
func (n *IntakePipelineNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	task, err := taskFromEnvelope(env)
	if err != nil {
		return nil, err
	}

	taskEnvelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		return nil, fmt.Errorf("normalize task failed: %w", err)
	}
	if taskEnvelope == nil {
		return nil, fmt.Errorf("normalize task returned nil envelope")
	}
	intentEvidence := taskEnvelope.Evidence
	if intentEvidence == nil {
		intentEvidence = BuildIntentEvidence(taskEnvelope)
		taskEnvelope.Evidence = intentEvidence
	}

	scoredClassification := ClassifyTaskScored(taskEnvelope, n.registry, nil)
	scoredClassification = n.disambiguateFamilyTie(ctx, scoredClassification, taskEnvelope.Instruction)
	interpretation := BuildIntentInterpretation(intentEvidence, scoredClassification)
	taskEnvelope.Interpretation = interpretation

	family, _ := n.lookupFamily(scoredClassification.WinningFamily)
	streamResult, streamErr := n.maybeStreamContext(ctx, env, family.RetrievalTemplate, taskEnvelope)
	if streamErr != nil {
		return nil, streamErr
	}
	classificationSource := "deterministic"
	if scoredClassification.DisambiguationSource != "" {
		classificationSource = scoredClassification.DisambiguationSource
	}
	intent := &IntentClassification{
		WinningFamily:        scoredClassification.WinningFamily,
		FamilyCandidates:     append([]families.FamilyCandidate(nil), scoredClassification.FamilyCandidates...),
		Confidence:           scoredClassification.Confidence,
		Ambiguous:            scoredClassification.Ambiguous,
		Signals:              append([]ClassificationSignal(nil), scoredClassification.Signals...),
		NegativeConstraints:  append([]string(nil), taskEnvelope.NegativeConstraintSeeds...),
		ClassificationSource: classificationSource,
		MixedIntent:          len(scoredClassification.FamilyCandidates) > 1,
		ReasonCodes:          generateReasonCodes(scoredClassification, taskEnvelope, classificationSource),
	}
	if family.ID != "" {
		intent.EditPermitted = family.DefaultHITLPolicy != families.HITLPolicyAlways
		intent.RequiresVerification = family.DefaultVerification == families.VerificationRequired
		intent.RiskLevel = getRiskLevelForFamily(family.ID)
	} else {
		intent.EditPermitted = true
		intent.RiskLevel = "unknown"
	}
	if len(taskEnvelope.WorkspaceScopes) > 0 {
		intent.Scope = strings.TrimSpace(taskEnvelope.WorkspaceScopes[0])
	} else {
		intent.Scope = "workspace"
	}

	familySelection := map[string]any{
		"winning_family": scoredClassification.WinningFamily,
		"confidence":     scoredClassification.Confidence,
		"ambiguous":      scoredClassification.Ambiguous,
		"source":         intent.ClassificationSource,
		"mixed_intent":   intent.MixedIntent,
	}

	env.SetWorkingValueWithClass(euclokeys.KeyTaskEnvelope, taskEnvelope, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclokeys.KeyIntentEvidence, intentEvidence, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(intentcontext.IntentEvidenceKey, intentEvidence, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclokeys.KeyIntentInterpretation, interpretation, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(intentcontext.IntentInterpretationKey, interpretation, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclokeys.KeyIntentClassification, intent, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclokeys.KeyFamilySelection, scoredClassification.WinningFamily, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclokeys.KeyNegativeConstraints, taskEnvelope.NegativeConstraintSeeds, contextdata.MemoryClassTask)
	if streamResult != nil {
		// Bounded summary only (FR-15): the full result's bodies live on the
		// envelope slice where the renderer reads them; working state carries
		// the shared summary shape the stream node writes.
		env.SetWorkingValueWithClass(euclokeys.KeyStreamResult, contextstream.SummaryOf(streamResult), contextdata.MemoryClassTask)
	}

	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"winning_family":         scoredClassification.WinningFamily,
			"confidence":             scoredClassification.Confidence,
			"ambiguous":              scoredClassification.Ambiguous,
			"has_stream_result":      streamResult != nil,
			"stream_result":          contextstream.SummaryOf(streamResult),
			"intent_evidence":        intentEvidence,
			"interpretation":         interpretation,
			"requires_clarification": intentEvidence != nil && intentEvidence.RequiresClarification,
			"missing_fields":         missingFields(intentEvidence),
			"classification_source":  intent.ClassificationSource,
			"family_selection":       familySelection,
		}),
	}, nil
}

func (n *IntakePipelineNode) lookupFamily(familyID string) (families.KeywordFamily, bool) {
	if n == nil || n.registry == nil {
		return families.KeywordFamily{}, false
	}
	return n.registry.Lookup(familyID)
}

// maybeStreamContext performs the intake compile on the BLOCKING path (D5):
// intake must complete before the first prompt composes — background
// semantics were always a lie here, and the immediate Wait proved it. The
// result is a first-class context delivery: it lands on the envelope through
// contextstream.ApplyResult, under the same epoch guard as every other
// compile, so the FIRST model call renders it. A compile error fails the
// node (the trigger-node rule); a stale-dropped apply is the designed drop
// and is reported on the node ctx's telemetry sink.
func (n *IntakePipelineNode) maybeStreamContext(ctx context.Context, env *contextdata.Envelope, templateStr string, envelope *TaskEnvelope) (*contextstream.Result, error) {
	if n == nil || n.streamTrigger == nil || strings.TrimSpace(templateStr) == "" {
		return nil, nil
	}

	req := BuildStreamRequestWithTemplate(templateStr, envelope.Instruction, envelope, n.maxStreamTokens, contextstream.ModeBlocking)
	if req == nil {
		return nil, nil
	}
	req.ID = "stream-req-" + n.id
	// The intake compile is part of the run's memory order: its request
	// carries the envelope's compile identity (epoch + run scope) so the
	// compiler's cache keys it correctly and can never serve it across runs.
	meta := env.AssemblyMetadataSnapshot()
	req.EpochID = meta.EpochID
	req.RunID = meta.RunID
	req.Metadata = map[string]any{
		"intake_node_id":  n.id,
		"family_template": templateStr,
	}

	result, err := n.streamTrigger.RequestBlocking(ctx, *req)
	if err != nil {
		return nil, fmt.Errorf("intake compile: %w", err)
	}
	if err := contextstream.ApplyResult(env, result, req.EpochID); err != nil {
		var dropped *contextstream.ErrStaleApplyDropped
		if !errors.As(err, &dropped) {
			return nil, fmt.Errorf("intake apply: %w", err)
		}
		// Designed drop: a newer compilation already owns the envelope
		// slice. Report it on the node ctx's sink; the node succeeds.
		contextstream.EmitStaleApplyDropped(ctx, dropped.CurrentEpoch, dropped.GotEpoch, dropped.RequestID)
	}
	return result, nil
}

func taskFromEnvelope(env *contextdata.Envelope) (*execution.Task, error) {
	for _, key := range []string{euclokeys.KeyTaskInput, euclokeys.KeyTaskRaw} {
		if task, ok := contextdata.GetTyped[*execution.Task](env, key); ok {
			if !ok {
				return nil, fmt.Errorf("%s is not *execution.Task", key)
			}
			return task, nil
		}
	}
	return nil, fmt.Errorf("no task input in envelope")
}

func missingFields(evidence *intentcontext.IntentEvidence) []string {
	if evidence == nil {
		return nil
	}
	return evidence.MissingFields
}
