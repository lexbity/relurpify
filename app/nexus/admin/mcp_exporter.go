package admin

import (
	"context"
	"net/url"
	"strings"

	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

type adminToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
	MinScope    string
	Handler     adminToolHandler
}

type MCPExporter struct {
	service AdminService
	tools   []adminToolDef
}

func NewMCPExporter(service AdminService) *MCPExporter {
	return &MCPExporter{
		service: service,
		tools: []adminToolDef{
			{Name: "nexus.nodes.list_pending", Description: "List pending node pairing requests", Schema: mustSchema(listPendingPairingsArgs{}), MinScope: "nexus:operator", Handler: handleListPendingPairings},
			{Name: "nexus.nodes.approve_pairing", Description: "Approve a pending node pairing request", Schema: mustSchema(approvePairingArgs{}), MinScope: "nexus:operator", Handler: handleApprovePairing},
			{Name: "nexus.nodes.reject_pairing", Description: "Reject a pending node pairing request", Schema: mustSchema(rejectPairingArgs{}), MinScope: "nexus:operator", Handler: handleRejectPairing},
			{Name: "nexus.nodes.list", Description: "List enrolled nodes", Schema: mustSchema(listNodesArgs{}), MinScope: "nexus:observer", Handler: handleListNodes},
			{Name: "nexus.nodes.get", Description: "Get a single enrolled node", Schema: mustSchema(getNodeArgs{}), MinScope: "nexus:observer", Handler: handleGetNode},
			{Name: "nexus.nodes.set_capabilities", Description: "Replace the approved capabilities for an enrolled node", Schema: mustSchema(updateNodeCapabilitiesArgs{}), MinScope: "nexus:admin", Handler: handleUpdateNodeCapabilities},
			{Name: "nexus.nodes.revoke", Description: "Revoke an enrolled node", Schema: mustSchema(revokeNodeArgs{}), MinScope: "nexus:admin", Handler: handleRevokeNode},
			{Name: "nexus.gateway.list_events", Description: "List gateway event counts", Schema: mustSchema(listEventsArgs{}), MinScope: "nexus:observer", Handler: handleListEvents},
			{Name: "nexus.runtime.describe_rex", Description: "Describe the managed Rex runtime", Schema: mustSchema(describeRexRuntimeArgs{}), MinScope: "nexus:observer", Handler: handleDescribeRexRuntime},
			{Name: "nexus.runtime.read_rex_admin_snapshot", Description: "Read the Rex admin snapshot", Schema: mustSchema(readRexAdminSnapshotArgs{}), MinScope: "nexus:observer", Handler: handleReadRexAdminSnapshot},
			{Name: "nexus.fmp.list_continuations", Description: "List tenant-scoped FMP continuations", Schema: mustSchema(listFMPContinuationsArgs{}), MinScope: "nexus:observer", Handler: handleListFMPContinuations},
			{Name: "nexus.fmp.read_audit", Description: "Read tenant-scoped FMP audit events for a lineage", Schema: mustSchema(readFMPContinuationAuditArgs{}), MinScope: "nexus:observer", Handler: handleReadFMPContinuationAudit},
			{Name: "nexus.fmp.verify_audit", Description: "Verify the tamper-evident FMP audit chain for a lineage", Schema: mustSchema(verifyFMPAuditTrailArgs{}), MinScope: "nexus:observer", Handler: handleVerifyFMPAuditTrail},
			{Name: "nexus.fmp.list_trust_bundles", Description: "List configured mesh trust bundles", Schema: mustSchema(listFMPTrustBundlesArgs{}), MinScope: "nexus:admin:global", Handler: handleListFMPTrustBundles},
			{Name: "nexus.fmp.upsert_trust_bundle", Description: "Create or update a mesh trust bundle", Schema: mustSchema(upsertFMPTrustBundleArgs{}), MinScope: "nexus:admin:global", Handler: handleUpsertFMPTrustBundle},
			{Name: "nexus.fmp.list_boundary_policies", Description: "List configured FMP boundary policies", Schema: mustSchema(listFMPBoundaryPoliciesArgs{}), MinScope: "nexus:admin:global", Handler: handleListFMPBoundaryPolicies},
			{Name: "nexus.fmp.set_boundary_policy", Description: "Create or update an FMP boundary policy", Schema: mustSchema(setFMPBoundaryPolicyArgs{}), MinScope: "nexus:admin:global", Handler: handleSetFMPBoundaryPolicy},
			{Name: "nexus.fmp.list_tenant_exports", Description: "List tenant export enablement overrides", Schema: mustSchema(listTenantFMPExportsArgs{}), MinScope: "nexus:admin", Handler: handleListTenantFMPExports},
			{Name: "nexus.fmp.set_tenant_export", Description: "Enable or disable an export for the current tenant", Schema: mustSchema(setTenantFMPExportArgs{}), MinScope: "nexus:admin", Handler: handleSetTenantFMPExport},
			{Name: "nexus.fmp.get_tenant_federation_policy", Description: "Get the current tenant federation policy", Schema: mustSchema(getTenantFMPFederationPolicyArgs{}), MinScope: "nexus:admin", Handler: handleGetTenantFMPFederationPolicy},
			{Name: "nexus.fmp.set_tenant_federation_policy", Description: "Set the current tenant federation policy", Schema: mustSchema(setTenantFMPFederationPolicyArgs{}), MinScope: "nexus:admin", Handler: handleSetTenantFMPFederationPolicy},
			{Name: "nexus.fmp.get_effective_federation_policy", Description: "Get the effective federation policy for the current tenant and trust domain", Schema: mustSchema(getEffectiveFMPFederationPolicyArgs{}), MinScope: "nexus:observer", Handler: handleGetEffectiveFMPFederationPolicy},
			{Name: "nexus.sessions.close", Description: "Close an active session", Schema: mustSchema(closeSessionArgs{}), MinScope: "nexus:operator", Handler: handleCloseSession},
			{Name: "nexus.sessions.grant_delegation", Description: "Grant a subject permission to act on a session", Schema: mustSchema(grantSessionDelegationArgs{}), MinScope: "nexus:admin", Handler: handleGrantSessionDelegation},
			{Name: "nexus.channels.restart", Description: "Restart a configured channel adapter", Schema: mustSchema(restartChannelArgs{}), MinScope: "nexus:operator", Handler: handleRestartChannel},
			{Name: "nexus.identity.create_subject", Description: "Create or update a tenant-scoped subject", Schema: mustSchema(createSubjectArgs{}), MinScope: "nexus:admin", Handler: handleCreateSubject},
			{Name: "nexus.identity.bind_external", Description: "Bind an external provider identity to a tenant-scoped subject", Schema: mustSchema(bindExternalIdentityArgs{}), MinScope: "nexus:admin", Handler: handleBindExternalIdentity},
			{Name: "nexus.tokens.issue", Description: "Issue a bearer token", Schema: mustSchema(issueTokenArgs{}), MinScope: "nexus:admin", Handler: handleIssueToken},
			{Name: "nexus.tokens.revoke", Description: "Revoke a bearer token", Schema: mustSchema(revokeTokenArgs{}), MinScope: "nexus:admin", Handler: handleRevokeToken},
			{Name: "nexus.policy.set_rule_enabled", Description: "Enable or disable a policy rule", Schema: mustSchema(setPolicyRuleEnabledArgs{}), MinScope: "nexus:admin", Handler: handleSetPolicyRuleEnabled},
			// Phase 6.4: Compatibility Window Management
			{Name: "nexus.fmp.list_compatibility_windows", Description: "List version compatibility windows", Schema: mustSchema(listFMPCompatibilityWindowsArgs{}), MinScope: "nexus:admin:global", Handler: handleListFMPCompatibilityWindows},
			{Name: "nexus.fmp.set_compatibility_window", Description: "Set a version compatibility window", Schema: mustSchema(setFMPCompatibilityWindowArgs{}), MinScope: "nexus:admin:global", Handler: handleSetFMPCompatibilityWindow},
			{Name: "nexus.fmp.delete_compatibility_window", Description: "Delete a version compatibility window", Schema: mustSchema(deleteFMPCompatibilityWindowArgs{}), MinScope: "nexus:admin:global", Handler: handleDeleteFMPCompatibilityWindow},
			// Phase 6.5: Circuit Breaker Management
			{Name: "nexus.fmp.list_circuit_breakers", Description: "List circuit breaker status per trust domain", Schema: mustSchema(listFMPCircuitBreakersArgs{}), MinScope: "nexus:admin:global", Handler: handleListFMPCircuitBreakers},
			{Name: "nexus.fmp.set_circuit_breaker_config", Description: "Configure a circuit breaker for a trust domain", Schema: mustSchema(setFMPCircuitBreakerConfigArgs{}), MinScope: "nexus:admin:global", Handler: handleSetFMPCircuitBreakerConfig},
			{Name: "nexus.fmp.reset_circuit_breaker", Description: "Reset a circuit breaker to closed state", Schema: mustSchema(resetFMPCircuitBreakerArgs{}), MinScope: "nexus:admin:global", Handler: handleResetFMPCircuitBreaker},
			// Phase 7.2: SLO Signals
			{Name: "nexus.rex.read_slo_signals", Description: "Read Rex control plane SLO signals", Schema: mustSchema(readRexSLOSignalsArgs{}), MinScope: "nexus:admin:global", Handler: handleReadRexSLOSignals},
		},
	}
}

func (e *MCPExporter) ListTools(ctx context.Context) ([]protocol.Tool, error) {
	principal, ok := principalFromContext(ctx)
	if !ok {
		return nil, AdminError{Code: AdminErrorUnauthorized, Message: "admin principal missing"}
	}
	tools := make([]protocol.Tool, 0, len(e.tools))
	for _, tool := range e.tools {
		if !hasScope(principal, tool.MinScope) {
			continue
		}
		tools = append(tools, protocol.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema})
	}
	return tools, nil
}

func (e *MCPExporter) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	principal, ok := principalFromContext(ctx)
	if !ok {
		return adminErrorResult(AdminError{Code: AdminErrorUnauthorized, Message: "admin principal missing"}), nil
	}
	for _, tool := range e.tools {
		if tool.Name != name {
			continue
		}
		if !hasScope(principal, tool.MinScope) {
			return adminErrorResult(AdminError{Code: AdminErrorPolicyDenied, Message: "insufficient scope", Detail: map[string]any{"tool": name, "required_scope": tool.MinScope}}), nil
		}
		version := stringArg(args, "api_version", APIVersionV1Alpha1)
		result, err := tool.Handler(ctx, e.service, principal, version, args)
		if err != nil {
			return adminErrorResult(normalizeAdminError(err)), nil
		}
		return result, nil
	}
	return adminErrorResult(AdminError{Code: AdminErrorNotFound, Message: "tool not found", Detail: map[string]any{"tool": name}}), nil
}

func (e *MCPExporter) ListPrompts(context.Context) ([]protocol.Prompt, error) {
	return nil, nil
}

func (e *MCPExporter) GetPrompt(context.Context, string, map[string]any) (*protocol.GetPromptResult, error) {
	return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "prompt not supported"}
}

func (e *MCPExporter) ListResources(context.Context) ([]protocol.Resource, error) {
	return []protocol.Resource{
		{URI: "nexus://gateway/health", Name: "gateway.health", MIMEType: "application/json"},
		{URI: "nexus://tenants/list", Name: "tenants.list", MIMEType: "application/json"},
		{URI: "nexus://nodes/enrolled", Name: "nodes.enrolled", MIMEType: "application/json"},
		{URI: "nexus://nodes/detail", Name: "nodes.detail", MIMEType: "application/json"},
		{URI: "nexus://nodes/pending", Name: "nodes.pending", MIMEType: "application/json"},
		{URI: "nexus://channels/status", Name: "channels.status", MIMEType: "application/json"},
		{URI: "nexus://sessions/active", Name: "sessions.active", MIMEType: "application/json"},
		{URI: "nexus://events/stream", Name: "events.stream", MIMEType: "application/json"},
		{URI: "nexus://identity/subjects", Name: "identity.subjects", MIMEType: "application/json"},
		{URI: "nexus://identity/externals", Name: "identity.externals", MIMEType: "application/json"},
		{URI: "nexus://tokens/list", Name: "tokens.list", MIMEType: "application/json"},
		{URI: "nexus://policy/rules", Name: "policy.rules", MIMEType: "application/json"},
		{URI: "nexus://gateway/events", Name: "gateway.events", MIMEType: "application/json"},
		{URI: "nexus://runtime/rex", Name: "runtime.rex", MIMEType: "application/json"},
		{URI: "nexus://runtime/rex_admin_snapshot", Name: "runtime.rex_admin_snapshot", MIMEType: "application/json"},
		{URI: "nexus://fmp/continuations", Name: "fmp.continuations", MIMEType: "application/json"},
		{URI: "nexus://fmp/audit", Name: "fmp.audit", MIMEType: "application/json"},
		{URI: "nexus://fmp/audit_verification", Name: "fmp.audit_verification", MIMEType: "application/json"},
		{URI: "nexus://fmp/trust_bundles", Name: "fmp.trust_bundles", MIMEType: "application/json"},
		{URI: "nexus://fmp/boundary_policies", Name: "fmp.boundary_policies", MIMEType: "application/json"},
		{URI: "nexus://fmp/tenant_exports", Name: "fmp.tenant_exports", MIMEType: "application/json"},
		{URI: "nexus://fmp/tenant_federation_policy", Name: "fmp.tenant_federation_policy", MIMEType: "application/json"},
		{URI: "nexus://fmp/effective_federation_policy", Name: "fmp.effective_federation_policy", MIMEType: "application/json"},
	}, nil
}

func (e *MCPExporter) ReadResource(ctx context.Context, uri string) (*protocol.ReadResourceResult, error) {
	principal, ok := principalFromContext(ctx)
	if !ok {
		return nil, AdminError{Code: AdminErrorUnauthorized, Message: "admin principal missing"}
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, AdminError{Code: AdminErrorInvalidArgument, Message: "invalid resource uri", Detail: map[string]any{"uri": uri}}
	}
	tenantID := strings.TrimSpace(parsed.Query().Get("tenant"))
	if tenantID == "" {
		tenantID = tenantFromPrincipal(principal)
	}
	authorizedTenantID, err := authorizeTenant(principal, tenantID)
	if err != nil {
		return nil, normalizeAdminError(err)
	}
	tenantID = authorizedTenantID
	switch parsed.Host + parsed.Path {
	case "gateway/health":
		result, err := e.service.Health(ctx, HealthRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID)})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "tenants/list":
		result, err := e.service.ListTenants(ctx, ListTenantsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "nodes/enrolled":
		result, err := e.service.ListNodes(ctx, ListNodesRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "nodes/detail":
		result, err := e.service.GetNode(ctx, GetNodeRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), NodeID: strings.TrimSpace(parsed.Query().Get("node_id"))})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "nodes/pending":
		result, err := e.service.ListPendingPairings(ctx, ListPendingPairingsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "channels/status":
		result, err := e.service.ListChannels(ctx, ListChannelsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "sessions/active":
		result, err := e.service.ListSessions(ctx, ListSessionsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "events/stream":
		result, err := e.service.ReadEventStream(ctx, ReadEventStreamRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			AfterSeq:     uintArg(parsed.Query(), "after_seq", uintArg(parsed.Query(), "afterSeq", 0)),
			Limit:        parseInt(parsed.Query().Get("limit")),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "identity/subjects":
		result, err := e.service.ListSubjects(ctx, ListSubjectsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "identity/externals":
		result, err := e.service.ListExternalIdentities(ctx, ListExternalIdentitiesRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "tokens/list":
		result, err := e.service.ListTokens(ctx, ListTokensRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "policy/rules":
		result, err := e.service.ListPolicyRules(ctx, ListPolicyRulesRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), Page: pageRequestFromQuery(parsed.Query())})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "gateway/events":
		page := pageRequestFromQuery(parsed.Query())
		result, err := e.service.ListEvents(ctx, ListEventsRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID), PageRequest: page})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "runtime/rex":
		result, err := e.service.DescribeRexRuntime(ctx, DescribeRexRuntimeRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "runtime/rex_admin_snapshot":
		result, err := e.service.ReadRexAdminSnapshot(ctx, ReadRexAdminSnapshotRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/continuations":
		result, err := e.service.ListFMPContinuations(ctx, ListFMPContinuationsRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			Page:         pageRequestFromQuery(parsed.Query()),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/audit":
		result, err := e.service.ReadFMPContinuationAudit(ctx, ReadFMPContinuationAuditRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			LineageID:    strings.TrimSpace(parsed.Query().Get("lineage_id")),
			Limit:        parseInt(parsed.Query().Get("limit")),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/audit_verification":
		result, err := e.service.VerifyFMPAuditTrail(ctx, VerifyFMPAuditTrailRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			LineageID:    strings.TrimSpace(parsed.Query().Get("lineage_id")),
			Limit:        parseInt(parsed.Query().Get("limit")),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/trust_bundles":
		result, err := e.service.ListFMPTrustBundles(ctx, ListFMPTrustBundlesRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			Page:         pageRequestFromQuery(parsed.Query()),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/boundary_policies":
		result, err := e.service.ListFMPBoundaryPolicies(ctx, ListFMPBoundaryPoliciesRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			Page:         pageRequestFromQuery(parsed.Query()),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/tenant_exports":
		result, err := e.service.ListTenantFMPExports(ctx, ListTenantFMPExportsRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			Page:         pageRequestFromQuery(parsed.Query()),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/tenant_federation_policy":
		result, err := e.service.GetTenantFMPFederationPolicy(ctx, GetTenantFMPFederationPolicyRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	case "fmp/effective_federation_policy":
		result, err := e.service.GetEffectiveFMPFederationPolicy(ctx, GetEffectiveFMPFederationPolicyRequest{
			AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID),
			TrustDomain:  strings.TrimSpace(parsed.Query().Get("trust_domain")),
		})
		if err != nil {
			return nil, normalizeAdminError(err)
		}
		return jsonResource(uri, result)
	default:
		return nil, AdminError{Code: AdminErrorNotFound, Message: "resource not found", Detail: map[string]any{"uri": uri}}
	}
}
