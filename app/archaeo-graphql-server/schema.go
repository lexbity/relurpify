package archaeographqlserver

const SchemaSDL = `
scalar Map

type Query {
  activeExploration(workspaceId: String!): Map
  explorationView(explorationId: String!): Map
  explorationByWorkflow(workflowId: String!): Map

  workflowProjection(workflowId: String!): Map
  timeline(workflowId: String!): Map
  mutationHistory(workflowId: String!): Map
  requestHistory(workflowId: String!): Map
  provenance(workflowId: String!): Map
  coherence(workflowId: String!): Map
  learningQueue(workflowId: String!): [Map!]!
  tensions(workflowId: String!): [Map!]!
  tensionSummary(workflowId: String!): Map
  activePlanVersion(workflowId: String!): Map
  planLineage(workflowId: String!): Map
  comparePlanVersions(workflowId: String!, left: Int!, right: Int!): Map

  deferredDrafts(workspaceId: String!, limit: Int): Map
  currentConvergence(workspaceId: String!): Map
  convergenceHistory(workspaceId: String!, limit: Int): Map
  decisionTrail(workspaceId: String!, limit: Int): Map
  workspaceSummary(workspaceId: String!): Map

  pendingRequests(workflowId: String!): [Map!]!
  request(workflowId: String!, requestId: String!): Map
}

type Mutation {
  resolveLearningInteraction(input: ResolveLearningInteractionInput!): Map
  updateTensionStatus(input: UpdateTensionStatusInput!): Map
  activatePlanVersion(workflowId: String!, version: Int!): Map
  archivePlanVersion(workflowId: String!, version: Int!, reason: String!): Map
  markPlanVersionStale(workflowId: String!, version: Int!, reason: String!): Map
  markExplorationStale(explorationId: String!, reason: String!): Map
  prepareLivingPlan(input: PrepareLivingPlanInput!): Map
  refreshExplorationSnapshot(input: RefreshExplorationSnapshotInput!): Map

  createOrUpdateDeferredDraft(input: CreateOrUpdateDeferredDraftInput!): Map
  finalizeDeferredDraft(input: FinalizeDeferredDraftInput!): Map
  createConvergenceRecord(input: CreateConvergenceRecordInput!): Map
  resolveConvergenceRecord(input: ResolveConvergenceRecordInput!): Map
  createDecisionRecord(input: CreateDecisionRecordInput!): Map
  resolveDecisionRecord(input: ResolveDecisionRecordInput!): Map

  dispatchRequest(workflowId: String!, requestId: String!, metadata: Map): Map
  claimRequest(input: ClaimRequestInput!): Map
  renewRequestClaim(input: RenewRequestClaimInput!): Map
  releaseRequestClaim(workflowId: String!, requestId: String!): Map
  applyRequestFulfillment(input: ApplyRequestFulfillmentInput!): Map
  failRequest(workflowId: String!, requestId: String!, errorText: String!, retry: Boolean!): Map
  invalidateRequest(workflowId: String!, requestId: String!, reason: String!, conflictingRefIds: [String!]): Map
  supersedeRequest(workflowId: String!, requestId: String!, successorId: String!, reason: String!): Map
}

type Subscription {
  workflowProjectionUpdated(workflowId: String!): Map
  timelineUpdated(workflowId: String!): Map
  requestHistoryUpdated(workflowId: String!): Map
  learningQueueUpdated(workflowId: String!): [Map!]!
  tensionsUpdated(workflowId: String!): [Map!]!
  tensionSummaryUpdated(workflowId: String!): Map
  activePlanVersionUpdated(workflowId: String!): Map
  planLineageUpdated(workflowId: String!): Map
  provenanceUpdated(workflowId: String!): Map
  coherenceUpdated(workflowId: String!): Map
  deferredDraftsUpdated(workspaceId: String!, limit: Int): Map
  currentConvergenceUpdated(workspaceId: String!): Map
  convergenceHistoryUpdated(workspaceId: String!, limit: Int): Map
  decisionTrailUpdated(workspaceId: String!, limit: Int): Map
}

input ResolveLearningInteractionInput {
  workflowId: String!
  interactionId: String!
  expectedStatus: String
  kind: String!
  choiceId: String
  refinedPayload: Map
  resolvedBy: String
  basedOnRevision: String
}

input UpdateTensionStatusInput {
  workflowId: String!
  tensionId: String!
  status: String!
  commentRefs: [String!]
}

input PrepareLivingPlanInput {
  workflowId: String!
  workspaceId: String!
  instruction: String
  corpusScope: String
  symbolScope: String
  basedOnRevision: String
  semanticSnapshotRef: String
}

input RefreshExplorationSnapshotInput {
  workflowId: String!
  snapshotId: String!
  basedOnRevision: String
  semanticSnapshotRef: String
  candidatePatternRefs: [String!]
  candidateAnchorRefs: [String!]
  tensionIds: [String!]
  openLearningIds: [String!]
  summary: String
}

input CreateOrUpdateDeferredDraftInput {
  workspaceId: String!
  workflowId: String!
  explorationId: String
  planId: String
  planVersion: Int
  requestId: String
  ambiguityKey: String!
  title: String
  description: String
  linkedDraftVersion: Int
  linkedDraftPlanId: String
  commentRefs: [String!]
  metadata: Map
}

input FinalizeDeferredDraftInput {
  workflowId: String!
  recordId: String!
  commentRefs: [String!]
  metadata: Map
}

input CreateConvergenceRecordInput {
  workspaceId: String!
  workflowId: String!
  explorationId: String
  planId: String
  planVersion: Int
  question: String
  title: String
  relevantTensionIds: [String!]
  pendingLearningIds: [String!]
  acceptedDebt: [String!]
  deferredDraftIds: [String!]
  provenanceRefs: [String!]
  commentRefs: [String!]
  metadata: Map
}

input ConvergenceResolutionInput {
  status: String!
  acceptedDebt: [String!]
  deferredIssues: [String!]
  chosenOption: String
  summary: String
  commentRefs: [String!]
  metadata: Map
}

input ResolveConvergenceRecordInput {
  workflowId: String!
  recordId: String!
  resolution: ConvergenceResolutionInput!
}

input CreateDecisionRecordInput {
  workspaceId: String!
  workflowId: String!
  kind: String!
  relatedRequestId: String
  relatedConvergenceId: String
  relatedDeferredDraftId: String
  relatedPlanId: String
  relatedPlanVersion: Int
  validity: String
  title: String!
  summary: String!
  commentRefs: [String!]
  metadata: Map
}

input ResolveDecisionRecordInput {
  workflowId: String!
  recordId: String!
  status: String!
  commentRefs: [String!]
  metadata: Map
}

input ClaimRequestInput {
  workflowId: String!
  requestId: String!
  claimedBy: String!
  leaseSeconds: Int
  metadata: Map
}

input RenewRequestClaimInput {
  workflowId: String!
  requestId: String!
  leaseSeconds: Int
  metadata: Map
}

input RequestFulfillmentInput {
  kind: String!
  refId: String
  summary: String
  metadata: Map
  executorRef: String
  sessionRef: String
  rejectedReason: String
}

input ApplyRequestFulfillmentInput {
  workflowId: String!
  requestId: String!
  fulfillment: RequestFulfillmentInput!
  currentRevision: String
  currentSnapshotId: String
  conflictingRefIds: [String!]
}
`
