package route

import (
	"strings"

	"codeburg.org/lexbit/relurpify/named/rex/classify"
	"codeburg.org/lexbit/relurpify/named/rex/envelope"
)

const (
	FamilyReAct     = "react"
	FamilyPlanner   = "planner"
	FamilyArchitect = "architect"
	FamilyPipeline  = "pipeline"
)

// RouteDecision is rex's durable orchestration decision.
type RouteDecision struct {
	Family             string
	Mode               string
	Profile            string
	RequirePersistence bool
	RequireProof       bool
	RequireRetrieval   bool
	Fallbacks          []string
}

// ExecutionPlan converts a route decision into execution requirements.
type ExecutionPlan struct {
	PrimaryFamily       string
	Fallbacks           []string
	RequirePersistence  bool
	RequireRetrieval    bool
	RequireVerification bool
}

// Decide selects a primary orchestration family from task classification.
func Decide(env envelope.Envelope, class classify.Classification) RouteDecision {
	if strings.TrimSpace(env.ResumedRoute) != "" {
		return decisionForFamily(env.ResumedRoute, class)
	}
	switch {
	case class.DeterministicPreferred:
		return decisionForFamily(FamilyPipeline, class)
	case class.Intent == "planning" && !class.MutationCapable:
		return decisionForFamily(FamilyPlanner, class)
	case class.MutationCapable || class.RecoveryHeavy:
		return decisionForFamily(FamilyArchitect, class)
	default:
		return decisionForFamily(FamilyReAct, class)
	}
}

// BuildExecutionPlan turns a route decision into execution policy.
func BuildExecutionPlan(decision RouteDecision) ExecutionPlan {
	return ExecutionPlan{
		PrimaryFamily:       decision.Family,
		Fallbacks:           append([]string{}, decision.Fallbacks...),
		RequirePersistence:  decision.RequirePersistence,
		RequireRetrieval:    decision.RequireRetrieval,
		RequireVerification: decision.RequireProof,
	}
}

func decisionForFamily(family string, class classify.Classification) RouteDecision {
	switch family {
	case FamilyPipeline:
		return RouteDecision{Family: FamilyPipeline, Mode: "structured", Profile: "nexus-managed", RequirePersistence: true, RequireProof: true, RequireRetrieval: true, Fallbacks: []string{FamilyArchitect, FamilyReAct}}
	case FamilyPlanner:
		return RouteDecision{Family: FamilyPlanner, Mode: "planning", Profile: "read-only", RequirePersistence: false, RequireProof: true, RequireRetrieval: true, Fallbacks: []string{FamilyReAct}}
	case FamilyArchitect:
		return RouteDecision{Family: FamilyArchitect, Mode: "mutation", Profile: "managed", RequirePersistence: true, RequireProof: true, RequireRetrieval: true, Fallbacks: []string{FamilyPipeline, FamilyReAct}}
	default:
		return RouteDecision{Family: FamilyReAct, Mode: "open", Profile: "managed", RequirePersistence: class.RecoveryHeavy || class.LongRunningManaged, RequireProof: true, RequireRetrieval: class.RecoveryHeavy || class.LongRunningManaged, Fallbacks: []string{FamilyPlanner}}
	}
}
