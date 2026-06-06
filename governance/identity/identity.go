package identity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// EventActor identifies the source of a framework event.
type EventActor struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Label       string   `json:"label,omitempty"`
	TenantID    string   `json:"tenant_id,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	SubjectKind string   `json:"subject_kind,omitempty"`
}

type SubjectKind string

const (
	SubjectKindUser             SubjectKind = "user"
	SubjectKindServiceAccount   SubjectKind = "service_account"
	SubjectKindNode             SubjectKind = "node"
	SubjectKindExternalIdentity SubjectKind = "external_identity"
	SubjectKindSystem           SubjectKind = "system"
)

type AuthMethod string

const (
	AuthMethodAnonymous      AuthMethod = "anonymous"
	AuthMethodBearerToken    AuthMethod = "bearer_token"
	AuthMethodOIDC           AuthMethod = "oidc"
	AuthMethodNodeChallenge  AuthMethod = "node_challenge"
	AuthMethodProviderBind   AuthMethod = "provider_binding"
	AuthMethodBootstrapAdmin AuthMethod = "bootstrap_admin"
)

type TrustClass string

const (
	TrustClassBuiltinTrusted         TrustClass = "builtin_trusted"
	TrustClassWorkspaceTrusted       TrustClass = "workspace_trusted"
	TrustClassProviderLocalUntrusted TrustClass = "provider_local_untrusted"
	TrustClassRemoteDeclared         TrustClass = "remote_declared"
	TrustClassRemoteApproved         TrustClass = "remote_approved"
)

type ExternalProvider string

const (
	ExternalProviderDiscord  ExternalProvider = "discord"
	ExternalProviderTelegram ExternalProvider = "telegram"
	ExternalProviderWebchat  ExternalProvider = "webchat"
	ExternalProviderNexus    ExternalProvider = "nexus"
)

// SubjectRef is the canonical internal identity for ownership and authorization.
type SubjectRef struct {
	TenantID string      `json:"tenant_id,omitempty"`
	Kind     SubjectKind `json:"kind"`
	ID       string      `json:"id"`
}

// AuthenticatedPrincipal is the resolved runtime principal after authn.
type AuthenticatedPrincipal struct {
	TenantID      string     `json:"tenant_id,omitempty"`
	Subject       SubjectRef `json:"subject"`
	AuthMethod    AuthMethod `json:"auth_method"`
	SessionID     string     `json:"session_id,omitempty"`
	Scopes        []string   `json:"scopes,omitempty"`
	Authenticated bool       `json:"authenticated"`
	IssuedAt      time.Time  `json:"issued_at,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at,omitempty"`
}

// ConnectionPrincipal is the resolved runtime principal for a gateway connection.
type ConnectionPrincipal struct {
	Role          string
	Actor         EventActor
	Authenticated bool
	Principal     *AuthenticatedPrincipal
	FeedScope     string
}

func (k SubjectKind) Validate() error {
	switch k {
	case SubjectKindUser, SubjectKindServiceAccount, SubjectKindNode, SubjectKindExternalIdentity, SubjectKindSystem:
		return nil
	default:
		return fmt.Errorf("subject kind %s invalid", k)
	}
}

func (m AuthMethod) Validate() error {
	switch m {
	case AuthMethodAnonymous, AuthMethodBearerToken, AuthMethodOIDC, AuthMethodNodeChallenge, AuthMethodProviderBind, AuthMethodBootstrapAdmin:
		return nil
	default:
		return fmt.Errorf("auth method %s invalid", m)
	}
}

func (p ExternalProvider) Validate() error {
	switch p {
	case ExternalProviderDiscord, ExternalProviderTelegram, ExternalProviderWebchat, ExternalProviderNexus:
		return nil
	default:
		return fmt.Errorf("external provider %s invalid", p)
	}
}

func (s SubjectRef) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.New("subject tenant_id required")
	}
	if err := s.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("subject id required")
	}
	return nil
}

func (s SubjectRef) Matches(actor EventActor) bool {
	if actor.ID == "" || string(s.Kind) == "" {
		return false
	}
	if s.TenantID != "" && actor.TenantID != "" && !strings.EqualFold(s.TenantID, actor.TenantID) {
		return false
	}
	if actor.SubjectKind != "" && actor.SubjectKind != string(s.Kind) {
		return false
	}
	return strings.EqualFold(actor.ID, s.ID)
}

func (p AuthenticatedPrincipal) Validate() error {
	if err := p.AuthMethod.Validate(); err != nil {
		return err
	}
	if err := p.Subject.Validate(); err != nil {
		return err
	}
	if p.TenantID != "" && !strings.EqualFold(p.TenantID, p.Subject.TenantID) {
		return errors.New("principal tenant_id must match subject tenant_id")
	}
	if !p.ExpiresAt.IsZero() && !p.IssuedAt.IsZero() && p.ExpiresAt.Before(p.IssuedAt) {
		return errors.New("principal expires_at must be after issued_at")
	}
	return nil
}
