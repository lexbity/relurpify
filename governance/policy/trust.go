package policy

// TrustClass classifies the trust level of a capability provider.
// Canonical definition — relocated from capability/agentspec.
type TrustClass string

const (
	TrustClassBuiltinTrusted         TrustClass = "builtin-trusted"
	TrustClassWorkspaceTrusted       TrustClass = "workspace-trusted"
	TrustClassLLMGenerated           TrustClass = "llm-generated"
	TrustClassToolResult             TrustClass = "tool-result"
	TrustClassProviderLocalUntrusted TrustClass = "provider-local-untrusted"
	TrustClassRemoteDeclared         TrustClass = "remote-declared-untrusted"
	TrustClassRemoteApproved         TrustClass = "remote-approved"
)
