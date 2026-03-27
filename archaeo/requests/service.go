package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/internal/keylock"
	"github.com/lexcodex/relurpify/archaeo/internal/storeutil"
	"github.com/lexcodex/relurpify/framework/memory"
)

const requestArtifactKind = "archaeo_request"

type CreateInput struct {
	WorkflowID      string
	ExplorationID   string
	SnapshotID      string
	PlanID          string
	PlanVersion     *int
	Kind            archaeodomain.RequestKind
	Title           string
	Description     string
	RequestedBy     string
	CorrelationID   string
	IdempotencyKey  string
	SubjectRefs     []string
	Input           map[string]any
	BasedOnRevision string
}

type CompleteInput struct {
	WorkflowID string
	RequestID  string
	Result     archaeodomain.RequestResult
}

type ClaimInput struct {
	WorkflowID string
	RequestID  string
	ClaimedBy  string
	LeaseTTL   time.Duration
	Metadata   map[string]any
}

type RenewInput struct {
	WorkflowID string
	RequestID  string
	LeaseTTL   time.Duration
	Metadata   map[string]any
}

type ApplyFulfillmentInput struct {
	WorkflowID        string
	RequestID         string
	Fulfillment       archaeodomain.RequestFulfillment
	CurrentRevision   string
	CurrentSnapshotID string
	ConflictingRefIDs []string
}

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(prefix string) string
}

var requestMutationLocks keylock.Locker

func (s Service) Create(ctx context.Context, input CreateInput) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
	)
	lockKey := "workflow:" + strings.TrimSpace(input.WorkflowID)
	err = requestMutationLocks.With(lockKey, func() error {
		if s.Store == nil {
			return errors.New("workflow state store required")
		}
		if strings.TrimSpace(input.WorkflowID) == "" {
			return errors.New("workflow id required")
		}
		if input.Kind == "" {
			return errors.New("request kind required")
		}
		if strings.TrimSpace(input.Title) == "" {
			return errors.New("request title required")
		}
		if _, ok, err := s.Store.GetWorkflow(ctx, input.WorkflowID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("workflow %s not found", input.WorkflowID)
		}
		now := s.now()
		if existing := s.findExisting(ctx, input); existing != nil {
			record = existing
			return nil
		}
		record = &archaeodomain.RequestRecord{
			ID:              s.newID("request"),
			WorkflowID:      strings.TrimSpace(input.WorkflowID),
			ExplorationID:   strings.TrimSpace(input.ExplorationID),
			SnapshotID:      strings.TrimSpace(input.SnapshotID),
			PlanID:          strings.TrimSpace(input.PlanID),
			PlanVersion:     cloneInt(input.PlanVersion),
			Kind:            input.Kind,
			Status:          archaeodomain.RequestStatusPending,
			Title:           strings.TrimSpace(input.Title),
			Description:     strings.TrimSpace(input.Description),
			RequestedBy:     strings.TrimSpace(input.RequestedBy),
			CorrelationID:   strings.TrimSpace(input.CorrelationID),
			IdempotencyKey:  strings.TrimSpace(input.IdempotencyKey),
			SubjectRefs:     cloneStrings(input.SubjectRefs),
			Input:           cloneMap(input.Input),
			BasedOnRevision: strings.TrimSpace(input.BasedOnRevision),
			RequestedAt:     now,
			UpdatedAt:       now,
		}
		if err := s.save(ctx, record); err != nil {
			return err
		}
		if err := archaeoevents.AppendRequestEvent(ctx, s.Store, *record, archaeoevents.EventRequestCreated, record.Title, nil, now); err != nil {
			return err
		}
		return nil
	})
	return record, err
}

func (s Service) Load(ctx context.Context, workflowID, requestID string) (*archaeodomain.RequestRecord, bool, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, false, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, strings.TrimSpace(requestID)); err != nil {
		return nil, false, err
	} else if ok && artifact != nil {
		var record archaeodomain.RequestRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, false, err
		}
		return &record, true, nil
	}
	records, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, false, err
	}
	for i := range records {
		if records[i].ID == requestID {
			record := records[i]
			return &record, true, nil
		}
	}
	return nil, false, nil
}

func (s Service) ListByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.RequestRecord, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKind(ctx, s.Store, workflowID, "", requestArtifactKind)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.RequestRecord, 0)
	for _, artifact := range artifacts {
		var record archaeodomain.RequestRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RequestedAt.Equal(out[j].RequestedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].RequestedAt.Before(out[j].RequestedAt)
	})
	return out, nil
}

func (s Service) Pending(ctx context.Context, workflowID string) ([]archaeodomain.RequestRecord, error) {
	all, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.RequestRecord, 0)
	for _, record := range all {
		if record.Status == archaeodomain.RequestStatusPending || record.Status == archaeodomain.RequestStatusDispatched || record.Status == archaeodomain.RequestStatusRunning {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s Service) ExpireClaims(ctx context.Context, workflowID string) ([]archaeodomain.RequestRecord, error) {
	var (
		out []archaeodomain.RequestRecord
		err error
	)
	err = requestMutationLocks.With("workflow:"+strings.TrimSpace(workflowID), func() error {
		all, err := s.ListByWorkflow(ctx, workflowID)
		if err != nil {
			return err
		}
		now := s.now()
		out = make([]archaeodomain.RequestRecord, 0)
		for i := range all {
			record := all[i]
			if record.Status != archaeodomain.RequestStatusRunning || record.LeaseExpiresAt == nil || record.LeaseExpiresAt.After(now) {
				continue
			}
			record.ClaimedBy = ""
			record.ClaimedAt = nil
			record.LeaseExpiresAt = nil
			updated, err := s.transitionWithRecord(ctx, &record, archaeodomain.RequestStatusDispatched, map[string]any{"claim_expired": true}, "claim expired", nil, false)
			if err != nil {
				return err
			}
			if updated != nil {
				out = append(out, *updated)
			}
		}
		return nil
	})
	return out, err
}

func (s Service) Claim(ctx context.Context, input ClaimInput) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(input.WorkflowID, input.RequestID), func() error {
		record, ok, err = s.Load(ctx, input.WorkflowID, input.RequestID)
		if err != nil || !ok {
			return err
		}
		if record.Status != archaeodomain.RequestStatusPending && record.Status != archaeodomain.RequestStatusDispatched {
			return nil
		}
		if record.Status == archaeodomain.RequestStatusRunning && strings.TrimSpace(record.ClaimedBy) != "" && strings.TrimSpace(record.ClaimedBy) != strings.TrimSpace(input.ClaimedBy) {
			return nil
		}
		now := s.now()
		record.ClaimedBy = strings.TrimSpace(input.ClaimedBy)
		record.ClaimedAt = &now
		if input.LeaseTTL > 0 {
			expiry := now.Add(input.LeaseTTL)
			record.LeaseExpiresAt = &expiry
		}
		record.Attempt++
		record, err = s.transitionWithRecord(ctx, record, archaeodomain.RequestStatusRunning, input.Metadata, "", nil, false)
		return err
	})
	return record, err
}

func (s Service) Release(ctx context.Context, workflowID, requestID string) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(workflowID, requestID), func() error {
		record, ok, err = s.Load(ctx, workflowID, requestID)
		if err != nil || !ok {
			return err
		}
		record.ClaimedBy = ""
		record.ClaimedAt = nil
		record.LeaseExpiresAt = nil
		record, err = s.transitionWithRecord(ctx, record, archaeodomain.RequestStatusDispatched, nil, "", nil, false)
		return err
	})
	return record, err
}

func (s Service) Renew(ctx context.Context, input RenewInput) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(input.WorkflowID, input.RequestID), func() error {
		record, ok, err = s.Load(ctx, input.WorkflowID, input.RequestID)
		if err != nil || !ok {
			return err
		}
		if record.Status != archaeodomain.RequestStatusRunning {
			return nil
		}
		now := s.now()
		if input.LeaseTTL > 0 {
			expiry := now.Add(input.LeaseTTL)
			record.LeaseExpiresAt = &expiry
		}
		if len(input.Metadata) > 0 {
			record.DispatchMetadata = mergeMap(record.DispatchMetadata, input.Metadata)
		}
		record.UpdatedAt = now
		if err := s.save(ctx, record); err != nil {
			return err
		}
		if err := archaeoevents.AppendRequestEvent(ctx, s.Store, *record, archaeoevents.EventRequestStarted, lifecycleMessage(record), input.Metadata, now); err != nil {
			return err
		}
		return nil
	})
	return record, err
}

func (s Service) Invalidate(ctx context.Context, workflowID, requestID, reason string, conflictingRefs []string) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(workflowID, requestID), func() error {
		record, ok, err = s.Load(ctx, workflowID, requestID)
		if err != nil || !ok {
			return err
		}
		now := s.now()
		record.InvalidatedAt = &now
		record.InvalidationReason = strings.TrimSpace(reason)
		if record.Fulfillment == nil {
			record.Fulfillment = &archaeodomain.RequestFulfillment{}
		}
		record.Fulfillment.Validity = archaeodomain.RequestValidityInvalidated
		record.Fulfillment.RejectedReason = strings.TrimSpace(reason)
		record.Fulfillment.Metadata = mergeMap(record.Fulfillment.Metadata, map[string]any{"conflicting_ref_ids": cloneStrings(conflictingRefs)})
		record, err = s.transitionWithRecord(ctx, record, archaeodomain.RequestStatusInvalidated, nil, reason, nil, false)
		return err
	})
	return record, err
}

func (s Service) Supersede(ctx context.Context, workflowID, requestID, successorID, reason string) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(workflowID, requestID), func() error {
		record, ok, err = s.Load(ctx, workflowID, requestID)
		if err != nil || !ok {
			return err
		}
		now := s.now()
		record.SupersedesRequestID = strings.TrimSpace(successorID)
		record.InvalidatedAt = &now
		record.InvalidationReason = strings.TrimSpace(reason)
		if record.Fulfillment == nil {
			record.Fulfillment = &archaeodomain.RequestFulfillment{}
		}
		record.Fulfillment.Validity = archaeodomain.RequestValiditySuperseded
		record.Fulfillment.RejectedReason = strings.TrimSpace(reason)
		record, err = s.transitionWithRecord(ctx, record, archaeodomain.RequestStatusSuperseded, nil, reason, nil, false)
		return err
	})
	return record, err
}

func (s Service) ApplyFulfillment(ctx context.Context, input ApplyFulfillmentInput) (*archaeodomain.RequestRecord, archaeodomain.RequestValidity, error) {
	var (
		record   *archaeodomain.RequestRecord
		validity archaeodomain.RequestValidity
		err      error
		ok       bool
	)
	err = requestMutationLocks.With(requestLockKey(input.WorkflowID, input.RequestID), func() error {
		record, ok, err = s.Load(ctx, input.WorkflowID, input.RequestID)
		if err != nil || !ok {
			return err
		}
		var invalidation *archaeodomain.RequestInvalidation
		validity, invalidation = EvaluateValidity(record, input.CurrentRevision, input.CurrentSnapshotID, input.ConflictingRefIDs)
		now := s.now()
		fulfillment := input.Fulfillment
		fulfillment.Validity = validity
		if validity == archaeodomain.RequestValidityValid || validity == archaeodomain.RequestValidityPartial {
			fulfillment.Applied = true
			fulfillment.AppliedAt = &now
			record.Result = &archaeodomain.RequestResult{
				Kind:     strings.TrimSpace(fulfillment.Kind),
				RefID:    strings.TrimSpace(fulfillment.RefID),
				Summary:  strings.TrimSpace(fulfillment.Summary),
				Metadata: cloneMap(fulfillment.Metadata),
			}
			record.FulfillmentRef = strings.TrimSpace(fulfillment.RefID)
			record.Fulfillment = &fulfillment
			record.ClaimedBy = ""
			record.ClaimedAt = nil
			record.LeaseExpiresAt = nil
			record, err = s.transitionWithRecord(ctx, record, archaeodomain.RequestStatusCompleted, nil, "", record.Result, false)
			return err
		}
		record.Fulfillment = &fulfillment
		if invalidation != nil {
			record.InvalidatedAt = &invalidation.At
			record.InvalidationReason = invalidation.Reason
			if invalidation.SupersededBy != "" {
				record.SupersedesRequestID = invalidation.SupersededBy
			}
		}
		status := archaeodomain.RequestStatusInvalidated
		if validity == archaeodomain.RequestValiditySuperseded {
			status = archaeodomain.RequestStatusSuperseded
		}
		record, err = s.transitionWithRecord(ctx, record, status, nil, fulfillment.RejectedReason, nil, false)
		if err == nil && record != nil {
			workspaceID := workspaceIDForRequest(record)
			if workspaceID != "" {
				_, _ = (archaeodecisions.Service{Store: s.Store, Now: s.Now, NewID: s.NewID}).Create(ctx, archaeodecisions.CreateInput{
					WorkspaceID:        workspaceID,
					WorkflowID:         record.WorkflowID,
					Kind:               archaeodomain.DecisionKindStaleResult,
					RelatedRequestID:   record.ID,
					RelatedPlanID:      record.PlanID,
					RelatedPlanVersion: cloneInt(record.PlanVersion),
					Validity:           validity,
					Title:              firstNonEmpty(record.Title, "stale result requires decision"),
					Summary:            firstNonEmpty(fulfillment.RejectedReason, record.InvalidationReason, "request fulfillment requires user decision"),
					Metadata: map[string]any{
						"request_kind":        string(record.Kind),
						"conflicting_ref_ids": cloneStrings(input.ConflictingRefIDs),
						"current_revision":    strings.TrimSpace(input.CurrentRevision),
						"current_snapshot_id": strings.TrimSpace(input.CurrentSnapshotID),
					},
				})
			}
		}
		return err
	})
	return record, validity, err
}

func (s Service) Dispatch(ctx context.Context, workflowID, requestID string, metadata map[string]any) (*archaeodomain.RequestRecord, error) {
	return s.transition(ctx, workflowID, requestID, archaeodomain.RequestStatusDispatched, metadata, "", nil, false)
}

func (s Service) Start(ctx context.Context, workflowID, requestID string, metadata map[string]any) (*archaeodomain.RequestRecord, error) {
	return s.transition(ctx, workflowID, requestID, archaeodomain.RequestStatusRunning, metadata, "", nil, false)
}

func (s Service) Complete(ctx context.Context, input CompleteInput) (*archaeodomain.RequestRecord, error) {
	return s.transition(ctx, input.WorkflowID, input.RequestID, archaeodomain.RequestStatusCompleted, nil, "", &input.Result, false)
}

func (s Service) Fail(ctx context.Context, workflowID, requestID, errorText string, retry bool) (*archaeodomain.RequestRecord, error) {
	return s.transition(ctx, workflowID, requestID, archaeodomain.RequestStatusFailed, nil, errorText, nil, retry)
}

func (s Service) Cancel(ctx context.Context, workflowID, requestID, reason string) (*archaeodomain.RequestRecord, error) {
	return s.transition(ctx, workflowID, requestID, archaeodomain.RequestStatusCanceled, nil, reason, nil, false)
}

func (s Service) transition(ctx context.Context, workflowID, requestID string, status archaeodomain.RequestStatus, metadata map[string]any, errorText string, result *archaeodomain.RequestResult, retry bool) (*archaeodomain.RequestRecord, error) {
	var (
		record *archaeodomain.RequestRecord
		err    error
		ok     bool
	)
	err = requestMutationLocks.With(requestLockKey(workflowID, requestID), func() error {
		record, ok, err = s.Load(ctx, workflowID, requestID)
		if err != nil || !ok {
			return err
		}
		record, err = s.transitionWithRecord(ctx, record, status, metadata, errorText, result, retry)
		return err
	})
	return record, err
}

func (s Service) transitionWithRecord(ctx context.Context, record *archaeodomain.RequestRecord, status archaeodomain.RequestStatus, metadata map[string]any, errorText string, result *archaeodomain.RequestResult, retry bool) (*archaeodomain.RequestRecord, error) {
	if record == nil {
		return nil, nil
	}
	now := s.now()
	record.Status = status
	record.UpdatedAt = now
	if len(metadata) > 0 {
		record.DispatchMetadata = mergeMap(record.DispatchMetadata, metadata)
	}
	if strings.TrimSpace(errorText) != "" {
		record.ErrorText = strings.TrimSpace(errorText)
	}
	if retry {
		record.RetryCount++
	}
	switch status {
	case archaeodomain.RequestStatusRunning:
		record.StartedAt = &now
	case archaeodomain.RequestStatusCompleted:
		record.CompletedAt = &now
		if result != nil {
			copyResult := *result
			copyResult.Metadata = cloneMap(copyResult.Metadata)
			record.Result = &copyResult
		}
	case archaeodomain.RequestStatusFailed, archaeodomain.RequestStatusCanceled, archaeodomain.RequestStatusInvalidated, archaeodomain.RequestStatusSuperseded:
		record.CompletedAt = &now
	}
	if err := s.save(ctx, record); err != nil {
		return nil, err
	}
	if err := archaeoevents.AppendRequestEvent(ctx, s.Store, *record, lifecycleEventForStatus(status), lifecycleMessage(record), metadata, now); err != nil {
		return nil, err
	}
	return record, nil
}

func lifecycleEventForStatus(status archaeodomain.RequestStatus) string {
	switch status {
	case archaeodomain.RequestStatusDispatched:
		return archaeoevents.EventRequestDispatched
	case archaeodomain.RequestStatusRunning:
		return archaeoevents.EventRequestStarted
	case archaeodomain.RequestStatusCompleted:
		return archaeoevents.EventRequestCompleted
	case archaeodomain.RequestStatusFailed:
		return archaeoevents.EventRequestFailed
	case archaeodomain.RequestStatusCanceled:
		return archaeoevents.EventRequestCanceled
	case archaeodomain.RequestStatusInvalidated:
		return archaeoevents.EventRequestCanceled
	case archaeodomain.RequestStatusSuperseded:
		return archaeoevents.EventRequestCanceled
	default:
		return archaeoevents.EventRequestCreated
	}
}

func lifecycleMessage(record *archaeodomain.RequestRecord) string {
	if record == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(record.Title); trimmed != "" {
		return trimmed
	}
	return string(record.Kind)
}

func (s Service) save(ctx context.Context, record *archaeodomain.RequestRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      record.ID,
		WorkflowID:      record.WorkflowID,
		Kind:            requestArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     lifecycleMessage(record),
		SummaryMetadata: map[string]any{"request_kind": string(record.Kind), "request_status": string(record.Status), "correlation_id": record.CorrelationID, "idempotency_key": record.IdempotencyKey},
		InlineRawText:   string(raw),
		CreatedAt:       record.UpdatedAt,
	})
}

func EvaluateValidity(record *archaeodomain.RequestRecord, currentRevision, currentSnapshotID string, conflictingRefs []string) (archaeodomain.RequestValidity, *archaeodomain.RequestInvalidation) {
	if record == nil {
		return archaeodomain.RequestValidityInvalidated, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValidityInvalidated, Reason: "request missing", At: time.Now().UTC()}
	}
	if record.Status == archaeodomain.RequestStatusSuperseded {
		return archaeodomain.RequestValiditySuperseded, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValiditySuperseded, Reason: firstNonEmpty(record.InvalidationReason, "request superseded"), SupersededBy: record.SupersedesRequestID, At: time.Now().UTC()}
	}
	if record.Status == archaeodomain.RequestStatusInvalidated {
		return archaeodomain.RequestValidityInvalidated, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValidityInvalidated, Reason: firstNonEmpty(record.InvalidationReason, "request invalidated"), At: time.Now().UTC()}
	}
	if currentRevision = strings.TrimSpace(currentRevision); currentRevision != "" && strings.TrimSpace(record.BasedOnRevision) != "" && currentRevision != strings.TrimSpace(record.BasedOnRevision) {
		return archaeodomain.RequestValidityInvalidated, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValidityInvalidated, Reason: "based_on_revision changed", At: time.Now().UTC()}
	}
	if currentSnapshotID = strings.TrimSpace(currentSnapshotID); currentSnapshotID != "" && strings.TrimSpace(record.SnapshotID) != "" && currentSnapshotID != strings.TrimSpace(record.SnapshotID) {
		return archaeodomain.RequestValidityInvalidated, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValidityInvalidated, Reason: "semantic snapshot superseded", At: time.Now().UTC()}
	}
	if len(conflictingRefs) > 0 && intersects(record.SubjectRefs, conflictingRefs) {
		return archaeodomain.RequestValidityPartial, &archaeodomain.RequestInvalidation{Validity: archaeodomain.RequestValidityPartial, Reason: "conflicting mutation touched request subject refs", ConflictingRefIDs: cloneStrings(conflictingRefs), At: time.Now().UTC()}
	}
	return archaeodomain.RequestValidityValid, nil
}

func (s Service) findExisting(ctx context.Context, input CreateInput) *archaeodomain.RequestRecord {
	if s.Store == nil || strings.TrimSpace(input.WorkflowID) == "" {
		return nil
	}
	records, err := s.ListByWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return nil
	}
	idempotency := strings.TrimSpace(input.IdempotencyKey)
	correlation := strings.TrimSpace(input.CorrelationID)
	subjectRefs := cloneStrings(input.SubjectRefs)
	normalizedInput := cloneMap(input.Input)
	for i := range records {
		record := records[i]
		if idempotency != "" && strings.TrimSpace(record.IdempotencyKey) == idempotency &&
			record.Status != archaeodomain.RequestStatusSuperseded &&
			record.Status != archaeodomain.RequestStatusInvalidated {
			return &record
		}
		if correlation != "" && strings.TrimSpace(record.CorrelationID) == correlation && record.Kind == input.Kind &&
			record.Status != archaeodomain.RequestStatusSuperseded &&
			record.Status != archaeodomain.RequestStatusInvalidated {
			return &record
		}
		if record.Kind == input.Kind &&
			strings.TrimSpace(record.ExplorationID) == strings.TrimSpace(input.ExplorationID) &&
			strings.TrimSpace(record.SnapshotID) == strings.TrimSpace(input.SnapshotID) &&
			strings.TrimSpace(record.BasedOnRevision) == strings.TrimSpace(input.BasedOnRevision) &&
			sameStringSet(record.SubjectRefs, subjectRefs) &&
			sameAnyMap(record.Input, normalizedInput) &&
			record.Status != archaeodomain.RequestStatusSuperseded &&
			record.Status != archaeodomain.RequestStatusInvalidated {
			return &record
		}
	}
	return nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", prefix, s.now().UnixNano())
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, entry := range value {
		out[key] = entry
	}
	return out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func mergeMap(existing, update map[string]any) map[string]any {
	if len(existing) == 0 && len(update) == 0 {
		return nil
	}
	out := cloneMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range update {
		out[key] = value
	}
	return out
}

func workspaceIDForRequest(record *archaeodomain.RequestRecord) string {
	if record == nil {
		return ""
	}
	for _, source := range []map[string]any{record.Input, record.DispatchMetadata} {
		if value, ok := source["workspace_id"]; ok {
			if typed, ok := value.(string); ok && strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		}
	}
	return ""
}

func requestLockKey(workflowID, requestID string) string {
	_ = requestID
	return "workflow:" + strings.TrimSpace(workflowID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func intersects(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			seen[trimmed] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[strings.TrimSpace(value)]; ok {
			return true
		}
	}
	return false
}

func sameStringSet(left, right []string) bool {
	if len(uniqueStrings(left)) != len(uniqueStrings(right)) {
		return false
	}
	return !intersects(diffStrings(left, right), right) && !intersects(diffStrings(right, left), left)
}

func sameAnyMap(left, right map[string]any) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			return false
		}
		switch typed := leftValue.(type) {
		case []string:
			other, ok := rightValue.([]string)
			if !ok || !sameStringSet(typed, other) {
				return false
			}
		default:
			if fmt.Sprintf("%v", leftValue) != fmt.Sprintf("%v", rightValue) {
				return false
			}
		}
	}
	return true
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}

func diffStrings(values, baseline []string) []string {
	seen := make(map[string]struct{}, len(baseline))
	for _, value := range baseline {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			seen[trimmed] = struct{}{}
		}
	}
	out := make([]string, 0)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
