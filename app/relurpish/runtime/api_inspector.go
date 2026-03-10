package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/api_server_old"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

func (r *Runtime) ListCapabilities(context.Context) ([]server.CapabilityResource, error) {
	if r == nil || r.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	out := make([]server.CapabilityResource, 0, len(r.Tools.AllCapabilities()))
	for _, capability := range r.Tools.AllCapabilities() {
		exposure := r.Tools.EffectiveExposure(capability)
		out = append(out, server.CapabilityResource{
			Meta: server.InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackServerSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			Capability: capabilityPayload(capability, exposure),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Meta.ID < out[j].Meta.ID
	})
	return out, nil
}

func (r *Runtime) GetCapability(_ context.Context, id string) (*server.CapabilityResource, error) {
	for _, resource := range mustCapabilities(r) {
		if resource.Meta.ID == strings.TrimSpace(id) {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("capability %s not found", id)
}

func (r *Runtime) ListPrompts(context.Context) ([]server.PromptResource, error) {
	if r == nil || r.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	out := make([]server.PromptResource, 0)
	for _, capability := range r.Tools.AllCapabilities() {
		if capability.Kind != core.CapabilityKindPrompt {
			continue
		}
		rendered, err := r.Tools.RenderPrompt(context.Background(), core.NewContext(), capability.ID, nil)
		if err != nil {
			return nil, err
		}
		exposure := r.Tools.EffectiveExposure(capability)
		out = append(out, server.PromptResource{
			Meta: server.InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackServerSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			Prompt: server.PromptPayload{
				PromptID:    capability.ID,
				ProviderID:  capability.Source.ProviderID,
				Description: capability.Description,
				Messages:    serverPromptMessages(rendered),
				Metadata:    summarizeServerAny(rendered.Metadata),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func (r *Runtime) GetPrompt(ctx context.Context, id string) (*server.PromptResource, error) {
	prompts, err := r.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range prompts {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("prompt %s not found", id)
}

func (r *Runtime) ListProviders(ctx context.Context) ([]server.ProviderResource, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	providers, _, err := r.CaptureProviderSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]server.ProviderResource, 0, len(providers))
	for _, provider := range providers {
		out = append(out, server.ProviderResource{
			Meta: server.InspectableMeta{
				ID:         provider.ProviderID,
				Kind:       string(provider.Descriptor.Kind),
				Title:      provider.ProviderID,
				TrustClass: string(provider.Descriptor.TrustBaseline),
				Source:     provider.Descriptor.ConfiguredSource,
				State:      provider.Health.Status,
				CapturedAt: provider.CapturedAt,
			},
			Provider: server.ProviderPayload{
				ProviderID:     provider.ProviderID,
				ProviderKind:   string(provider.Descriptor.Kind),
				TrustBaseline:  string(provider.Descriptor.TrustBaseline),
				Recoverability: string(provider.Recoverability),
				ConfiguredFrom: provider.Descriptor.ConfiguredSource,
				CapabilityIDs:  append([]string(nil), provider.CapabilityIDs...),
				Metadata:       summarizeServerAny(provider.Metadata),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func (r *Runtime) GetProvider(ctx context.Context, id string) (*server.ProviderResource, error) {
	providers, err := r.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range providers {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("provider %s not found", id)
}

func (r *Runtime) ListResources(context.Context) ([]server.ReadableResource, error) {
	if r == nil || r.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	out := make([]server.ReadableResource, 0)
	for _, capability := range r.Tools.AllCapabilities() {
		if capability.Kind != core.CapabilityKindResource {
			continue
		}
		read, err := r.Tools.ReadResource(context.Background(), core.NewContext(), capability.ID)
		if err != nil {
			return nil, err
		}
		exposure := r.Tools.EffectiveExposure(capability)
		out = append(out, server.ReadableResource{
			Meta: server.InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackServerSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			Resource: server.ReadableResourcePayload{
				ResourceID:  capability.ID,
				ProviderID:  capability.Source.ProviderID,
				Description: capability.Description,
				Contents:    serverContentBlocks(read.Contents),
				Metadata:    summarizeServerAny(read.Metadata),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func (r *Runtime) GetResource(ctx context.Context, id string) (*server.ReadableResource, error) {
	resources, err := r.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range resources {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("resource %s not found", id)
}

func (r *Runtime) GetWorkflowResource(_ context.Context, uri string) (*server.ReadableResource, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	store, err := r.openWorkflowStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	ref, err := memory.ParseWorkflowResourceURI(strings.TrimSpace(uri))
	if err != nil {
		return nil, err
	}
	read, err := (memory.WorkflowProjectionService{Store: store}).Project(context.Background(), ref)
	if err != nil {
		return nil, err
	}
	return &server.ReadableResource{
		Meta: server.InspectableMeta{
			ID:         uri,
			Kind:       "workflow-resource",
			Title:      strings.TrimSpace(uri),
			TrustClass: string(core.TrustClassWorkspaceTrusted),
			Scope:      ref.WorkflowID,
			Source:     "workflow",
			State:      string(ref.Tier),
		},
		Resource: server.ReadableResourcePayload{
			ResourceID:       uri,
			Description:      fmt.Sprintf("%s workflow projection resource", ref.Tier),
			WorkflowResource: true,
			WorkflowURI:      uri,
			Contents:         serverContentBlocks(read.Contents),
			Metadata:         summarizeServerAny(read.Metadata),
		},
	}, nil
}

func (r *Runtime) ListSessions(ctx context.Context) ([]server.SessionResource, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	_, sessions, err := r.CaptureProviderSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]server.SessionResource, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, server.SessionResource{
			Meta: server.InspectableMeta{
				ID:         session.Session.ID,
				Kind:       "session",
				Title:      session.Session.ID,
				TrustClass: string(session.Session.TrustClass),
				Scope:      session.Session.WorkflowID,
				Source:     session.Session.ProviderID,
				State:      session.Session.Health,
				CapturedAt: session.CapturedAt,
			},
			Session: server.SessionPayload{
				SessionID:       session.Session.ID,
				ProviderID:      session.Session.ProviderID,
				WorkflowID:      session.Session.WorkflowID,
				TaskID:          session.Session.TaskID,
				Recoverability:  string(session.Session.Recoverability),
				CapabilityIDs:   append([]string(nil), session.Session.CapabilityIDs...),
				LastActivityAt:  session.Session.LastActivityAt,
				MetadataSummary: summarizeServerInterface(session.Session.Metadata),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func (r *Runtime) GetSession(ctx context.Context, id string) (*server.SessionResource, error) {
	sessions, err := r.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range sessions {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

func (r *Runtime) ListApprovals(context.Context) ([]server.ApprovalResource, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	requests := r.PendingHITL()
	out := make([]server.ApprovalResource, 0, len(requests))
	for _, request := range requests {
		if request == nil {
			continue
		}
		kind := serverApprovalKind(*request)
		out = append(out, server.ApprovalResource{
			Meta: server.InspectableMeta{
				ID:         request.ID,
				Kind:       kind,
				Title:      request.Permission.Action,
				Source:     request.Permission.Resource,
				State:      request.State,
				CapturedAt: request.RequestedAt.Format(time.RFC3339),
			},
			Approval: server.ApprovalPayload{
				ID:             request.ID,
				Kind:           kind,
				PermissionType: string(request.Permission.Type),
				Action:         request.Permission.Action,
				Resource:       request.Permission.Resource,
				Risk:           string(request.Risk),
				Scope:          string(request.Scope),
				Justification:  request.Justification,
				Metadata:       cloneStringMap(request.Permission.Metadata),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func (r *Runtime) GetApproval(ctx context.Context, id string) (*server.ApprovalResource, error) {
	approvals, err := r.ListApprovals(ctx)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range approvals {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("approval %s not found", id)
}

func mustCapabilities(r *Runtime) []server.CapabilityResource {
	resources, _ := r.ListCapabilities(context.Background())
	return resources
}

func capabilityPayload(capability core.CapabilityDescriptor, exposure core.CapabilityExposure) server.CapabilityPayload {
	payload := server.CapabilityPayload{
		Description:     capability.Description,
		Category:        capability.Category,
		Exposure:        string(exposure),
		Callable:        exposure == core.CapabilityExposureCallable,
		ProviderID:      capability.Source.ProviderID,
		SessionAffinity: capability.SessionAffinity,
		Availability:    availabilityLabel(capability.Availability),
		RiskClasses:     riskClasses(capability.RiskClasses),
		EffectClasses:   effectClasses(capability.EffectClasses),
		Tags:            append([]string(nil), capability.Tags...),
	}
	if capability.Coordination != nil {
		payload.CoordinationRole = string(capability.Coordination.Role)
		payload.CoordinationTaskTypes = append([]string(nil), capability.Coordination.TaskTypes...)
	}
	return payload
}

func availabilityLabel(spec core.AvailabilitySpec) string {
	if spec.Available {
		return "available"
	}
	if strings.TrimSpace(spec.Reason) != "" {
		return "unavailable: " + strings.TrimSpace(spec.Reason)
	}
	return "unavailable"
}

func riskClasses(values []core.RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClasses(values []core.EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func summarizeServerAny(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return out
}

func serverPromptMessages(rendered *core.PromptRenderResult) []server.PromptMessagePayload {
	if rendered == nil {
		return nil
	}
	out := make([]server.PromptMessagePayload, 0, len(rendered.Messages))
	for _, message := range rendered.Messages {
		out = append(out, server.PromptMessagePayload{
			Role:    message.Role,
			Content: serverContentBlocks(message.Content),
		})
	}
	return out
}

func serverContentBlocks(blocks []core.ContentBlock) []map[string]interface{} {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		switch typed := block.(type) {
		case core.TextContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "text": typed.Text})
		case core.StructuredContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "data": typed.Data})
		case core.ResourceLinkContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "uri": typed.URI, "name": typed.Name, "mime_type": typed.MIMEType})
		case core.EmbeddedResourceContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "resource": typed.Resource})
		case core.BinaryReferenceContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "ref": typed.Ref, "mime_type": typed.MIMEType})
		case core.ErrorContentBlock:
			out = append(out, map[string]interface{}{"type": typed.ContentType(), "code": typed.Code, "message": typed.Message})
		default:
			out = append(out, map[string]interface{}{"type": block.ContentType()})
		}
	}
	return out
}

func summarizeServerInterface(values map[string]interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return out
}

func serverApprovalKind(request fauthorization.PermissionRequest) string {
	action := strings.TrimSpace(request.Permission.Action)
	switch {
	case strings.HasPrefix(action, "provider:"):
		return "provider_operation"
	case strings.Contains(action, "insert"):
		return "insertion"
	case strings.Contains(action, "activate"), strings.Contains(action, "admission"):
		return "admission"
	default:
		return "execution"
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func fallbackServerSource(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return strings.TrimSpace(fallback)
}

func (r *Runtime) openWorkflowStore() (*db.SQLiteWorkflowStateStore, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	path := config.New(r.Config.Workspace).WorkflowStateFile()
	return db.NewSQLiteWorkflowStateStore(path)
}
