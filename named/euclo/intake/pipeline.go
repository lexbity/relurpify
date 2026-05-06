package intake

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// StreamTrigger captures the subset of contextstream.Trigger behavior required by the intake node.
type StreamTrigger interface {
	RequestBlocking(ctx context.Context, req contextstream.Request) (*contextstream.Result, error)
	RequestBackground(ctx context.Context, req contextstream.Request) (*contextstream.Job, error)
}

// Tier2Classifier performs capability sequencing for the winning family.
type Tier2Classifier interface {
	Classify(ctx context.Context, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error)
}

// IntakePipelineNode performs the full intake pipeline: normalize, tier-1 scoring,
// context stream seeding, and tier-2 capability selection.
type IntakePipelineNode struct {
	id                string
	registry          *families.KeywordFamilyRegistry
	maxStreamTokens   int
	defaultStreamMode contextstream.Mode
	streamTrigger     StreamTrigger
	classifier        Tier2Classifier
}

// NewIntakePipelineNode creates a new intake pipeline node.
func NewIntakePipelineNode(id string, registry *families.KeywordFamilyRegistry, maxStreamTokens int, defaultStreamMode contextstream.Mode, trigger StreamTrigger, classifier Tier2Classifier) *IntakePipelineNode {
	return &IntakePipelineNode{
		id:                id,
		registry:          registry,
		maxStreamTokens:   maxStreamTokens,
		defaultStreamMode: defaultStreamMode,
		streamTrigger:     trigger,
		classifier:        classifier,
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

// Execute performs the intake pipeline as per the phase-4 specification.
func (n *IntakePipelineNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if env == nil {
		return nil, fmt.Errorf("intake pipeline %q requires an envelope", n.id)
	}

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

	scoredClassification := ClassifyTaskScored(taskEnvelope, n.registry, nil)

	family, _ := n.lookupFamily(scoredClassification.WinningFamily)
	streamResult := n.maybeStreamContext(ctx, family.RetrievalTemplate, taskEnvelope)
	streamedContext := serializeStreamResult(streamResult)

	intent := ResolveIntent(scoredClassification, taskEnvelope, n.registry, nil, "tier1")
	intent.ClassificationSource = "tier1"
	capabilitySequence := append([]string(nil), intent.CapabilitySequence...)
	capabilityOperator := intent.CapabilityOperator

	if n.classifier != nil {
		seq, op, classifyErr := n.classifier.Classify(ctx, taskEnvelope.Instruction, scoredClassification.WinningFamily, streamedContext, taskEnvelope.NegativeConstraintSeeds)
		if classifyErr == nil && len(seq) > 0 {
			capabilitySequence = append([]string(nil), seq...)
			capabilityOperator = normalizeCapabilityOperator(op, len(seq))
			intent.ClassificationSource = "tier1+tier2"
		}
	}

	if len(capabilitySequence) == 0 {
		capabilitySequence = append([]string(nil), scoredClassification.WinningFamily)
		if family.FallbackCapability != "" {
			capabilitySequence = []string{family.FallbackCapability}
		}
	}
	if capabilityOperator == "" {
		capabilityOperator = defaultCapabilityOperator(capabilitySequence)
	}

	intent.CapabilitySequence = capabilitySequence
	intent.CapabilityOperator = capabilityOperator
	intent.NegativeConstraints = append([]string(nil), taskEnvelope.NegativeConstraintSeeds...)

	familySelection := map[string]any{
		"winning_family": scoredClassification.WinningFamily,
		"confidence":     scoredClassification.Confidence,
		"ambiguous":      scoredClassification.Ambiguous,
	}

	env.SetWorkingValue("euclo.task_envelope", taskEnvelope, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.intent_classification", intent, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.family_selection", familySelection, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.capability_sequence", capabilitySequence, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.negative_constraints", taskEnvelope.NegativeConstraintSeeds, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.capability_operator", capabilityOperator, contextdata.MemoryClassTask)
	if streamResult != nil {
		env.SetWorkingValue("euclo.stream_result", streamResult, contextdata.MemoryClassTask)
	}

	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"winning_family":    scoredClassification.WinningFamily,
			"confidence":        scoredClassification.Confidence,
			"ambiguous":         scoredClassification.Ambiguous,
			"capability_count":  len(capabilitySequence),
			"has_stream_result": streamResult != nil,
			"stream_mode":       string(n.effectiveStreamMode()),
		},
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
	if n.classifier != nil && mode == contextstream.ModeBackground {
		return contextstream.ModeBlocking
	}
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

func taskFromEnvelope(env *contextdata.Envelope) (*core.Task, error) {
	for _, key := range []string{"task.input", "euclo.task.input", "euclo.task"} {
		if value, ok := env.GetWorkingValue(key); ok {
			task, ok := value.(*core.Task)
			if !ok {
				return nil, fmt.Errorf("%s is not *core.Task", key)
			}
			return task, nil
		}
	}
	return nil, fmt.Errorf("no task input in envelope")
}

func serializeStreamResult(result *contextstream.Result) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	if result.Request.ID != "" {
		b.WriteString("request:")
		b.WriteString(result.Request.ID)
		b.WriteString(" ")
	}
	if result.Err != nil {
		b.WriteString("error:")
		b.WriteString(result.Err.Error())
		b.WriteString(" ")
	}
	if result.Compilation != nil {
		b.WriteString("chunks:")
		b.WriteString(fmt.Sprintf("%d", len(result.Compilation.Chunks)))
		b.WriteString(" tokens:")
		b.WriteString(fmt.Sprintf("%d", result.Compilation.TotalTokens))
	}
	return strings.TrimSpace(b.String())
}

func normalizeCapabilityOperator(op string, seqLen int) string {
	switch strings.ToUpper(strings.TrimSpace(op)) {
	case "AND":
		return "AND"
	case "OR":
		return "OR"
	}
	if seqLen > 1 {
		return "AND"
	}
	return "OR"
}

func defaultCapabilityOperator(seq []string) string {
	if len(seq) > 1 {
		return "AND"
	}
	return "OR"
}
