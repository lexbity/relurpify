package admin

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func testPrincipal(scopes ...string) core.AuthenticatedPrincipal {
	return core.AuthenticatedPrincipal{
		TenantID:      "tenant-a",
		Authenticated: true,
		Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
		Scopes:        scopes,
	}
}

func toolNames(tools []protocol.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

type testPayload struct {
	Name string `json:"name"`
}

func TestMCPHelpersAndParsing(t *testing.T) {
	t.Parallel()

	t.Run("principal context", func(t *testing.T) {
		t.Parallel()

		require.False(t, func() bool {
			_, ok := principalFromContext(nil)
			return ok
		}())

		ctx := WithPrincipal(context.Background(), testPrincipal("nexus:observer"))
		principal, ok := principalFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, "tenant-a", principal.TenantID)
	})

	t.Run("schemas", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			sample   any
			required []string
		}{
			{name: "approve", sample: approvePairingArgs{}, required: []string{"code"}},
			{name: "node", sample: getNodeArgs{}, required: []string{"node_id"}},
			{name: "grant", sample: grantSessionDelegationArgs{}, required: []string{"session_id", "subject_kind", "subject_id"}},
			{name: "policy", sample: setTenantFMPFederationPolicyArgs{}, required: nil},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				schema := mustSchema(tc.sample)
				require.Equal(t, "object", schema["type"])
				if len(tc.required) > 0 {
					require.Equal(t, tc.required, schema["required"])
				}
			})
		}

		schema := mustSchema(struct{ Foo string }{})
		props := schema["properties"].(map[string]any)
		require.Contains(t, props, "cursor")
		require.Contains(t, props, "limit")
	})

	t.Run("result helpers", func(t *testing.T) {
		t.Parallel()

		env := requestEnvelope(testPrincipal("nexus:observer"), "  custom  ", "tenant-a")
		require.Equal(t, "custom", env.APIVersion)
		require.Equal(t, "tenant-a", env.TenantID)
		require.NotEmpty(t, env.RequestID)

		standard, err := structuredResult(map[string]any{"ok": true})
		require.NoError(t, err)
		require.Equal(t, map[string]any{"ok": true}, standard.StructuredContent)

		typed, err := structuredResult(testPayload{Name: "value"})
		require.NoError(t, err)
		require.Equal(t, "value", typed.StructuredContent["name"])

		_, err = structuredResult(make(chan int))
		require.Error(t, err)

		errResult := adminErrorResult(AdminError{Code: AdminErrorNotFound, Message: "missing"})
		require.True(t, errResult.IsError)
		require.Equal(t, AdminErrorNotFound, errResult.StructuredContent["error"].(map[string]any)["code"])

		normalized := normalizeAdminError(errors.New("boom"))
		require.Equal(t, AdminErrorInternal, normalized.Code)
		require.Equal(t, "boom", normalized.Message)

		var converted AdminError
		require.True(t, errorsAs(AdminError{Code: AdminErrorInvalidArgument, Message: "bad"}, &converted))
		require.Equal(t, AdminErrorInvalidArgument, converted.Code)
		require.False(t, errorsAs(errors.New("nope"), &converted))

		resource, err := jsonResource("nexus://test/resource", map[string]any{"name": "value"})
		require.NoError(t, err)
		require.Len(t, resource.Contents, 1)
		require.Equal(t, "nexus://test/resource", resource.Contents[0].URI)
		require.Equal(t, "application/json", resource.Contents[0].MIMEType)
		require.True(t, strings.Contains(resource.Contents[0].Text, `"name":"value"`))

		page := pageRequestFromQuery(url.Values{"cursor": []string{"  token  "}, "limit": []string{"9"}})
		require.Equal(t, "token", page.Cursor)
		require.Equal(t, 9, page.Limit)
		require.Equal(t, "default", defaultTenant("   "))
	})

	t.Run("scope helpers", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "nexus:admin", normalizeScope("gateway:admin"))
		require.Equal(t, "nexus:admin:global", normalizeScope("admin:global"))
		require.Equal(t, "nexus:operator", normalizeScope(" operator "))

		principal := testPrincipal("operator", "gateway:admin:global")
		require.True(t, hasScope(principal, "nexus:observer"))
		require.True(t, hasScope(principal, "nexus:operator"))
		require.True(t, hasScope(principal, "nexus:admin:global"))
		require.False(t, hasScope(testPrincipal("nexus:admin"), "nexus:admin:global"))
		require.False(t, hasScope(principal, "does:not:exist"))

		require.Equal(t, "tenant-a", tenantFromPrincipal(testPrincipal("nexus:observer")))
		require.Equal(t, "fallback", stringArg(nil, "name", "fallback"))
		require.Equal(t, "value", stringArg(map[string]any{"name": " value "}, "name", "fallback"))
		require.Equal(t, 7, intArg(map[string]any{"count": float64(7)}, "count", 0))
		require.True(t, boolArg(map[string]any{"enabled": true}, "enabled", false))
		require.Equal(t, 5.5, floatArg(map[string]any{"ratio": 5.5}, "ratio", 0))
		require.Equal(t, 42, parseInt(" 42 "))
		require.Equal(t, uint64(17), uintArg(url.Values{"seq": []string{"17"}}, "seq", 0))
		require.Equal(t, []string{"a", "b"}, stringListArg(map[string]any{"items": []any{" a ", 3, "", "b"}}, "items"))
		require.Equal(t, []core.SessionOperation{core.SessionOperationInvoke, core.SessionOperationClose}, sessionOperationsArg(map[string]any{"ops": []any{"invoke", "close"}}, "ops"))
	})

	t.Run("json arguments", func(t *testing.T) {
		t.Parallel()

		capabilities := capabilityDescriptorsArg(map[string]any{
			"capabilities": []any{
				map[string]any{"id": "cap-1", "kind": string(core.CapabilityKindTool), "name": "tool-one"},
			},
		}, "capabilities")
		require.Len(t, capabilities, 1)
		require.Equal(t, "cap-1", capabilities[0].ID)
		require.Equal(t, core.CapabilityKindTool, capabilities[0].Kind)

		bundle, err := trustBundleArg(map[string]any{"bundle": map[string]any{"trust_domain": "mesh.example", "bundle_id": "bundle-1"}}, "bundle")
		require.NoError(t, err)
		require.Equal(t, "mesh.example", bundle.TrustDomain)

		policy, err := boundaryPolicyArg(map[string]any{"policy": map[string]any{"trust_domain": "mesh.example", "allow_mediation": true}}, "policy")
		require.NoError(t, err)
		require.True(t, policy.AllowMediation)

		parsed, err := timeArg(map[string]any{"expires_at": "2026-01-02T03:04:05Z"}, "expires_at")
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), parsed.UTC())

		_, err = timeArg(map[string]any{"expires_at": "not-a-time"}, "expires_at")
		require.Error(t, err)

		require.Error(t, decodeJSONArg(nil, "value", &struct{}{}))
		require.Error(t, decodeJSONArg(map[string]any{}, "value", &struct{}{}))

		var decoded testPayload
		require.NoError(t, DecodeStructuredContent(&protocol.CallToolResult{StructuredContent: map[string]any{"name": "value"}}, &decoded))
		require.Equal(t, "value", decoded.Name)
		require.Error(t, DecodeStructuredContent(nil, &decoded))
	})
}

func TestMCPExporterAuthorizationAndMetadata(t *testing.T) {
	t.Parallel()

	exporter := NewMCPExporter(nil)

	_, err := exporter.ListTools(context.Background())
	require.Error(t, err)

	observerTools, err := exporter.ListTools(WithPrincipal(context.Background(), testPrincipal("nexus:observer")))
	require.NoError(t, err)
	require.Contains(t, toolNames(observerTools), "nexus.nodes.list")
	require.NotContains(t, toolNames(observerTools), "nexus.nodes.set_capabilities")
	require.NotContains(t, toolNames(observerTools), "nexus.fmp.list_trust_bundles")

	globalTools, err := exporter.ListTools(WithPrincipal(context.Background(), testPrincipal("nexus:admin:global")))
	require.NoError(t, err)
	require.Contains(t, toolNames(globalTools), "nexus.fmp.list_trust_bundles")
	require.Contains(t, toolNames(globalTools), "nexus.rex.read_slo_signals")

	resources, err := exporter.ListResources(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, resources)
	require.Equal(t, "nexus://gateway/health", resources[0].URI)

	_, err = exporter.ReadResource(context.Background(), "nexus://gateway/health")
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorUnauthorized, adminErr.Code)

	_, err = exporter.ReadResource(WithPrincipal(context.Background(), testPrincipal("nexus:observer")), "://bad-uri")
	require.Error(t, err)
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorInvalidArgument, adminErr.Code)

	_, err = exporter.ReadResource(WithPrincipal(context.Background(), testPrincipal("nexus:observer")), "nexus://unknown/resource")
	require.Error(t, err)
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorNotFound, adminErr.Code)

	unauthorized, err := exporter.CallTool(context.Background(), "nexus.nodes.list", nil)
	require.NoError(t, err)
	require.True(t, unauthorized.IsError)
	require.Equal(t, AdminErrorUnauthorized, unauthorized.StructuredContent["error"].(map[string]any)["code"])

	denied, err := exporter.CallTool(WithPrincipal(context.Background(), testPrincipal("nexus:observer")), "nexus.fmp.list_trust_bundles", nil)
	require.NoError(t, err)
	require.True(t, denied.IsError)
	require.Equal(t, AdminErrorPolicyDenied, denied.StructuredContent["error"].(map[string]any)["code"])

	notFound, err := exporter.CallTool(WithPrincipal(context.Background(), testPrincipal("nexus:admin:global")), "nexus.tool.does_not_exist", nil)
	require.NoError(t, err)
	require.True(t, notFound.IsError)
	require.Equal(t, AdminErrorNotFound, notFound.StructuredContent["error"].(map[string]any)["code"])
}

func TestMCPHandlersRejectUnsupportedVersions(t *testing.T) {
	t.Parallel()

	principal := testPrincipal("nexus:admin:global")
	cases := []struct {
		name string
		fn   adminToolHandler
	}{
		{name: "list pending pairings", fn: handleListPendingPairings},
		{name: "approve pairing", fn: handleApprovePairing},
		{name: "reject pairing", fn: handleRejectPairing},
		{name: "list nodes", fn: handleListNodes},
		{name: "get node", fn: handleGetNode},
		{name: "update node capabilities", fn: handleUpdateNodeCapabilities},
		{name: "revoke node", fn: handleRevokeNode},
		{name: "list events", fn: handleListEvents},
		{name: "describe rex", fn: handleDescribeRexRuntime},
		{name: "read rex snapshot", fn: handleReadRexAdminSnapshot},
		{name: "list continuations", fn: handleListFMPContinuations},
		{name: "read audit", fn: handleReadFMPContinuationAudit},
		{name: "verify audit", fn: handleVerifyFMPAuditTrail},
		{name: "list trust bundles", fn: handleListFMPTrustBundles},
		{name: "list boundary policies", fn: handleListFMPBoundaryPolicies},
		{name: "list tenant exports", fn: handleListTenantFMPExports},
		{name: "get tenant policy", fn: handleGetTenantFMPFederationPolicy},
		{name: "get effective policy", fn: handleGetEffectiveFMPFederationPolicy},
		{name: "close session", fn: handleCloseSession},
		{name: "restart channel", fn: handleRestartChannel},
		{name: "issue token", fn: handleIssueToken},
		{name: "create subject", fn: handleCreateSubject},
		{name: "bind external identity", fn: handleBindExternalIdentity},
		{name: "revoke token", fn: handleRevokeToken},
		{name: "set policy rule", fn: handleSetPolicyRuleEnabled},
		{name: "compatibility windows", fn: handleListFMPCompatibilityWindows},
		{name: "set compatibility window", fn: handleSetFMPCompatibilityWindow},
		{name: "delete compatibility window", fn: handleDeleteFMPCompatibilityWindow},
		{name: "list circuit breakers", fn: handleListFMPCircuitBreakers},
		{name: "reset circuit breaker", fn: handleResetFMPCircuitBreaker},
		{name: "read slo signals", fn: handleReadRexSLOSignals},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := tc.fn(context.Background(), nil, principal, "bogus", nil)
			require.Error(t, err)
			require.Nil(t, result)
			var adminErr AdminError
			require.ErrorAs(t, err, &adminErr)
			require.Equal(t, AdminErrorInvalidArgument, adminErr.Code)
		})
	}
}

func TestMCPHandlersParseArgumentsBeforeServiceCall(t *testing.T) {
	t.Parallel()

	principal := testPrincipal("nexus:admin:global")

	t.Run("grant session delegation", func(t *testing.T) {
		t.Parallel()

		result, err := handleGrantSessionDelegation(context.Background(), nil, principal, APIVersionV1Alpha1, map[string]any{"expires_at": "nope"})
		require.Error(t, err)
		require.Nil(t, result)
		var adminErr AdminError
		require.ErrorAs(t, err, &adminErr)
		require.Equal(t, AdminErrorInvalidArgument, adminErr.Code)
	})

	t.Run("upsert trust bundle", func(t *testing.T) {
		t.Parallel()

		result, err := handleUpsertFMPTrustBundle(context.Background(), nil, principal, APIVersionV1Alpha1, map[string]any{"bundle": "invalid"})
		require.Error(t, err)
		require.Nil(t, result)
		var adminErr AdminError
		require.ErrorAs(t, err, &adminErr)
		require.Equal(t, AdminErrorInvalidArgument, adminErr.Code)
	})

	t.Run("set boundary policy", func(t *testing.T) {
		t.Parallel()

		result, err := handleSetFMPBoundaryPolicy(context.Background(), nil, principal, APIVersionV1Alpha1, map[string]any{"policy": "invalid"})
		require.Error(t, err)
		require.Nil(t, result)
		var adminErr AdminError
		require.ErrorAs(t, err, &adminErr)
		require.Equal(t, AdminErrorInvalidArgument, adminErr.Code)
	})
}
