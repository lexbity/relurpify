package relurpicabilities

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type WorkContext struct {
	PrimaryCapabilityID             string
	SupportingRelurpicCapabilityIDs []string
	PatternRefs                     []string
	TensionRefs                     []string
	ProspectiveRefs                 []string
	ConvergenceRefs                 []string
	RequestProvenanceRefs           []string
}

type RoutineInput struct {
	Task          *core.Task
	State         *core.Context
	Work          WorkContext
	Environment   agentenv.AgentEnvironment
	ServiceBundle any
}

type SupportingRoutine interface {
	ID() string
	Execute(context.Context, RoutineInput) ([]euclotypes.Artifact, error)
}
