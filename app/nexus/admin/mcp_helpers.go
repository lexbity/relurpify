package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
)

type principalKey struct{}

type adminToolHandler func(context.Context, AdminService, core.AuthenticatedPrincipal, string, map[string]any) (*protocol.CallToolResult, error)

func WithPrincipal(ctx context.Context, principal core.AuthenticatedPrincipal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principalFromContext(ctx context.Context) (core.AuthenticatedPrincipal, bool) {
	if ctx == nil {
		return core.AuthenticatedPrincipal{}, false
	}
	principal, ok := ctx.Value(principalKey{}).(core.AuthenticatedPrincipal)
	return principal, ok
}

func mustSchema(sample any) map[string]any {
	switch sample.(type) {
	case approvePairingArgs, rejectPairingArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "code": map[string]any{"type": "string"}}, "required": []string{"code"}}
	case getNodeArgs, revokeNodeArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "node_id": map[string]any{"type": "string"}}, "required": []string{"node_id"}}
	case getEffectiveFMPFederationPolicyArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "trust_domain": map[string]any{"type": "string"}}, "required": []string{"trust_domain"}}
	case updateNodeCapabilitiesArgs:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_version": map[string]any{"type": "string"},
				"node_id":     map[string]any{"type": "string"},
				"capabilities": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "object"},
				},
			},
			"required": []string{"node_id"},
		}
	case closeSessionArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "session_id": map[string]any{"type": "string"}}, "required": []string{"session_id"}}
	case grantSessionDelegationArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "session_id": map[string]any{"type": "string"}, "subject_kind": map[string]any{"type": "string"}, "subject_id": map[string]any{"type": "string"}, "operations": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "expires_at": map[string]any{"type": "string"}}, "required": []string{"session_id", "subject_kind", "subject_id"}}
	case restartChannelArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "channel": map[string]any{"type": "string"}}, "required": []string{"channel"}}
	case issueTokenArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "subject_tenant_id": map[string]any{"type": "string"}, "subject_kind": map[string]any{"type": "string"}, "subject_id": map[string]any{"type": "string"}, "scopes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"subject_id"}}
	case createSubjectArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "subject_tenant_id": map[string]any{"type": "string"}, "subject_kind": map[string]any{"type": "string"}, "subject_id": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string"}, "roles": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"subject_kind", "subject_id"}}
	case bindExternalIdentityArgs:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_version":       map[string]any{"type": "string"},
				"subject_tenant_id": map[string]any{"type": "string"},
				"provider":          map[string]any{"type": "string"},
				"account_id":        map[string]any{"type": "string"},
				"external_id":       map[string]any{"type": "string"},
				"subject_kind":      map[string]any{"type": "string"},
				"subject_id":        map[string]any{"type": "string"},
				"display_name":      map[string]any{"type": "string"},
				"provider_label":    map[string]any{"type": "string"},
			},
			"required": []string{"provider", "external_id", "subject_kind", "subject_id"},
		}
	case revokeTokenArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "token_id": map[string]any{"type": "string"}}, "required": []string{"token_id"}}
	case setPolicyRuleEnabledArgs:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "rule_id": map[string]any{"type": "string"}, "enabled": map[string]any{"type": "boolean"}}, "required": []string{"rule_id", "enabled"}}
	case setTenantFMPFederationPolicyArgs:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_version":           map[string]any{"type": "string"},
				"allowed_trust_domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"allowed_route_modes":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"allow_mediation":       map[string]any{"type": "boolean"},
				"max_transfer_bytes":    map[string]any{"type": "integer"},
			},
		}
	default:
		return map[string]any{"type": "object", "properties": map[string]any{"api_version": map[string]any{"type": "string"}, "cursor": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}}}
	}
}

func requestEnvelope(principal core.AuthenticatedPrincipal, version, tenantID string) AdminRequest {
	return AdminRequest{
		APIVersion: apiVersionOrDefault(version),
		Principal:  principal,
		TenantID:   tenantID,
		RequestID:  fmt.Sprintf("admin-%d", time.Now().UTC().UnixNano()),
	}
}

func structuredResult(v any) (*protocol.CallToolResult, error) {
	// Fast path: already a map — no round-trip needed.
	if m, ok := v.(map[string]any); ok {
		return &protocol.CallToolResult{StructuredContent: m}, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, err
	}
	return &protocol.CallToolResult{StructuredContent: content}, nil
}

func adminErrorResult(err AdminError) *protocol.CallToolResult {
	return &protocol.CallToolResult{
		IsError: true,
		StructuredContent: map[string]any{
			"error": map[string]any{
				"code":    err.Code,
				"message": err.Message,
				"detail":  err.Detail,
			},
		},
		Content: []protocol.ContentBlock{{Type: "text", Text: err.Error()}},
	}
}

func normalizeAdminError(err error) AdminError {
	if err == nil {
		return AdminError{}
	}
	var adminErr AdminError
	if ok := errorsAs(err, &adminErr); ok {
		return adminErr
	}
	return AdminError{Code: AdminErrorInternal, Message: err.Error()}
}

func jsonResource(uri string, value any) (*protocol.ReadResourceResult, error) {
	text, err := MarshalJSONContent(value)
	if err != nil {
		return nil, err
	}
	return &protocol.ReadResourceResult{
		Contents: []protocol.ContentBlock{{Type: "text", Text: text, URI: uri, MIMEType: "application/json"}},
	}, nil
}

func pageRequestFromQuery(values url.Values) PageRequest {
	return PageRequest{Cursor: strings.TrimSpace(values.Get("cursor")), Limit: parseInt(values.Get("limit"))}
}

// scopeRank maps canonical nexus scope names to their rank. Aliases are
// resolved in normalizeScope before the map is consulted.
var scopeRank = map[string]int{
	"nexus:observer":     1,
	"nexus:operator":     2,
	"nexus:admin":        3,
	"nexus:admin:global": 4,
}

// normalizeScope maps well-known scope aliases to their canonical form.
// It allocates only when the input needs trimming/lowering, so the common
// case (already-normalized scopes stored at principal creation) is zero-alloc.
func normalizeScope(s string) string {
	switch s {
	case "nexus:observer", "nexus:operator", "nexus:admin", "nexus:admin:global":
		return s
	case "gateway:admin", "admin":
		return "nexus:admin"
	case "gateway:admin:global", "admin:global":
		return "nexus:admin:global"
	case "operator":
		return "nexus:operator"
	}
	// Only pay for lowercase+trim when the fast-path didn't match.
	normalized := strings.ToLower(strings.TrimSpace(s))
	switch normalized {
	case "gateway:admin", "admin":
		return "nexus:admin"
	case "gateway:admin:global", "admin:global":
		return "nexus:admin:global"
	case "operator":
		return "nexus:operator"
	case "nexus:admin:global":
		return normalized
	default:
		return normalized
	}
}

func hasScope(principal core.AuthenticatedPrincipal, minimum string) bool {
	if minimum == "" {
		return true
	}
	minimum = normalizeScope(minimum)
	minRank, ok := scopeRank[minimum]
	if !ok {
		return false
	}
	for _, scope := range principal.Scopes {
		if scopeRank[normalizeScope(scope)] >= minRank {
			return true
		}
	}
	return false
}

func tenantFromPrincipal(principal core.AuthenticatedPrincipal) string {
	return strings.TrimSpace(principal.TenantID)
}

func stringArg(args map[string]any, key, fallback string) string {
	if args == nil {
		return fallback
	}
	value, _ := args[key].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func intArg(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	switch value := args[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return fallback
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if args == nil {
		return fallback
	}
	value, ok := args[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func floatArg(args map[string]any, key string, fallback float64) float64 {
	if args == nil {
		return fallback
	}
	switch value := args[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return fallback
	}
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func uintArg(values url.Values, key string, fallback uint64) uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(values.Get(key)), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func stringListArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	raw, ok := args[key].([]any)
	if !ok {
		if direct, ok := args[key].([]string); ok {
			return append([]string(nil), direct...)
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func capabilityDescriptorsArg(args map[string]any, key string) []core.CapabilityDescriptor {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out []core.CapabilityDescriptor
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func sessionOperationsArg(args map[string]any, key string) []core.SessionOperation {
	values := stringListArg(args, key)
	if len(values) == 0 {
		return nil
	}
	out := make([]core.SessionOperation, 0, len(values))
	for _, value := range values {
		out = append(out, core.SessionOperation(value))
	}
	return out
}

func timeArg(args map[string]any, key string) (*time.Time, error) {
	value := stringArg(args, key, "")
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func trustBundleArg(args map[string]any, key string) (core.TrustBundle, error) {
	var bundle core.TrustBundle
	if err := decodeJSONArg(args, key, &bundle); err != nil {
		return core.TrustBundle{}, AdminError{Code: AdminErrorInvalidArgument, Message: "bundle invalid", Detail: map[string]any{"field": key, "cause": err.Error()}}
	}
	return bundle, nil
}

func boundaryPolicyArg(args map[string]any, key string) (core.BoundaryPolicy, error) {
	var policy core.BoundaryPolicy
	if err := decodeJSONArg(args, key, &policy); err != nil {
		return core.BoundaryPolicy{}, AdminError{Code: AdminErrorInvalidArgument, Message: "policy invalid", Detail: map[string]any{"field": key, "cause": err.Error()}}
	}
	return policy, nil
}

func decodeJSONArg(args map[string]any, key string, out any) error {
	if args == nil {
		return fmt.Errorf("argument %s required", key)
	}
	value, ok := args[key]
	if !ok {
		return fmt.Errorf("argument %s required", key)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func DecodeStructuredContent[T any](result *protocol.CallToolResult, target *T) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func errorsAs(err error, target *AdminError) bool {
	adminErr, ok := err.(AdminError)
	if ok {
		*target = adminErr
		return true
	}
	return false
}
