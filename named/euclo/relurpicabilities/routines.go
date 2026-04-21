package relurpicabilities

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
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
