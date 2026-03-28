package archaeographqlserver

import (
	"context"
	"time"

	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
)

func (r *mutationResolver) ResolveLearningInteraction(ctx context.Context, args struct {
	Input struct {
		WorkflowID      string
		InteractionID   string
		ExpectedStatus  *string
		Kind            string
		ChoiceID        *string
		RefinedPayload  *Map
		ResolvedBy      *string
		BasedOnRevision *string
	}
}) (*Map, error) {
	input := archaeolearning.ResolveInput{
		WorkflowID:    args.Input.WorkflowID,
		InteractionID: args.Input.InteractionID,
		Kind:          archaeolearning.ResolutionKind(args.Input.Kind),
	}
	if args.Input.ExpectedStatus != nil {
		input.ExpectedStatus = archaeolearning.InteractionStatus(*args.Input.ExpectedStatus)
	}
	if args.Input.ChoiceID != nil {
		input.ChoiceID = *args.Input.ChoiceID
	}
	if args.Input.RefinedPayload != nil {
		input.RefinedPayload = cloneMapAny(*args.Input.RefinedPayload)
	}
	if args.Input.ResolvedBy != nil {
		input.ResolvedBy = *args.Input.ResolvedBy
	}
	if args.Input.BasedOnRevision != nil {
		input.BasedOnRevision = *args.Input.BasedOnRevision
	}
	value, err := r.runtime.ResolveLearningInteraction(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) UpdateTensionStatus(ctx context.Context, args struct {
	Input struct {
		WorkflowID  string
		TensionID   string
		Status      string
		CommentRefs *[]string
	}
}) (*Map, error) {
	value, err := r.runtime.UpdateTensionStatus(ctx, args.Input.WorkflowID, args.Input.TensionID, archaeodomain.TensionStatus(args.Input.Status), idSlice(args.Input.CommentRefs))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ActivatePlanVersion(ctx context.Context, args struct {
	WorkflowID string
	Version    int32
}) (*Map, error) {
	value, err := r.runtime.ActivatePlanVersion(ctx, args.WorkflowID, int(args.Version))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ArchivePlanVersion(ctx context.Context, args struct {
	WorkflowID string
	Version    int32
	Reason     string
}) (*Map, error) {
	value, err := r.runtime.ArchivePlanVersion(ctx, args.WorkflowID, int(args.Version), args.Reason)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) MarkPlanVersionStale(ctx context.Context, args struct {
	WorkflowID string
	Version    int32
	Reason     string
}) (*Map, error) {
	value, err := r.runtime.MarkPlanVersionStale(ctx, args.WorkflowID, int(args.Version), args.Reason)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) MarkExplorationStale(ctx context.Context, args struct {
	ExplorationID string
	Reason        string
}) (*Map, error) {
	value, err := r.runtime.MarkExplorationStale(ctx, args.ExplorationID, args.Reason)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) PrepareLivingPlan(ctx context.Context, args struct {
	Input struct {
		WorkflowID          string
		WorkspaceID         string
		Instruction         *string
		CorpusScope         *string
		SymbolScope         *string
		BasedOnRevision     *string
		SemanticSnapshotRef *string
	}
}) (*Map, error) {
	input := PrepareLivingPlanInput{
		WorkflowID:  args.Input.WorkflowID,
		WorkspaceID: args.Input.WorkspaceID,
	}
	if args.Input.Instruction != nil {
		input.Instruction = *args.Input.Instruction
	}
	if args.Input.CorpusScope != nil {
		input.CorpusScope = *args.Input.CorpusScope
	}
	if args.Input.SymbolScope != nil {
		input.SymbolScope = *args.Input.SymbolScope
	}
	if args.Input.BasedOnRevision != nil {
		input.BasedOnRevision = *args.Input.BasedOnRevision
	}
	if args.Input.SemanticSnapshotRef != nil {
		input.SemanticSnapshotRef = *args.Input.SemanticSnapshotRef
	}
	value, err := r.runtime.PrepareLivingPlan(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) RefreshExplorationSnapshot(ctx context.Context, args struct {
	Input struct {
		WorkflowID           string
		SnapshotID           string
		BasedOnRevision      *string
		SemanticSnapshotRef  *string
		CandidatePatternRefs *[]string
		CandidateAnchorRefs  *[]string
		TensionIDs           *[]string
		OpenLearningIDs      *[]string
		Summary              *string
	}
}) (*Map, error) {
	input := RefreshExplorationSnapshotInput{
		WorkflowID: args.Input.WorkflowID,
		SnapshotID: args.Input.SnapshotID,
	}
	if args.Input.BasedOnRevision != nil {
		input.BasedOnRevision = *args.Input.BasedOnRevision
	}
	if args.Input.SemanticSnapshotRef != nil {
		input.SemanticSnapshotRef = *args.Input.SemanticSnapshotRef
	}
	input.CandidatePatternRefs = idSlice(args.Input.CandidatePatternRefs)
	input.CandidateAnchorRefs = idSlice(args.Input.CandidateAnchorRefs)
	input.TensionIDs = idSlice(args.Input.TensionIDs)
	input.OpenLearningIDs = idSlice(args.Input.OpenLearningIDs)
	if args.Input.Summary != nil {
		input.Summary = *args.Input.Summary
	}
	value, err := r.runtime.RefreshExplorationSnapshot(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) CreateOrUpdateDeferredDraft(ctx context.Context, args struct {
	Input struct {
		WorkspaceID        string
		WorkflowID         string
		ExplorationID      *string
		PlanID             *string
		PlanVersion        *int32
		RequestID          *string
		AmbiguityKey       string
		Title              *string
		Description        *string
		LinkedDraftVersion *int32
		LinkedDraftPlanID  *string
		CommentRefs        *[]string
		Metadata           *Map
	}
}) (*Map, error) {
	input := archaeodeferred.CreateInput{
		WorkspaceID:  args.Input.WorkspaceID,
		WorkflowID:   args.Input.WorkflowID,
		AmbiguityKey: args.Input.AmbiguityKey,
		CommentRefs:  idSlice(args.Input.CommentRefs),
		Metadata:     cloneMapAnyPtr(args.Input.Metadata),
	}
	if args.Input.ExplorationID != nil {
		input.ExplorationID = *args.Input.ExplorationID
	}
	if args.Input.PlanID != nil {
		input.PlanID = *args.Input.PlanID
	}
	input.PlanVersion = intPtr32(args.Input.PlanVersion)
	if args.Input.RequestID != nil {
		input.RequestID = *args.Input.RequestID
	}
	if args.Input.Title != nil {
		input.Title = *args.Input.Title
	}
	if args.Input.Description != nil {
		input.Description = *args.Input.Description
	}
	input.LinkedDraftVersion = intPtr32(args.Input.LinkedDraftVersion)
	if args.Input.LinkedDraftPlanID != nil {
		input.LinkedDraftPlanID = *args.Input.LinkedDraftPlanID
	}
	value, err := r.runtime.CreateOrUpdateDeferredDraft(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) FinalizeDeferredDraft(ctx context.Context, args struct {
	Input struct {
		WorkflowID  string
		RecordID    string
		CommentRefs *[]string
		Metadata    *Map
	}
}) (*Map, error) {
	value, err := r.runtime.FinalizeDeferredDraft(ctx, archaeodeferred.FinalizeInput{
		WorkflowID:  args.Input.WorkflowID,
		RecordID:    args.Input.RecordID,
		CommentRefs: idSlice(args.Input.CommentRefs),
		Metadata:    cloneMapAnyPtr(args.Input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) CreateConvergenceRecord(ctx context.Context, args struct {
	Input struct {
		WorkspaceID        string
		WorkflowID         string
		ExplorationID      *string
		PlanID             *string
		PlanVersion        *int32
		Question           *string
		Title              *string
		RelevantTensionIDs *[]string
		PendingLearningIDs *[]string
		AcceptedDebt       *[]string
		DeferredDraftIDs   *[]string
		ProvenanceRefs     *[]string
		CommentRefs        *[]string
		Metadata           *Map
	}
}) (*Map, error) {
	input := archaeoconvergence.CreateInput{
		WorkspaceID:        args.Input.WorkspaceID,
		WorkflowID:         args.Input.WorkflowID,
		RelevantTensionIDs: idSlice(args.Input.RelevantTensionIDs),
		PendingLearningIDs: idSlice(args.Input.PendingLearningIDs),
		DeferredDraftIDs:   idSlice(args.Input.DeferredDraftIDs),
		ProvenanceRefs:     idSlice(args.Input.ProvenanceRefs),
		CommentRefs:        idSlice(args.Input.CommentRefs),
		Metadata:           cloneMapAnyPtr(args.Input.Metadata),
	}
	if args.Input.ExplorationID != nil {
		input.ExplorationID = *args.Input.ExplorationID
	}
	if args.Input.PlanID != nil {
		input.PlanID = *args.Input.PlanID
	}
	input.PlanVersion = intPtr32(args.Input.PlanVersion)
	if args.Input.Question != nil {
		input.Question = *args.Input.Question
	}
	if args.Input.Title != nil {
		input.Title = *args.Input.Title
	}
	if args.Input.AcceptedDebt != nil {
		input.AcceptedDebt = append([]string(nil), (*args.Input.AcceptedDebt)...)
	}
	value, err := r.runtime.CreateConvergenceRecord(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ResolveConvergenceRecord(ctx context.Context, args struct {
	Input struct {
		WorkflowID string
		RecordID   string
		Resolution struct {
			Status         string
			AcceptedDebt   *[]string
			DeferredIssues *[]string
			ChosenOption   *string
			Summary        *string
			CommentRefs    *[]string
			Metadata       *Map
		}
	}
}) (*Map, error) {
	resolution := archaeodomain.ConvergenceResolution{
		Status:      archaeodomain.ConvergenceResolutionStatus(args.Input.Resolution.Status),
		CommentRefs: idSlice(args.Input.Resolution.CommentRefs),
		Metadata:    cloneMapAnyPtr(args.Input.Resolution.Metadata),
	}
	if args.Input.Resolution.AcceptedDebt != nil {
		resolution.AcceptedDebt = append([]string(nil), (*args.Input.Resolution.AcceptedDebt)...)
	}
	if args.Input.Resolution.DeferredIssues != nil {
		resolution.DeferredIssues = append([]string(nil), (*args.Input.Resolution.DeferredIssues)...)
	}
	if args.Input.Resolution.ChosenOption != nil {
		resolution.ChosenOption = *args.Input.Resolution.ChosenOption
	}
	if args.Input.Resolution.Summary != nil {
		resolution.Summary = *args.Input.Resolution.Summary
	}
	resolution.ResolvedAt = timePtr(time.Now().UTC())
	value, err := r.runtime.ResolveConvergenceRecord(ctx, archaeoconvergence.ResolveInput{
		WorkflowID: args.Input.WorkflowID,
		RecordID:   args.Input.RecordID,
		Resolution: resolution,
	})
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) CreateDecisionRecord(ctx context.Context, args struct {
	Input struct {
		WorkspaceID            string
		WorkflowID             string
		Kind                   string
		RelatedRequestID       *string
		RelatedConvergenceID   *string
		RelatedDeferredDraftID *string
		RelatedPlanID          *string
		RelatedPlanVersion     *int32
		Validity               *string
		Title                  string
		Summary                string
		CommentRefs            *[]string
		Metadata               *Map
	}
}) (*Map, error) {
	input := archaeodecisions.CreateInput{
		WorkspaceID: args.Input.WorkspaceID,
		WorkflowID:  args.Input.WorkflowID,
		Kind:        archaeodomain.DecisionKind(args.Input.Kind),
		Title:       args.Input.Title,
		Summary:     args.Input.Summary,
		CommentRefs: idSlice(args.Input.CommentRefs),
		Metadata:    cloneMapAnyPtr(args.Input.Metadata),
	}
	if args.Input.RelatedRequestID != nil {
		input.RelatedRequestID = *args.Input.RelatedRequestID
	}
	if args.Input.RelatedConvergenceID != nil {
		input.RelatedConvergenceID = *args.Input.RelatedConvergenceID
	}
	if args.Input.RelatedDeferredDraftID != nil {
		input.RelatedDeferredDraftID = *args.Input.RelatedDeferredDraftID
	}
	if args.Input.RelatedPlanID != nil {
		input.RelatedPlanID = *args.Input.RelatedPlanID
	}
	input.RelatedPlanVersion = intPtr32(args.Input.RelatedPlanVersion)
	if args.Input.Validity != nil {
		input.Validity = archaeodomain.RequestValidity(*args.Input.Validity)
	}
	value, err := r.runtime.CreateDecisionRecord(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ResolveDecisionRecord(ctx context.Context, args struct {
	Input struct {
		WorkflowID  string
		RecordID    string
		Status      string
		CommentRefs *[]string
		Metadata    *Map
	}
}) (*Map, error) {
	value, err := r.runtime.ResolveDecisionRecord(ctx, archaeodecisions.ResolveInput{
		WorkflowID:  args.Input.WorkflowID,
		RecordID:    args.Input.RecordID,
		Status:      archaeodomain.DecisionStatus(args.Input.Status),
		CommentRefs: idSlice(args.Input.CommentRefs),
		Metadata:    cloneMapAnyPtr(args.Input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) DispatchRequest(ctx context.Context, args struct {
	WorkflowID string
	RequestID  string
	Metadata   *Map
}) (*Map, error) {
	value, err := r.runtime.DispatchRequest(ctx, args.WorkflowID, args.RequestID, cloneMapAnyPtr(args.Metadata))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ClaimRequest(ctx context.Context, args struct {
	Input struct {
		WorkflowID   string
		RequestID    string
		ClaimedBy    string
		LeaseSeconds *int32
		Metadata     *Map
	}
}) (*Map, error) {
	value, err := r.runtime.ClaimRequest(ctx, archaeorequests.ClaimInput{
		WorkflowID: args.Input.WorkflowID,
		RequestID:  args.Input.RequestID,
		ClaimedBy:  args.Input.ClaimedBy,
		LeaseTTL:   seconds(args.Input.LeaseSeconds),
		Metadata:   cloneMapAnyPtr(args.Input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) RenewRequestClaim(ctx context.Context, args struct {
	Input struct {
		WorkflowID   string
		RequestID    string
		LeaseSeconds *int32
		Metadata     *Map
	}
}) (*Map, error) {
	value, err := r.runtime.RenewRequestClaim(ctx, archaeorequests.RenewInput{
		WorkflowID: args.Input.WorkflowID,
		RequestID:  args.Input.RequestID,
		LeaseTTL:   seconds(args.Input.LeaseSeconds),
		Metadata:   cloneMapAnyPtr(args.Input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ReleaseRequestClaim(ctx context.Context, args struct {
	WorkflowID string
	RequestID  string
}) (*Map, error) {
	value, err := r.runtime.ReleaseRequestClaim(ctx, args.WorkflowID, args.RequestID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) ApplyRequestFulfillment(ctx context.Context, args struct {
	Input struct {
		WorkflowID  string
		RequestID   string
		Fulfillment struct {
			Kind           string
			RefID          *string
			Summary        *string
			Metadata       *Map
			ExecutorRef    *string
			SessionRef     *string
			RejectedReason *string
		}
		CurrentRevision   *string
		CurrentSnapshotID *string
		ConflictingRefIDs *[]string
	}
}) (*Map, error) {
	fulfillment := archaeodomain.RequestFulfillment{
		Kind:     args.Input.Fulfillment.Kind,
		Metadata: cloneMapAnyPtr(args.Input.Fulfillment.Metadata),
	}
	if args.Input.Fulfillment.RefID != nil {
		fulfillment.RefID = *args.Input.Fulfillment.RefID
	}
	if args.Input.Fulfillment.Summary != nil {
		fulfillment.Summary = *args.Input.Fulfillment.Summary
	}
	if args.Input.Fulfillment.ExecutorRef != nil {
		fulfillment.ExecutorRef = *args.Input.Fulfillment.ExecutorRef
	}
	if args.Input.Fulfillment.SessionRef != nil {
		fulfillment.SessionRef = *args.Input.Fulfillment.SessionRef
	}
	if args.Input.Fulfillment.RejectedReason != nil {
		fulfillment.RejectedReason = *args.Input.Fulfillment.RejectedReason
	}
	input := archaeorequests.ApplyFulfillmentInput{
		WorkflowID:        args.Input.WorkflowID,
		RequestID:         args.Input.RequestID,
		Fulfillment:       fulfillment,
		ConflictingRefIDs: idSlice(args.Input.ConflictingRefIDs),
	}
	if args.Input.CurrentRevision != nil {
		input.CurrentRevision = *args.Input.CurrentRevision
	}
	if args.Input.CurrentSnapshotID != nil {
		input.CurrentSnapshotID = *args.Input.CurrentSnapshotID
	}
	value, err := r.runtime.ApplyRequestFulfillment(ctx, input)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) FailRequest(ctx context.Context, args struct {
	WorkflowID string
	RequestID  string
	ErrorText  string
	Retry      bool
}) (*Map, error) {
	value, err := r.runtime.FailRequest(ctx, args.WorkflowID, args.RequestID, args.ErrorText, args.Retry)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) InvalidateRequest(ctx context.Context, args struct {
	WorkflowID        string
	RequestID         string
	Reason            string
	ConflictingRefIDs *[]string
}) (*Map, error) {
	value, err := r.runtime.InvalidateRequest(ctx, args.WorkflowID, args.RequestID, args.Reason, idSlice(args.ConflictingRefIDs))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *mutationResolver) SupersedeRequest(ctx context.Context, args struct {
	WorkflowID  string
	RequestID   string
	SuccessorID string
	Reason      string
}) (*Map, error) {
	value, err := r.runtime.SupersedeRequest(ctx, args.WorkflowID, args.RequestID, args.SuccessorID, args.Reason)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func seconds(v *int32) time.Duration {
	if v == nil || *v <= 0 {
		return 0
	}
	return time.Duration(*v) * time.Second
}

func idSlice(ids *[]string) []string {
	if ids == nil {
		return nil
	}
	out := make([]string, 0, len(*ids))
	for _, id := range *ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func intPtr32(value *int32) *int {
	if value == nil {
		return nil
	}
	copy := int(*value)
	return &copy
}

func cloneMapAnyPtr(m *Map) map[string]any {
	if m == nil {
		return nil
	}
	return cloneMapAny(*m)
}

func cloneMapAny(m Map) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

func timePtr(v time.Time) *time.Time {
	return &v
}
