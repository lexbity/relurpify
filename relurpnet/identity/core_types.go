package identity

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// EventActor identifies the source of a framework event.
// Defined here to avoid import cycle with framework/core.
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
	TenantID string      `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Kind     SubjectKind `json:"kind" yaml:"kind"`
	ID       string      `json:"id" yaml:"id"`
}

// AuthenticatedPrincipal is the resolved runtime principal after authn.
type AuthenticatedPrincipal struct {
	TenantID      string     `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Subject       SubjectRef `json:"subject" yaml:"subject"`
	AuthMethod    AuthMethod `json:"auth_method" yaml:"auth_method"`
	SessionID     string     `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	Scopes        []string   `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Authenticated bool       `json:"authenticated" yaml:"authenticated"`
	IssuedAt      time.Time  `json:"issued_at,omitempty" yaml:"issued_at,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// ConnectionPrincipal is the resolved runtime principal for a gateway connection.
// It carries the role, event actor, and optional authenticated identity.
type ConnectionPrincipal struct {
	Role          string
	Actor         EventActor
	Authenticated bool
	Principal     *AuthenticatedPrincipal
	FeedScope     string
}

// ExternalIdentity binds a provider identity to a tenant-scoped subject.
type ExternalIdentity struct {
	TenantID      string           `json:"tenant_id" yaml:"tenant_id"`
	Provider      ExternalProvider `json:"provider" yaml:"provider"`
	AccountID     string           `json:"account_id,omitempty" yaml:"account_id,omitempty"`
	ExternalID    string           `json:"external_id" yaml:"external_id"`
	Subject       SubjectRef       `json:"subject" yaml:"subject"`
	VerifiedAt    time.Time        `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
	LastSeenAt    time.Time        `json:"last_seen_at,omitempty" yaml:"last_seen_at,omitempty"`
	DisplayName   string           `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	ProviderLabel string           `json:"provider_label,omitempty" yaml:"provider_label,omitempty"`
}

// ExternalSessionBinding captures how an internal session maps to an external conversation.
type ExternalSessionBinding struct {
	Provider       ExternalProvider `json:"provider" yaml:"provider"`
	AccountID      string           `json:"account_id,omitempty" yaml:"account_id,omitempty"`
	ChannelID      string           `json:"channel_id,omitempty" yaml:"channel_id,omitempty"`
	ConversationID string           `json:"conversation_id,omitempty" yaml:"conversation_id,omitempty"`
	ThreadID       string           `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`
	ExternalUserID string           `json:"external_user_id,omitempty" yaml:"external_user_id,omitempty"`
}

// NodeEnrollment is the durable, server-assigned pairing record for a node.
type NodeEnrollment struct {
	TenantID       string     `json:"tenant_id" yaml:"tenant_id"`
	NodeID         string     `json:"node_id" yaml:"node_id"`
	Owner          SubjectRef `json:"owner" yaml:"owner"`
	TrustClass     TrustClass `json:"trust_class" yaml:"trust_class"`
	PublicKey      []byte     `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	KeyID          string     `json:"key_id,omitempty" yaml:"key_id,omitempty"`
	PairedAt       time.Time  `json:"paired_at,omitempty" yaml:"paired_at,omitempty"`
	LastVerifiedAt time.Time  `json:"last_verified_at,omitempty" yaml:"last_verified_at,omitempty"`
	AuthMethod     AuthMethod `json:"auth_method,omitempty" yaml:"auth_method,omitempty"`
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

func (e ExternalIdentity) Validate() error {
	if strings.TrimSpace(e.TenantID) == "" {
		return errors.New("external identity tenant_id required")
	}
	if err := e.Provider.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.ExternalID) == "" {
		return errors.New("external identity external_id required")
	}
	if err := e.Subject.Validate(); err != nil {
		return err
	}
	if !strings.EqualFold(e.TenantID, e.Subject.TenantID) {
		return errors.New("external identity tenant_id must match subject tenant_id")
	}
	return nil
}

func (b ExternalSessionBinding) Validate() error {
	if err := b.Provider.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(b.ConversationID) == "" {
		return errors.New("session binding conversation_id required")
	}
	return nil
}

func (n NodeEnrollment) Validate() error {
	if strings.TrimSpace(n.TenantID) == "" {
		return errors.New("node enrollment tenant_id required")
	}
	if strings.TrimSpace(n.NodeID) == "" {
		return errors.New("node enrollment node_id required")
	}
	if err := n.Owner.Validate(); err != nil {
		return err
	}
	if !strings.EqualFold(n.TenantID, n.Owner.TenantID) {
		return errors.New("node enrollment tenant_id must match owner tenant_id")
	}
	switch n.TrustClass {
	case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
	default:
		return fmt.Errorf("node enrollment trust class %s invalid", n.TrustClass)
	}
	if !n.LastVerifiedAt.IsZero() && !n.PairedAt.IsZero() && n.LastVerifiedAt.Before(n.PairedAt) {
		return errors.New("node enrollment last_verified_at must be after paired_at")
	}
	if n.AuthMethod != "" {
		if err := n.AuthMethod.Validate(); err != nil {
			return err
		}
	}
	return nil
}
