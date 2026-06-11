package intake

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// StreamTrigger captures the subset of contextstream.Trigger behavior required by the intake node.
type StreamTrigger interface {
	RequestBlocking(ctx context.Context, req contextstream.Request) (*contextstream.Result, error)
	RequestBackground(ctx context.Context, req contextstream.Request) (*contextstream.Job, error)
}

// IntakePipelineNode coordinates normalization, evidence extraction,
// interpretation, stream seeding, and clarification gating.
type IntakePipelineNode struct {
	id                string
	registry          *families.KeywordFamilyRegistry
	maxStreamTokens   int
	defaultStreamMode contextstream.Mode
	streamTrigger     StreamTrigger
}

// NewIntakePipelineNode creates a new intake pipeline node.
func NewIntakePipelineNode(id string, registry *families.KeywordFamilyRegistry, maxStreamTokens int, defaultStreamMode contextstream.Mode, trigger StreamTrigger) *IntakePipelineNode {
	return &IntakePipelineNode{
		id:                id,
		registry:          registry,
		maxStreamTokens:   maxStreamTokens,
		defaultStreamMode: defaultStreamMode,
		streamTrigger:     trigger,
	}
}

// ID returns the node ID.
func (n *IntakePipelineNode) ID() string { return n.id }

// Type returns the node type.
func (n *IntakePipelineNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Contract returns the node contract.
func (n *IntakePipelineNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectNone,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
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
	interpretation := BuildIntentInterpretation(intentEvidence, scoredClassification)
	taskEnvelope.Interpretation = interpretation

	family, _ := n.lookupFamily(scoredClassification.WinningFamily)
	streamResult := n.maybeStreamContext(ctx, family.RetrievalTemplate, taskEnvelope)
	intent := &IntentClassification{
		WinningFamily:        scoredClassification.WinningFamily,
		FamilyCandidates:     append([]families.FamilyCandidate(nil), scoredClassification.FamilyCandidates...),
		Confidence:           scoredClassification.Confidence,
		Ambiguous:            scoredClassification.Ambiguous,
		Signals:              append([]ClassificationSignal(nil), scoredClassification.Signals...),
		NegativeConstraints:  append([]string(nil), taskEnvelope.NegativeConstraintSeeds...),
		ClassificationSource: "deterministic",
		MixedIntent:          len(scoredClassification.FamilyCandidates) > 1,
		ReasonCodes:          generateReasonCodes(scoredClassification, taskEnvelope, "deterministic"),
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
		env.SetWorkingValueWithClass(euclokeys.KeyStreamResult, streamResult, contextdata.MemoryClassTask)
	}

	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"winning_family":         scoredClassification.WinningFamily,
			"confidence":             scoredClassification.Confidence,
			"ambiguous":              scoredClassification.Ambiguous,
			"has_stream_result":      streamResult != nil,
			"stream_result":          streamResult,
			"stream_mode":            string(n.effectiveStreamMode()),
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

func (n *IntakePipelineNode) effectiveStreamMode() contextstream.Mode {
	mode := n.defaultStreamMode
	if mode == "" {
		return contextstream.ModeBlocking
	}
	return mode
}

func (n *IntakePipelineNode) maybeStreamContext(ctx context.Context, templateStr string, envelope *TaskEnvelope) *contextstream.Result {
	if n == nil || n.streamTrigger == nil || strings.TrimSpace(templateStr) == "" {
		return nil
	}

	mode := n.effectiveStreamMode()
	req := BuildStreamRequestWithTemplate(templateStr, envelope.Instruction, envelope, n.maxStreamTokens, mode)
	if req == nil {
		return nil
	}
	req.ID = "stream-req-" + n.id
	req.Metadata = map[string]any{
		"intake_node_id":  n.id,
		"family_template": templateStr,
	}

	if mode == contextstream.ModeBackground {
		job, err := n.streamTrigger.RequestBackground(ctx, *req)
		if err != nil {
			return &contextstream.Result{Request: *req, Err: err}
		}
		result, waitErr := job.Wait(ctx)
		if waitErr != nil {
			return &contextstream.Result{Request: *req, Err: waitErr}
		}
		return result
	}

	result, err := n.streamTrigger.RequestBlocking(ctx, *req)
	if err != nil {
		return &contextstream.Result{Request: *req, Err: err}
	}
	return result
}

func taskFromEnvelope(env *contextdata.Envelope) (*execution.Task, error) {
	for _, key := range []string{euclokeys.KeyTaskInputLegacy, euclokeys.KeyTaskInput, euclokeys.KeyTaskRaw} {
		if value, ok := env.GetWorkingValue(key); ok {
			task, ok := value.(*execution.Task)
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
