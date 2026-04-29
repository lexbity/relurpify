package core

import "codeburg.org/lexbit/relurpify/relurpnet/identity"

type SubjectKind = identity.SubjectKind
type AuthMethod = identity.AuthMethod

const (
	SubjectKindUser             = identity.SubjectKindUser
	SubjectKindServiceAccount   = identity.SubjectKindServiceAccount
	SubjectKindNode             = identity.SubjectKindNode
	SubjectKindExternalIdentity = identity.SubjectKindExternalIdentity
	SubjectKindSystem           = identity.SubjectKindSystem

	AuthMethodAnonymous      = identity.AuthMethodAnonymous
	AuthMethodBearerToken    = identity.AuthMethodBearerToken
	AuthMethodOIDC           = identity.AuthMethodOIDC
	AuthMethodNodeChallenge  = identity.AuthMethodNodeChallenge
	AuthMethodProviderBind   = identity.AuthMethodProviderBind
	AuthMethodBootstrapAdmin = identity.AuthMethodBootstrapAdmin
)

type SubjectRef = identity.SubjectRef
type AuthenticatedPrincipal = identity.AuthenticatedPrincipal
type ExternalIdentity = identity.ExternalIdentity
type ExternalSessionBinding = identity.ExternalSessionBinding
type NodeEnrollment = identity.NodeEnrollment
type AdminTokenRecord = identity.AdminTokenRecord
type TenantRecord = identity.TenantRecord
type SubjectRecord = identity.SubjectRecord
