package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	platformbrowser "github.com/lexcodex/relurpify/platform/browser"
)

type browserCapability struct {
	service *BrowserService
	spec    *core.AgentRuntimeSpec
}

func (h *browserCapability) Name() string { return "browser" }

func (h *browserCapability) Description() string {
	return "Controls a browser session via a single action-dispatch tool."
}

func (h *browserCapability) Category() string { return "browser" }

func (h *browserCapability) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "action", Type: "string", Required: true},
		{Name: "session_id", Type: "string", Required: false},
		{Name: "backend", Type: "string", Required: false, Default: defaultBrowserBackend},
		{Name: "url", Type: "string", Required: false},
		{Name: "selector", Type: "string", Required: false},
		{Name: "text", Type: "string", Required: false},
		{Name: "script", Type: "string", Required: false},
		{Name: "timeout_ms", Type: "number", Required: false, Default: 10000},
	}
}

func (h *browserCapability) IsAvailable(context.Context, *core.Context) bool {
	if h != nil && h.spec != nil && h.spec.Browser != nil {
		return h.spec.Browser.Enabled
	}
	return true
}

func (h *browserCapability) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(".", core.FileSystemRead)}
}

func (h *browserCapability) Tags() []string { return []string{core.TagNetwork, "browser", "web"} }

func (h *browserCapability) SetAgentSpec(spec *core.AgentRuntimeSpec, _ string) {
	h.spec = spec
	if h != nil && h.service != nil {
		h.service.agentSpec = spec
	}
}

func (h *browserCapability) SetPermissionManager(manager *fauthorization.PermissionManager, _ string) {
	if h != nil && h.service != nil {
		h.service.permissionManager = manager
	}
}

func (h *browserCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	desc := core.CapabilityDescriptor{
		ID:          "tool:browser",
		Kind:        core.CapabilityKindTool,
		Name:        "browser",
		Version:     "v1",
		Description: "Controls a browser session via a single action-dispatch tool.",
		Category:    "browser",
		Source: core.CapabilitySource{
			ProviderID: "browser",
			Scope:      core.CapabilityScopeProvider,
		},
		TrustClass:    core.TrustClassProviderLocalUntrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassNetwork, core.RiskClassSessioned, core.RiskClassExfiltration},
		EffectClasses: []core.EffectClass{core.EffectClassNetworkEgress, core.EffectClassContextInsertion, core.EffectClassSessionCreation},
		InputSchema:   core.ToolInputSchema(h),
		Availability: core.AvailabilitySpec{
			Available: true,
		},
		Annotations: map[string]any{
			"provider_id": "browser",
		},
	}
	return core.NormalizeCapabilityDescriptor(desc)
}

func (h *browserCapability) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	return h.Execute(ctx, state, args)
}

func (h *browserCapability) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	action := canonicalBrowserAction(fmt.Sprint(args["action"]))
	if err := h.authorizeAction(ctx, action, state, args); err != nil {
		return nil, err
	}
	switch action {
	case browserActionOpen:
		return h.service.open(ctx, state, args)
	case browserActionNavigate:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Navigate(ctx, fmt.Sprint(args["url"])); err != nil {
			return nil, err
		}
		return h.service.successWithSnapshot(ctx, state, session, sessionID, nil)
	case browserActionClick:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Click(ctx, fmt.Sprint(args["selector"])); err != nil {
			return nil, err
		}
		return h.service.successWithSnapshot(ctx, state, session, sessionID, nil)
	case browserActionType:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Type(ctx, fmt.Sprint(args["selector"]), fmt.Sprint(args["text"])); err != nil {
			return nil, err
		}
		return h.service.successWithSnapshot(ctx, state, session, sessionID, nil)
	case browserActionGetText:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractText(ctx, fmt.Sprint(args["selector"]))
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "text")), nil
	case browserActionExtract:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		pageState, err := session.CapturePageState(ctx)
		if err != nil {
			return nil, err
		}
		structured, structuredExtraction, err := session.ExtractStructured(ctx)
		if err != nil {
			return nil, err
		}
		axTree, err := session.ExtractAccessibilityTree(ctx)
		if err != nil {
			return nil, err
		}
		result := withExtraction(sessionID, axTree, "accessibility_tree")
		result["page_state"] = pageState
		result["structured"] = structured
		result["structured_truncated"] = structuredExtraction.Truncated
		result["structured_original_tokens"] = structuredExtraction.OriginalTokens
		result["structured_final_tokens"] = structuredExtraction.FinalTokens
		result["capabilities"] = session.Capabilities()
		recordBrowserObservation(state, pageState)
		return success(result), nil
	case browserActionGetHTML:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractHTML(ctx)
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "html")), nil
	case browserActionGetAXTree:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractAccessibilityTree(ctx)
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "accessibility_tree")), nil
	case browserActionExecuteJS:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		result, err := session.ExecuteScript(ctx, fmt.Sprint(args["script"]))
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID, "result": result}), nil
	case browserActionScreenshot:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		data, err := session.Screenshot(ctx)
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{
			"session_id": sessionID,
			"png_base64": base64.StdEncoding.EncodeToString(data),
			"size_bytes": len(data),
		}), nil
	case browserActionWait:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.WaitFor(ctx, waitConditionFromArgs(args), timeoutFromArgs(args)); err != nil {
			return nil, err
		}
		return h.service.successWithSnapshot(ctx, state, session, sessionID, nil)
	case browserActionCurrentURL:
		session, sessionID, err := h.service.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		currentURL, err := session.CurrentURL(ctx)
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID, "url": currentURL}), nil
	case browserActionClose:
		return h.service.close(state, args)
	default:
		return nil, fmt.Errorf("unsupported browser action %q", action)
	}
}

func (h *browserCapability) authorizeAction(ctx context.Context, action string, state *core.Context, args map[string]interface{}) error {
	if action == "" {
		return fmt.Errorf("browser action required")
	}
	if h == nil || h.service == nil {
		return nil
	}
	if !shouldEnableBrowserService(h.service.agentSpec) {
		return fmt.Errorf("browser tool disabled by agent spec")
	}
	policy := h.service.actionPolicy(action)
	if action == browserActionOpen {
		backend := strings.ToLower(strings.TrimSpace(h.service.resolveBackend(args)))
		if h.service.agentSpec != nil && h.service.agentSpec.Browser != nil && len(h.service.agentSpec.Browser.AllowedBackends) > 0 {
			allowed := false
			for _, candidate := range h.service.agentSpec.Browser.AllowedBackends {
				if strings.EqualFold(strings.TrimSpace(candidate), backend) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("browser backend %s blocked by agent spec", backend)
			}
		}
		if !h.service.backendAllowed(backend) {
			return fmt.Errorf("browser backend %s blocked by service policy", backend)
		}
	}
	if action == browserActionNavigate {
		if err := h.service.authorizeNavigation(ctx, args); err != nil {
			return err
		}
	}
	if action == browserActionExecuteJS && policy != core.AgentPermissionDeny {
		policy = core.AgentPermissionAsk
	}
	switch policy {
	case "", core.AgentPermissionAllow:
		return nil
	case core.AgentPermissionDeny:
		return fmt.Errorf("browser action %s denied by agent spec", action)
	case core.AgentPermissionAsk:
		risk := fauthorization.RiskLevelMedium
		if action == browserActionExecuteJS {
			risk = fauthorization.RiskLevelHigh
		}
		return h.service.requireActionApproval(ctx, action, state, args, risk)
	default:
		return fmt.Errorf("browser action %s has invalid policy %s", action, policy)
	}
}

func waitConditionFromArgs(args map[string]interface{}) platformbrowser.WaitCondition {
	switch {
	case strings.TrimSpace(fmt.Sprint(args["selector"])) != "" && strings.TrimSpace(fmt.Sprint(args["text"])) != "":
		return platformbrowser.WaitCondition{
			Type:     platformbrowser.WaitForText,
			Selector: fmt.Sprint(args["selector"]),
			Text:     fmt.Sprint(args["text"]),
		}
	case strings.TrimSpace(fmt.Sprint(args["selector"])) != "":
		return platformbrowser.WaitCondition{
			Type:     platformbrowser.WaitForSelector,
			Selector: fmt.Sprint(args["selector"]),
		}
	case strings.TrimSpace(fmt.Sprint(args["url"])) != "":
		return platformbrowser.WaitCondition{
			Type:        platformbrowser.WaitForURLContains,
			URLContains: fmt.Sprint(args["url"]),
		}
	default:
		return platformbrowser.WaitCondition{Type: platformbrowser.WaitForLoad}
	}
}

func timeoutFromArgs(args map[string]interface{}) time.Duration {
	raw := strings.TrimSpace(fmt.Sprint(args["timeout_ms"]))
	if raw == "" || raw == "<nil>" {
		return 10 * time.Second
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 10 * time.Second
	}
	return time.Duration(value) * time.Millisecond
}

func (s *BrowserService) authorizeNavigation(ctx context.Context, args map[string]interface{}) error {
	if s == nil || s.permissionManager == nil {
		return fmt.Errorf("browser navigation requires permission manager")
	}
	rawURL := strings.TrimSpace(fmt.Sprint(args["url"]))
	if rawURL == "" || rawURL == "<nil>" {
		return fmt.Errorf("browser navigation requires url")
	}
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browser navigation url invalid: %w", err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("browser navigation url missing scheme")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("browser navigation url missing host")
	}
	port := parsed.Port()
	portNum := 0
	if port != "" {
		portNum, err = strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("browser navigation url port invalid: %w", err)
		}
	} else if strings.EqualFold(parsed.Scheme, "https") {
		portNum = 443
	} else {
		portNum = 80
	}
	if err := s.permissionManager.CheckNetwork(ctx, s.agentID(), "egress", parsed.Scheme, host, portNum); err != nil {
		return err
	}
	return nil
}

func (s *BrowserService) requireActionApproval(ctx context.Context, action string, state *core.Context, args map[string]interface{}, risk fauthorization.RiskLevel) error {
	if s == nil || s.permissionManager == nil {
		return fmt.Errorf("browser action %s requires approval but permission manager missing", action)
	}
	resource := s.agentID()
	if state != nil {
		if sessionID := defaultSessionID(state, args); sessionID != "" {
			resource = sessionID
		}
	}
	metadata := map[string]string{
		"browser_action": browserPermissionAction(action),
	}
	return s.permissionManager.RequireApproval(ctx, s.agentID(), core.PermissionDescriptor{
		Type:         core.PermissionTypeCapability,
		Action:       browserPermissionAction(action),
		Resource:     resource,
		Metadata:     metadata,
		RequiresHITL: true,
	}, "browser action approval", fauthorization.GrantScopeOneTime, risk, 0)
}
