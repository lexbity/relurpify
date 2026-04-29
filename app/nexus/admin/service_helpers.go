package admin

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
)

func resultEnvelope(req AdminRequest) AdminResult {
	return AdminResult{
		APIVersion: apiVersionOrDefault(req.APIVersion),
		RequestID:  req.RequestID,
	}
}

func apiVersionOrDefault(version string) string {
	if strings.TrimSpace(version) == "" {
		return APIVersionV1Alpha1
	}
	return strings.TrimSpace(version)
}

func defaultTenant(tenantID string) string {
	if strings.TrimSpace(tenantID) == "" {
		return "default"
	}
	return tenantID
}

func authorizeTenant(principal core.AuthenticatedPrincipal, requestedTenantID string) (string, error) {
	principalTenantID := strings.TrimSpace(principal.TenantID)
	requestedTenantID = strings.TrimSpace(requestedTenantID)
	if requestedTenantID == "" {
		if principalTenantID != "" {
			return principalTenantID, nil
		}
		return defaultTenant(""), nil
	}
	if principalTenantID == "" || strings.EqualFold(requestedTenantID, principalTenantID) || hasGlobalTenantScope(principal) {
		return requestedTenantID, nil
	}
	return "", AdminError{
		Code:    AdminErrorPolicyDenied,
		Message: "cross-tenant access denied",
		Detail: map[string]any{
			"requested_tenant_id": requestedTenantID,
			"principal_tenant_id": principalTenantID,
		},
	}
}

func hasGlobalTenantScope(principal core.AuthenticatedPrincipal) bool {
	for _, scope := range principal.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "nexus:admin:global", "gateway:admin:global", "admin:global":
			return true
		}
	}
	return false
}

func filterEventsByTenant(events []core.FrameworkEvent, tenantID string) []core.FrameworkEvent {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil
	}
	filtered := make([]core.FrameworkEvent, 0, len(events))
	for _, ev := range events {
		if strings.EqualFold(strings.TrimSpace(ev.Actor.TenantID), tenantID) {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}

func copyEventCounts(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func filterNodesByTenant(nodes []fwnode.NodeDescriptor, tenantID string) []fwnode.NodeDescriptor {
	if strings.TrimSpace(tenantID) == "" {
		return nodes // already a fresh slice from the store; no copy needed
	}
	out := make([]fwnode.NodeDescriptor, 0, len(nodes))
	for _, node := range nodes {
		if strings.EqualFold(node.TenantID, tenantID) {
			out = append(out, node)
		}
	}
	return out
}

func coreNodeDescriptorFromNodeDescriptor(node fwnode.NodeDescriptor) core.NodeDescriptor {
	out := core.NodeDescriptor{
		ID:         node.ID,
		TenantID:   node.TenantID,
		Name:       node.Name,
		Platform:   core.NodePlatform(node.Platform),
		TrustClass: node.TrustClass,
		PairedAt:   node.PairedAt.Unix(),
		Owner:      node.Owner.ID,
	}
	if len(node.Tags) > 0 {
		out.Tags = copyStringMap(node.Tags)
	}
	if len(node.ApprovedCapabilities) > 0 {
		out.ApprovedCapabilities = append([]core.CapabilityDescriptor(nil), node.ApprovedCapabilities...)
	}
	return out
}

func coreNodeDescriptorsFromNodeDescriptors(nodes []fwnode.NodeDescriptor) []core.NodeDescriptor {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]core.NodeDescriptor, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, coreNodeDescriptorFromNodeDescriptor(node))
	}
	return out
}

func nodeDescriptorFromCoreNodeDescriptor(node core.NodeDescriptor) fwnode.NodeDescriptor {
	out := fwnode.NodeDescriptor{
		ID:         node.ID,
		TenantID:   node.TenantID,
		Name:       node.Name,
		Platform:   relurpnet.NodePlatform(node.Platform),
		TrustClass: node.TrustClass,
		PairedAt:   time.Unix(node.PairedAt, 0).UTC(),
		Owner:      core.SubjectRef{TenantID: node.TenantID, Kind: core.SubjectKindNode, ID: node.Owner},
	}
	if len(node.Tags) > 0 {
		out.Tags = copyStringMap(node.Tags)
	}
	if len(node.ApprovedCapabilities) > 0 {
		out.ApprovedCapabilities = append([]core.CapabilityDescriptor(nil), node.ApprovedCapabilities...)
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func applyPage[T any](items []T, page PageRequest) []T {
	start := 0
	if page.Cursor != "" {
		if after, err := decodeCursor(page.Cursor); err == nil && after >= 0 {
			start = int(after)
			if start > len(items) {
				start = len(items)
			}
		}
	}
	limit := page.Limit
	if limit <= 0 || start+limit > len(items) {
		limit = len(items) - start
	}
	out := make([]T, 0, limit)
	out = append(out, items[start:start+limit]...)
	return out
}

func pageResult(total int) PageResult {
	return PageResult{NextCursor: "", Total: total}
}

// decodeCursor decodes a pagination cursor into the last-seen sequence number.
// Cursors are base64(decimal seq) — a deliberately simple format that avoids
// a JSON round-trip on every paginated request.
func decodeCursor(cursor string) (uint64, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(string(raw), 10, 64)
}

func nextCursor(events []core.FrameworkEvent) string {
	if len(events) == 0 {
		return ""
	}
	seq := events[len(events)-1].Seq
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(seq, 10)))
}

func invalidArgument(message string, detail map[string]any) error {
	return AdminError{Code: AdminErrorInvalidArgument, Message: message, Detail: detail}
}

func notFound(message string, detail map[string]any) error {
	return AdminError{Code: AdminErrorNotFound, Message: message, Detail: detail}
}

func notImplemented(message string, detail map[string]any) error {
	return AdminError{Code: AdminErrorNotImplemented, Message: message, Detail: detail}
}

func internalError(message string, err error, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	if err != nil {
		detail["cause"] = err.Error()
	}
	return AdminError{Code: AdminErrorInternal, Message: message, Detail: detail}
}
