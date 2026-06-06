package agentgraph

import (
	"context"
	"testing"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"github.com/stretchr/testify/require"
)

type internalContractNode struct {
	id   string
	kind NodeType
}

func (n internalContractNode) ID() string     { return n.id }
func (n internalContractNode) Type() NodeType { return n.kind }
func (n internalContractNode) Execute(context.Context, *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func TestValidateNodeContractRejectsInvalidPlacement(t *testing.T) {
	err := validateNodeContract(internalContractNode{id: "n1", kind: NodeTypeSystem}, NodeContract{
		SideEffectClass:    SideEffectNone,
		Idempotency:        IdempotencyReplaySafe,
		PreferredPlacement: PlacementPreference("invalid"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid placement preference")
}

func TestValidateNodeContractAllowsBoundaryValues(t *testing.T) {
	err := validateNodeContract(internalContractNode{id: "n1", kind: NodeTypeSystem}, NodeContract{
		SideEffectClass:  SideEffectNone,
		Idempotency:      IdempotencyReplaySafe,
		Recoverability:   NodeRecoverabilityPersisted,
		CheckpointPolicy: CheckpointPolicyRequired,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       0,
			MaxInlineCollectionItems: 0,
		},
	})
	require.NoError(t, err)
}
