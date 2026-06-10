package policy

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	govpolicy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	policyDecisionKey      = "euclo.policy_decision"
	policyMutationKey      = "euclo.policy.mutation_permitted"
	policyHITLRequiredKey  = "euclo.policy.hitl_required"
	policyVerificationKey  = "euclo.policy.verification_required"
	policyReasonCodesKey   = "euclo.policy.reason_codes"
	policyHITLTriggeredKey = "euclo.hitl_triggered"
	policyHITLResponseKey  = "euclo.hitl_response"
)

// PermissionManager captures the subset of authorization.PermissionManager
// behavior required by GateNode.
type PermissionManager interface {
	RequireApproval(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, justification string, scope govpolicy.GrantScope, risk govpolicy.RiskLevel, duration time.Duration) error
}

// HITLBroker captures the subset of authorization.HITLBroker behavior required
// by GateNode.
type HITLBroker interface {
	RequestPermission(ctx context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error)
}

// GateNode enforces policy decisions before allowing execution to proceed.
type GateNode struct {
	id                string
	evaluator         *Evaluator
	permissionManager PermissionManager
	hitlBroker        HITLBroker
	agentID           string
	approvalTimeout   time.Duration
	eventLog          telemetry.Telemetry
}

// NewGateNode creates a new gate node.
func NewGateNode(id string, evaluator *Evaluator) *GateNode {
	return &GateNode{
		id:              id,
		evaluator:       evaluator,
		approvalTimeout: 5 * time.Minute,
	}
}

// WithPermissionManager wires the gate to a permission-manager fallback.
func (n *GateNode) WithPermissionManager(manager PermissionManager) *GateNode {
	n.permissionManager = manager
	return n
}

// WithHITLBroker wires the gate to a HITL broker.
func (n *GateNode) WithHITLBroker(broker HITLBroker) *GateNode {
	n.hitlBroker = broker
	return n
}

// WithAgentID sets the agent identifier used in approval descriptors.
func (n *GateNode) WithAgentID(agentID string) *GateNode {
	n.agentID = strings.TrimSpace(agentID)
	return n
}

// WithApprovalTimeout sets the timeout used for approval requests.
func (n *GateNode) WithApprovalTimeout(timeout time.Duration) *GateNode {
	if timeout > 0 {
		n.approvalTimeout = timeout
	}
	return n
}

// WithTelemetry wires a telemetry sink for frame emission.
func (n *GateNode) WithTelemetry(telemetry telemetry.Telemetry) *GateNode {
	n.eventLog = telemetry
	return n
}

// ID returns the node ID.
func (n *GateNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *GateNode) Type() string {
	return "gate"
}

// Execute performs policy enforcement.
func (n *GateNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	decision := n.readDecision(env)
	if decision == nil {
		return nil, fmt.Errorf("gate %q could not determine policy decision", n.id)
	}

	n.writeDecision(env, decision)

	switch {
	case decision.HITLRequired:
		response, err := n.handleHITL(ctx, env, decision)
		if err != nil {
			n.writeHITLState(env, false, nil)
			contextdata.SetTyped(env, "euclo.outcome.category", "hitl_rejected")
			contextdata.SetTyped(env, "euclo.outcome.reason", err.Error())
			contextdata.SetTyped(env, "euclo.outcome.completed", false)
			return nil, err
		}
		n.writeHITLState(env, true, response)
		decision.MutationPermitted = true
		n.writeDecision(env, decision)
		return n.resultFromDecision(decision, "ask_approved"), nil
	case decision.MutationPermitted:
		n.writeHITLState(env, false, nil)
		return n.resultFromDecision(decision, "allow"), nil
	default:
		n.writeHITLState(env, false, nil)
		return nil, fmt.Errorf("policy denied: %s", n.reasonString(decision))
	}
}

func (n *GateNode) readDecision(env *contextdata.Envelope) *PolicyDecision {
	if decision, ok := contextdata.GetTyped[*PolicyDecision](env, policyDecisionKey); ok && decision != nil {
		return decision
	}
	if n == nil || n.evaluator == nil {
		return nil
	}
	return n.evaluator.Evaluate(n.policyContextFromEnvelope(env))
}

func (n *GateNode) policyContextFromEnvelope(env *contextdata.Envelope) *PolicyContext {
	return &PolicyContext{
		FamilyID:             getString(env, "euclo.family_selection"),
		EditPermitted:        getBool(env, "euclo.task_envelope.edit_permitted"),
		RequiresVerification: getBool(env, policyVerificationKey),
		RiskLevel:            cmp.Or(getString(env, "euclo.policy.risk_level"), "low"),
		WorkspaceScopes:      getStringSlice(env, "euclo.workspace_scopes"),
	}
}

func (n *GateNode) writeDecision(env *contextdata.Envelope, decision *PolicyDecision) {
	if decision == nil {
		return
	}
	contextdata.SetTyped(env, policyDecisionKey, decision)
	contextdata.SetTyped(env, policyMutationKey, decision.MutationPermitted)
	contextdata.SetTyped(env, policyHITLRequiredKey, decision.HITLRequired)
	contextdata.SetTyped(env, policyVerificationKey, decision.VerificationRequired)
	contextdata.SetTyped(env, policyReasonCodesKey, append([]string(nil), decision.ReasonCodes...))
}

func (n *GateNode) writeHITLState(env *contextdata.Envelope, triggered bool, response *interaction.HITLResponse) {
	contextdata.SetTyped(env, policyHITLTriggeredKey, triggered)
	contextdata.SetTyped(env, policyHITLResponseKey, response)
}

func (n *GateNode) handleHITL(ctx context.Context, env *contextdata.Envelope, decision *PolicyDecision) (*interaction.HITLResponse, error) {
	if decision == nil {
		return nil, fmt.Errorf("gate %q missing policy decision", n.id)
	}

	frame := interaction.NewHITLApprovalFrame(env.TaskID, env.SessionID, "euclo.policy.gate", n.reasonString(decision))
	if err := interaction.EmitFrame(ctx, frame, env, n.telemetry()); err != nil {
		return nil, err
	}

	if n.hitlBroker != nil {
		req := authorization.PermissionRequest{
			Permission: permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeHITL,
				Action:       "euclo.policy.gate",
				Resource:     n.resourceID(env),
				RequiresHITL: true,
			},
			Justification:   n.reasonString(decision),
			Scope:           govpolicy.GrantScopeOneTime,
			Risk:            n.riskLevel(decision),
			Duration:        0,
			Timeout:         n.approvalTimeout,
			TimeoutBehavior: authorization.HITLTimeoutBehaviorFail,
		}
		grant, err := n.hitlBroker.RequestPermission(ctx, req)
		if err != nil {
			return nil, err
		}
		if grant != nil {
			extra := make(map[string]any, len(grant.Conditions))
			for k, v := range grant.Conditions {
				extra[k] = v
			}
			return &interaction.HITLResponse{
				ChosenSlot:  "approve",
				ExtraData:   extra,
				RespondedBy: grant.ApprovedBy,
				RespondedAt: grant.GrantedAt,
			}, nil
		}
		return &interaction.HITLResponse{ChosenSlot: "approve"}, nil
	}

	if n.permissionManager != nil {
		desc := permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeHITL,
			Action:       "euclo.policy.gate",
			Resource:     n.resourceID(env),
			RequiresHITL: true,
		}
		if err := n.permissionManager.RequireApproval(ctx, n.agentID, desc, n.reasonString(decision), govpolicy.GrantScopeOneTime, n.riskLevel(decision), n.approvalTimeout); err != nil {
			return nil, err
		}
		return &interaction.HITLResponse{ChosenSlot: "approve"}, nil
	}

	return nil, fmt.Errorf("gate %q has no hitl broker or permission manager", n.id)
}

func (n *GateNode) resultFromDecision(decision *PolicyDecision, mode string) map[string]any {
	return map[string]any{
		"decision":              mode,
		"mutation_permitted":    decision.MutationPermitted,
		"hitl_required":         decision.HITLRequired,
		"verification_required": decision.VerificationRequired,
		"reason_codes":          append([]string(nil), decision.ReasonCodes...),
	}
}

func (n *GateNode) reasonString(decision *PolicyDecision) string {
	if decision == nil || len(decision.ReasonCodes) == 0 {
		return "policy gate"
	}
	return strings.Join(decision.ReasonCodes, ", ")
}

func (n *GateNode) riskLevel(decision *PolicyDecision) govpolicy.RiskLevel {
	if decision != nil && decision.HITLRequired {
		return govpolicy.RiskLevelMedium
	}
	return govpolicy.RiskLevelLow
}

func (n *GateNode) resourceID(env *contextdata.Envelope) string {
	if id := strings.TrimSpace(env.TaskID); id != "" {
		return id
	}
	return strings.TrimSpace(n.agentID)
}

func (n *GateNode) telemetry() telemetry.Telemetry {
	if n != nil && n.eventLog != nil {
		return n.eventLog
	}
	return nil
}

func getString(env *contextdata.Envelope, key string) string {
	v, _ := contextdata.GetTyped[string](env, key)
	return strings.TrimSpace(v)
}

func getBool(env *contextdata.Envelope, key string) bool {
	v, _ := contextdata.GetTyped[bool](env, key)
	return v
}

func getStringSlice(env *contextdata.Envelope, key string) []string {
	v, ok := contextdata.GetTyped[[]string](env, key)
	if !ok {
		return nil
	}
	return append([]string(nil), v...)
}
