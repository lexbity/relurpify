package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func (s *service) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListSessionsResult{}, err
	}
	boundaries, err := s.cfg.Sessions.ListBoundaries(ctx, s.cfg.Partition)
	if err != nil {
		return ListSessionsResult{}, internalError("list sessions failed", err, nil)
	}
	filtered := make([]core.SessionBoundary, 0, len(boundaries))
	for _, boundary := range boundaries {
		if tenantID != "" && boundary.TenantID != "" && !strings.EqualFold(tenantID, boundary.TenantID) {
			continue
		}
		filtered = append(filtered, boundary)
	}
	filtered = applyPage(filtered, req.Page)
	return ListSessionsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(filtered)), Sessions: filtered}, nil
}

func (s *service) GetSession(ctx context.Context, req GetSessionRequest) (GetSessionResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return GetSessionResult{}, invalidArgument("session_id required", map[string]any{"field": "session_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return GetSessionResult{}, err
	}
	boundary, err := s.cfg.Sessions.GetBoundaryBySessionID(ctx, req.SessionID)
	if err != nil {
		return GetSessionResult{}, internalError("get session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if boundary == nil {
		return GetSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	if tenantID != "" && boundary.TenantID != "" && !strings.EqualFold(tenantID, boundary.TenantID) {
		return GetSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	return GetSessionResult{AdminResult: resultEnvelope(req.AdminRequest), Session: boundary}, nil
}

func (s *service) CloseSession(ctx context.Context, req CloseSessionRequest) (CloseSessionResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return CloseSessionResult{}, invalidArgument("session_id required", map[string]any{"field": "session_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return CloseSessionResult{}, err
	}
	boundary, err := s.cfg.Sessions.GetBoundaryBySessionID(ctx, req.SessionID)
	if err != nil {
		return CloseSessionResult{}, internalError("get session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if boundary == nil {
		return CloseSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	if tenantID != "" && boundary.TenantID != "" && !strings.EqualFold(tenantID, boundary.TenantID) {
		return CloseSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	key := boundary.RoutingKey
	if strings.TrimSpace(key) == "" {
		key = core.SessionBoundaryKey(boundary.Scope, boundary.Partition, boundary.ChannelID, boundary.PeerID, "")
	}
	if err := s.cfg.Sessions.DeleteBoundary(ctx, key); err != nil {
		return CloseSessionResult{}, internalError("close session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if s.cfg.Events != nil {
		payload, _ := json.Marshal(map[string]any{"session_id": req.SessionID})
		_, _ = s.cfg.Events.Append(ctx, s.cfg.Partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionClosed,
			Payload:   payload,
			Actor: core.EventActor{
				Kind:     "system",
				ID:       req.SessionID,
				TenantID: boundary.TenantID,
			},
			Partition: s.cfg.Partition,
		}})
	}
	return CloseSessionResult{AdminResult: resultEnvelope(req.AdminRequest), SessionID: req.SessionID}, nil
}

func (s *service) GrantSessionDelegation(ctx context.Context, req GrantSessionDelegationRequest) (GrantSessionDelegationResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return GrantSessionDelegationResult{}, invalidArgument("session_id required", map[string]any{"field": "session_id"})
	}
	subjectID := strings.TrimSpace(req.SubjectID)
	if subjectID == "" {
		return GrantSessionDelegationResult{}, invalidArgument("subject_id required", map[string]any{"field": "subject_id"})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return GrantSessionDelegationResult{}, err
	}
	boundary, err := s.cfg.Sessions.GetBoundaryBySessionID(ctx, req.SessionID)
	if err != nil {
		return GrantSessionDelegationResult{}, internalError("get session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if boundary == nil || (tenantID != "" && boundary.TenantID != "" && !strings.EqualFold(tenantID, boundary.TenantID)) {
		return GrantSessionDelegationResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	if err := req.SubjectKind.Validate(); err != nil {
		return GrantSessionDelegationResult{}, invalidArgument("subject_kind invalid", map[string]any{"field": "subject_kind", "cause": err.Error()})
	}
	if s.cfg.Identities != nil {
		subject, err := s.cfg.Identities.GetSubject(ctx, boundary.TenantID, req.SubjectKind, subjectID)
		if err != nil {
			return GrantSessionDelegationResult{}, internalError("lookup subject failed", err, map[string]any{"subject_kind": req.SubjectKind, "subject_id": subjectID})
		}
		if subject == nil {
			return GrantSessionDelegationResult{}, notFound("subject not found", map[string]any{"subject_kind": req.SubjectKind, "subject_id": subjectID})
		}
	}
	record := core.SessionDelegationRecord{
		TenantID:   boundary.TenantID,
		SessionID:  boundary.SessionID,
		Grantee:    core.SubjectRef{TenantID: boundary.TenantID, Kind: req.SubjectKind, ID: subjectID},
		Operations: append([]core.SessionOperation(nil), req.Operations...),
		CreatedAt:  time.Now().UTC(),
	}
	if req.ExpiresAt != nil {
		record.ExpiresAt = req.ExpiresAt.UTC()
	}
	if err := s.cfg.Sessions.UpsertDelegation(ctx, record); err != nil {
		return GrantSessionDelegationResult{}, internalError("grant session delegation failed", err, map[string]any{"session_id": req.SessionID, "subject_id": subjectID})
	}
	return GrantSessionDelegationResult{AdminResult: resultEnvelope(req.AdminRequest), Delegation: record}, nil
}

func (s *service) ListSubjects(ctx context.Context, req ListSubjectsRequest) (ListSubjectsResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListSubjectsResult{}, err
	}
	subjects := make([]SubjectInfo, 0)
	seen := map[string]struct{}{}
	if s.cfg.Identities != nil {
		records, err := s.cfg.Identities.ListSubjects(ctx, tenantID)
		if err != nil {
			return ListSubjectsResult{}, internalError("list subjects failed", err, map[string]any{"tenant_id": tenantID})
		}
		for _, record := range records {
			key := string(record.Kind) + ":" + record.ID
			seen[key] = struct{}{}
			subjects = append(subjects, SubjectInfo{
				TenantID:    record.TenantID,
				Kind:        record.Kind,
				ID:          record.ID,
				DisplayName: record.DisplayName,
				Roles:       append([]string(nil), record.Roles...),
			})
		}
		enrollments, err := s.cfg.Identities.ListNodeEnrollments(ctx, tenantID)
		if err != nil {
			return ListSubjectsResult{}, internalError("list subjects failed", err, map[string]any{"tenant_id": tenantID})
		}
		for _, enrollment := range enrollments {
			key := string(enrollment.Owner.Kind) + ":" + enrollment.Owner.ID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			subjects = append(subjects, SubjectInfo{
				TenantID:    enrollment.Owner.TenantID,
				Kind:        enrollment.Owner.Kind,
				ID:          enrollment.Owner.ID,
				DisplayName: enrollment.Owner.ID,
			})
		}
	}
	subjects = applyPage(subjects, req.Page)
	return ListSubjectsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(subjects)), Subjects: subjects}, nil
}

func (s *service) CreateSubject(ctx context.Context, req CreateSubjectRequest) (CreateSubjectResult, error) {
	if s.cfg.Identities == nil {
		return CreateSubjectResult{}, notImplemented("create subject not implemented", nil)
	}
	subjectID := strings.TrimSpace(req.SubjectID)
	if subjectID == "" {
		return CreateSubjectResult{}, invalidArgument("subject_id required", map[string]any{"field": "subject_id"})
	}
	tenantID := strings.TrimSpace(req.SubjectTenantID)
	if tenantID == "" {
		tenantID = req.AdminRequest.TenantID
	}
	tenantID, err := authorizeTenant(req.Principal, tenantID)
	if err != nil {
		return CreateSubjectResult{}, err
	}
	subjectKind := req.SubjectKind
	if err := subjectKind.Validate(); err != nil {
		return CreateSubjectResult{}, invalidArgument("subject_kind invalid", map[string]any{"field": "subject_kind", "cause": err.Error()})
	}
	now := time.Now().UTC()
	if err := upsertTenantAndSubject(ctx, s.cfg.Identities, tenantID, subjectKind, subjectID, req.DisplayName, req.Roles, now); err != nil {
		return CreateSubjectResult{}, internalError("persist subject failed", err, map[string]any{"tenant_id": tenantID, "subject_kind": subjectKind, "subject_id": subjectID})
	}
	return CreateSubjectResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Subject: SubjectInfo{
			TenantID:    tenantID,
			Kind:        subjectKind,
			ID:          subjectID,
			DisplayName: firstNonEmpty(req.DisplayName, subjectID),
			Roles:       append([]string(nil), req.Roles...),
		},
	}, nil
}

func (s *service) BindExternalIdentity(ctx context.Context, req BindExternalIdentityRequest) (BindExternalIdentityResult, error) {
	if s.cfg.Identities == nil {
		return BindExternalIdentityResult{}, notImplemented("bind external identity not implemented", nil)
	}
	externalID := strings.TrimSpace(req.ExternalID)
	if externalID == "" {
		return BindExternalIdentityResult{}, invalidArgument("external_id required", map[string]any{"field": "external_id"})
	}
	subjectID := strings.TrimSpace(req.SubjectID)
	if subjectID == "" {
		return BindExternalIdentityResult{}, invalidArgument("subject_id required", map[string]any{"field": "subject_id"})
	}
	tenantID := strings.TrimSpace(req.SubjectTenantID)
	if tenantID == "" {
		tenantID = req.AdminRequest.TenantID
	}
	tenantID, err := authorizeTenant(req.Principal, tenantID)
	if err != nil {
		return BindExternalIdentityResult{}, err
	}
	if err := req.Provider.Validate(); err != nil {
		return BindExternalIdentityResult{}, invalidArgument("provider invalid", map[string]any{"field": "provider", "cause": err.Error()})
	}
	if err := req.SubjectKind.Validate(); err != nil {
		return BindExternalIdentityResult{}, invalidArgument("subject_kind invalid", map[string]any{"field": "subject_kind", "cause": err.Error()})
	}
	subject, err := s.cfg.Identities.GetSubject(ctx, tenantID, req.SubjectKind, subjectID)
	if err != nil {
		return BindExternalIdentityResult{}, internalError("lookup subject failed", err, map[string]any{"tenant_id": tenantID, "subject_kind": req.SubjectKind, "subject_id": subjectID})
	}
	if subject == nil {
		return BindExternalIdentityResult{}, notFound("subject not found", map[string]any{"tenant_id": tenantID, "subject_kind": req.SubjectKind, "subject_id": subjectID})
	}
	now := time.Now().UTC()
	identity := core.ExternalIdentity{
		TenantID:      tenantID,
		Provider:      req.Provider,
		AccountID:     strings.TrimSpace(req.AccountID),
		ExternalID:    externalID,
		Subject:       core.SubjectRef{TenantID: tenantID, Kind: req.SubjectKind, ID: subjectID},
		VerifiedAt:    now,
		LastSeenAt:    now,
		DisplayName:   strings.TrimSpace(req.DisplayName),
		ProviderLabel: strings.TrimSpace(req.ProviderLabel),
	}
	if err := s.cfg.Identities.UpsertExternalIdentity(ctx, identity); err != nil {
		return BindExternalIdentityResult{}, internalError("persist external identity failed", err, map[string]any{"tenant_id": tenantID, "provider": req.Provider, "external_id": externalID})
	}
	return BindExternalIdentityResult{
		AdminResult: resultEnvelope(req.AdminRequest),
		Identity:    identity,
	}, nil
}

func (s *service) ListExternalIdentities(ctx context.Context, req ListExternalIdentitiesRequest) (ListExternalIdentitiesResult, error) {
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListExternalIdentitiesResult{}, err
	}
	identities, err := s.cfg.Identities.ListExternalIdentities(ctx, tenantID)
	if err != nil {
		return ListExternalIdentitiesResult{}, internalError("list external identities failed", err, map[string]any{"tenant_id": tenantID})
	}
	identities = applyPage(identities, req.Page)
	return ListExternalIdentitiesResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(identities)), Identities: identities}, nil
}

func (s *service) ListTokens(ctx context.Context, req ListTokensRequest) (ListTokensResult, error) {
	if s.cfg.Tokens == nil {
		return ListTokensResult{}, notImplemented("list tokens not implemented", nil)
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return ListTokensResult{}, err
	}
	records, err := s.cfg.Tokens.ListTokens(ctx)
	if err != nil {
		return ListTokensResult{}, internalError("list tokens failed", err, nil)
	}
	tokens := make([]TokenInfo, 0, len(records))
	for _, record := range records {
		if tenantID != "" && record.TenantID != "" && !strings.EqualFold(tenantID, record.TenantID) {
			continue
		}
		tokens = append(tokens, TokenInfo{
			ID:          record.ID,
			Name:        record.Name,
			TenantID:    record.TenantID,
			SubjectKind: record.SubjectKind,
			SubjectID:   record.SubjectID,
			Scope:       append([]string(nil), record.Scopes...),
			IssuedAt:    record.IssuedAt,
			ExpiresAt:   record.ExpiresAt,
			LastUsedAt:  record.LastUsedAt,
			RevokedAt:   record.RevokedAt,
		})
	}
	tokens = applyPage(tokens, req.Page)
	return ListTokensResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(tokens)), Tokens: tokens}, nil
}

func (s *service) IssueToken(ctx context.Context, req IssueTokenRequest) (IssueTokenResult, error) {
	if s.cfg.Tokens == nil {
		return IssueTokenResult{}, notImplemented("issue token not implemented", nil)
	}
	subjectID := strings.TrimSpace(req.SubjectID)
	if subjectID == "" {
		return IssueTokenResult{}, invalidArgument("subject_id required", map[string]any{"field": "subject_id"})
	}
	tenantID := strings.TrimSpace(req.SubjectTenantID)
	if tenantID == "" {
		tenantID = req.AdminRequest.TenantID
	}
	tenantID, err := authorizeTenant(req.Principal, tenantID)
	if err != nil {
		return IssueTokenResult{}, err
	}
	subjectKind := req.SubjectKind
	if subjectKind == "" {
		subjectKind = core.SubjectKindServiceAccount
	}
	tokenID, token, err := newAdminToken()
	if err != nil {
		return IssueTokenResult{}, internalError("issue token failed", err, nil)
	}
	record := core.AdminTokenRecord{
		ID:          tokenID,
		Name:        subjectID,
		TenantID:    tenantID,
		SubjectKind: subjectKind,
		SubjectID:   subjectID,
		TokenHash:   HashToken(token),
		Scopes:      append([]string(nil), req.Scopes...),
		IssuedAt:    time.Now().UTC(),
	}
	if s.cfg.Identities != nil {
		tenant, err := s.cfg.Identities.GetTenant(ctx, tenantID)
		if err != nil {
			return IssueTokenResult{}, internalError("lookup tenant failed", err, map[string]any{"tenant_id": tenantID})
		}
		if tenant == nil {
			return IssueTokenResult{}, invalidArgument("subject tenant not found", map[string]any{"tenant_id": tenantID})
		}
		subject, err := s.cfg.Identities.GetSubject(ctx, tenantID, subjectKind, subjectID)
		if err != nil {
			return IssueTokenResult{}, internalError("lookup subject failed", err, map[string]any{"tenant_id": tenantID, "subject_kind": subjectKind, "subject_id": subjectID})
		}
		if subject == nil {
			return IssueTokenResult{}, invalidArgument("subject not found", map[string]any{"tenant_id": tenantID, "subject_kind": subjectKind, "subject_id": subjectID})
		}
		if subject.DisabledAt != nil {
			return IssueTokenResult{}, invalidArgument("subject disabled", map[string]any{"tenant_id": tenantID, "subject_kind": subjectKind, "subject_id": subjectID})
		}
	}
	if err := s.cfg.Tokens.CreateToken(ctx, record); err != nil {
		return IssueTokenResult{}, internalError("persist token failed", err, map[string]any{"tenant_id": tenantID, "subject_id": subjectID})
	}
	return IssueTokenResult{AdminResult: resultEnvelope(req.AdminRequest), TokenID: tokenID, Token: token}, nil
}

func (s *service) RevokeToken(ctx context.Context, req RevokeTokenRequest) (RevokeTokenResult, error) {
	if s.cfg.Tokens == nil {
		return RevokeTokenResult{}, notImplemented("revoke token not implemented", nil)
	}
	if strings.TrimSpace(req.TokenID) == "" {
		return RevokeTokenResult{}, invalidArgument("token_id required", map[string]any{"field": "token_id"})
	}
	record, err := s.cfg.Tokens.GetToken(ctx, req.TokenID)
	if err != nil {
		return RevokeTokenResult{}, internalError("get token failed", err, map[string]any{"token_id": req.TokenID})
	}
	if record == nil {
		return RevokeTokenResult{}, notFound("token not found", map[string]any{"token_id": req.TokenID})
	}
	tenantID, err := authorizeTenant(req.Principal, req.TenantID)
	if err != nil {
		return RevokeTokenResult{}, err
	}
	if tenantID != "" && record.TenantID != "" && !strings.EqualFold(tenantID, record.TenantID) {
		return RevokeTokenResult{}, notFound("token not found", map[string]any{"token_id": req.TokenID})
	}
	if err := s.cfg.Tokens.RevokeToken(ctx, req.TokenID, time.Now().UTC()); err != nil {
		return RevokeTokenResult{}, internalError("revoke token failed", err, map[string]any{"token_id": req.TokenID})
	}
	return RevokeTokenResult{AdminResult: resultEnvelope(req.AdminRequest), TokenID: req.TokenID}, nil
}
