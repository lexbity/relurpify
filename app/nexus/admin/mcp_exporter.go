package admin

import (
	"context"
	"net/url"
	"strings"

	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
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
			{Name: "nexus.nodes.revoke", Description: "Revoke an enrolled node", Schema: mustSchema(revokeNodeArgs{}), MinScope: "nexus:admin", Handler: handleRevokeNode},
			{Name: "nexus.gateway.list_events", Description: "List gateway event counts", Schema: mustSchema(listEventsArgs{}), MinScope: "nexus:observer", Handler: handleListEvents},
			{Name: "nexus.sessions.close", Description: "Close an active session", Schema: mustSchema(closeSessionArgs{}), MinScope: "nexus:operator", Handler: handleCloseSession},
			{Name: "nexus.channels.restart", Description: "Restart a configured channel adapter", Schema: mustSchema(restartChannelArgs{}), MinScope: "nexus:operator", Handler: handleRestartChannel},
			{Name: "nexus.tokens.issue", Description: "Issue a bearer token", Schema: mustSchema(issueTokenArgs{}), MinScope: "nexus:admin", Handler: handleIssueToken},
			{Name: "nexus.tokens.revoke", Description: "Revoke a bearer token", Schema: mustSchema(revokeTokenArgs{}), MinScope: "nexus:admin", Handler: handleRevokeToken},
			{Name: "nexus.policy.set_rule_enabled", Description: "Enable or disable a policy rule", Schema: mustSchema(setPolicyRuleEnabledArgs{}), MinScope: "nexus:admin", Handler: handleSetPolicyRuleEnabled},
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
		{URI: "nexus://nodes/enrolled", Name: "nodes.enrolled", MIMEType: "application/json"},
		{URI: "nexus://nodes/pending", Name: "nodes.pending", MIMEType: "application/json"},
		{URI: "nexus://channels/status", Name: "channels.status", MIMEType: "application/json"},
		{URI: "nexus://sessions/active", Name: "sessions.active", MIMEType: "application/json"},
		{URI: "nexus://events/stream", Name: "events.stream", MIMEType: "application/json"},
		{URI: "nexus://identity/subjects", Name: "identity.subjects", MIMEType: "application/json"},
		{URI: "nexus://identity/externals", Name: "identity.externals", MIMEType: "application/json"},
		{URI: "nexus://tokens/list", Name: "tokens.list", MIMEType: "application/json"},
		{URI: "nexus://policy/rules", Name: "policy.rules", MIMEType: "application/json"},
		{URI: "nexus://gateway/events", Name: "gateway.events", MIMEType: "application/json"},
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
	switch parsed.Host + parsed.Path {
	case "gateway/health":
		result, err := e.service.Health(ctx, HealthRequest{AdminRequest: requestEnvelope(principal, APIVersionV1Alpha1, tenantID)})
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
	default:
		return nil, AdminError{Code: AdminErrorNotFound, Message: "resource not found", Detail: map[string]any{"uri": uri}}
	}
}
