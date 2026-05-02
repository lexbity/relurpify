package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/capabilities"
)

// CapabilityExecutionNode executes a selected capability through the framework registry.
type CapabilityExecutionNode struct {
	id       string
	registry *capability.CapabilityRegistry
}

// NewCapabilityExecutionNode creates a new capability execution node.
func NewCapabilityExecutionNode(id string) *CapabilityExecutionNode {
	return &CapabilityExecutionNode{
		id: id,
	}
}

// WithCapabilityRegistry sets the registry used to invoke capabilities.
func (n *CapabilityExecutionNode) WithCapabilityRegistry(reg *capability.CapabilityRegistry) *CapabilityExecutionNode {
	if n != nil && reg != nil {
		n.registry = reg
	}
	return n
}

// ID implements agentgraph.Node.
func (n *CapabilityExecutionNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *CapabilityExecutionNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute invokes the selected capability and persists execution metadata.
func (n *CapabilityExecutionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	capabilityID := "euclo:cap.ast_query"
	if env != nil {
		if v, ok := env.GetWorkingValue("euclo.route_selection"); ok {
			if selection, ok := v.(*RouteSelection); ok && selection != nil {
				if strings.TrimSpace(selection.CapabilityID) != "" {
					capabilityID = strings.TrimSpace(selection.CapabilityID)
				}
			}
		}
		if v, ok := env.GetWorkingValue("euclo.route.capability_id"); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				capabilityID = strings.TrimSpace(s)
			}
		}
	}

	var (
		result *core.Result
		err    error
	)
	if n.registry == nil {
		result, err = capabilities.InvokeCapability(ctx, capabilityID, nil, env, nil)
	} else {
		result, err = capabilities.InvokeCapability(ctx, capabilityID, nil, env, n.registry)
	}
	if env != nil {
		env.SetWorkingValue("euclo.execution.kind", "capability", contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.execution.capability_id", capabilityID, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.execution.completed", result != nil && result.Success, contextdata.MemoryClassTask)
	}
	if result == nil {
		result = &core.Result{NodeID: n.id, Success: err == nil, Data: map[string]any{}}
	}
	result.NodeID = n.id
	return result, err
}
