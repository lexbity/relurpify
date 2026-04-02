package archaeographqlserver

import "testing"

func TestScenario_GraphQL_Query_WorkflowProjection(t *testing.T) {
	TestHandlerQueryWorkflowProjectionUsesGraphQLEngine(t)
}

func TestScenario_GraphQL_Mutation_ResolveLearningInteraction(t *testing.T) {
	TestHandlerMutationResolveLearningInteractionUsesRuntime(t)
}

func TestScenario_GraphQL_Query_DecisionTrail(t *testing.T) {
	TestHandlerWorkspaceDecisionTrailQueryUsesCurrentArchaeoRecords(t)
}

func TestScenario_GraphQL_Mutation_ClaimRequest(t *testing.T) {
	TestHandlerMutationClaimRequestUsesRequestLifecycle(t)
}

func TestScenario_GraphQL_Subscription_WorkflowProjection(t *testing.T) {
	TestHandlerSubscriptionWorkflowProjectionUsesSchemaSubscribe(t)
}
