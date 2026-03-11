package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func (s *service) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error) {
	boundaries, err := s.cfg.Sessions.ListBoundaries(ctx, s.cfg.Partition)
	if err != nil {
		return ListSessionsResult{}, internalError("list sessions failed", err, nil)
	}
	filtered := make([]core.SessionBoundary, 0, len(boundaries))
	for _, boundary := range boundaries {
		if req.TenantID != "" && boundary.TenantID != "" && !strings.EqualFold(req.TenantID, boundary.TenantID) {
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
	boundary, err := s.cfg.Sessions.GetBoundaryBySessionID(ctx, req.SessionID)
	if err != nil {
		return GetSessionResult{}, internalError("get session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if boundary == nil {
		return GetSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	return GetSessionResult{AdminResult: resultEnvelope(req.AdminRequest), Session: boundary}, nil
}

func (s *service) CloseSession(ctx context.Context, req CloseSessionRequest) (CloseSessionResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return CloseSessionResult{}, invalidArgument("session_id required", map[string]any{"field": "session_id"})
	}
	boundary, err := s.cfg.Sessions.GetBoundaryBySessionID(ctx, req.SessionID)
	if err != nil {
		return CloseSessionResult{}, internalError("get session failed", err, map[string]any{"session_id": req.SessionID})
	}
	if boundary == nil {
		return CloseSessionResult{}, notFound("session not found", map[string]any{"session_id": req.SessionID})
	}
	if req.TenantID != "" && boundary.TenantID != "" && !strings.EqualFold(req.TenantID, boundary.TenantID) {
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

func (s *service) ListSubjects(ctx context.Context, req ListSubjectsRequest) (ListSubjectsResult, error) {
	enrollments, err := s.cfg.Identities.ListNodeEnrollments(ctx, defaultTenant(req.TenantID))
	if err != nil {
		return ListSubjectsResult{}, internalError("list subjects failed", err, nil)
	}
	subjects := make([]SubjectInfo, 0, len(enrollments))
	seen := map[string]struct{}{}
	for _, enrollment := range enrollments {
		key := string(enrollment.Owner.Kind) + ":" + enrollment.Owner.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		subjects = append(subjects, SubjectInfo{
			TenantID: enrollment.Owner.TenantID,
			Kind:     enrollment.Owner.Kind,
			ID:       enrollment.Owner.ID,
		})
	}
	subjects = applyPage(subjects, req.Page)
	return ListSubjectsResult{AdminResult: resultEnvelope(req.AdminRequest), PageResult: pageResult(len(subjects)), Subjects: subjects}, nil
}

func (s *service) ListExternalIdentities(ctx context.Context, req ListExternalIdentitiesRequest) (ListExternalIdentitiesResult, error) {
	tenantID := defaultTenant(req.TenantID)
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
	records, err := s.cfg.Tokens.ListTokens(ctx)
	if err != nil {
		return ListTokensResult{}, internalError("list tokens failed", err, nil)
	}
	tokens := make([]TokenInfo, 0, len(records))
	for _, record := range records {
		tokens = append(tokens, TokenInfo{
			ID:         record.ID,
			Name:       record.Name,
			SubjectID:  record.SubjectID,
			Scope:      append([]string(nil), record.Scopes...),
			IssuedAt:   record.IssuedAt,
			ExpiresAt:  record.ExpiresAt,
			LastUsedAt: record.LastUsedAt,
			RevokedAt:  record.RevokedAt,
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
	tokenID, token, err := newAdminToken()
	if err != nil {
		return IssueTokenResult{}, internalError("issue token failed", err, nil)
	}
	record := core.AdminTokenRecord{
		ID:        tokenID,
		Name:      subjectID,
		SubjectID: subjectID,
		TokenHash: HashToken(token),
		Scopes:    append([]string(nil), req.Scopes...),
		IssuedAt:  time.Now().UTC(),
	}
	if err := s.cfg.Tokens.CreateToken(ctx, record); err != nil {
		return IssueTokenResult{}, internalError("persist token failed", err, map[string]any{"subject_id": subjectID})
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
	if err := s.cfg.Tokens.RevokeToken(ctx, req.TokenID, time.Now().UTC()); err != nil {
		return RevokeTokenResult{}, internalError("revoke token failed", err, map[string]any{"token_id": req.TokenID})
	}
	return RevokeTokenResult{AdminResult: resultEnvelope(req.AdminRequest), TokenID: req.TokenID}, nil
}
