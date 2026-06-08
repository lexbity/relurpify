package capability

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/safety"
)

// RevocationSnapshot captures the current set of revoked capabilities, providers, and sessions.
type RevocationSnapshot struct {
	Capabilities map[string]string `json:"capabilities,omitempty"`
	Providers    map[string]string `json:"providers,omitempty"`
	Sessions     map[string]string `json:"sessions,omitempty"`
}

// runtimeSafetyController tracks runtime budgets and revocations.
type runtimeSafetyController struct {
	mu sync.Mutex

	spec *safety.RuntimeSafetySpec

	capabilityCalls     map[string]int
	providerCalls       map[string]int
	sessionBytes        map[string]int
	sessionTokens       map[string]int
	sessionSubprocesses map[string]int
	sessionNetworkReqs  map[string]int

	revokedCapabilities map[string]string
	revokedProviders    map[string]string
	revokedSessions     map[string]string
}

func newRuntimeSafetyController() *runtimeSafetyController {
	return &runtimeSafetyController{
		capabilityCalls:     make(map[string]int),
		providerCalls:       make(map[string]int),
		sessionBytes:        make(map[string]int),
		sessionTokens:       make(map[string]int),
		sessionSubprocesses: make(map[string]int),
		sessionNetworkReqs:  make(map[string]int),
		revokedCapabilities: make(map[string]string),
		revokedProviders:    make(map[string]string),
		revokedSessions:     make(map[string]string),
	}
}

func (c *runtimeSafetyController) Configure(spec *safety.RuntimeSafetySpec) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if spec == nil {
		c.spec = nil
		return
	}
	clone := *spec
	c.spec = &clone
}

func (c *runtimeSafetyController) SnapshotSpec() *safety.RuntimeSafetySpec {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.spec == nil {
		return nil
	}
	clone := *c.spec
	return &clone
}

func (c *runtimeSafetyController) RevocationSnapshot() RevocationSnapshot {
	if c == nil {
		return RevocationSnapshot{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return RevocationSnapshot{
		Capabilities: cloneReasonMap(c.revokedCapabilities),
		Providers:    cloneReasonMap(c.revokedProviders),
		Sessions:     cloneReasonMap(c.revokedSessions),
	}
}

func (c *runtimeSafetyController) RevokeCapability(id, reason string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revokedCapabilities[id] = defaultReason(reason)
}

func (c *runtimeSafetyController) RevokeProvider(id, reason string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revokedProviders[id] = defaultReason(reason)
}

func (c *runtimeSafetyController) RevokeSession(id, reason string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revokedSessions[id] = defaultReason(reason)
}

func (c *runtimeSafetyController) ReinstateCapability(id string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.revokedCapabilities, id)
}

func (c *runtimeSafetyController) ReinstateProvider(id string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.revokedProviders, id)
}

func (c *runtimeSafetyController) ReinstateSession(id string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.revokedSessions, id)
}

func (c *runtimeSafetyController) CheckBeforeExecution(desc CapabilityDescriptor) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if reason, ok := c.revokedCapabilities[desc.ID]; ok {
		return fmt.Errorf("capability %s revoked: %s", desc.ID, reason)
	}
	if providerID := desc.Source.ProviderID; providerID != "" {
		if reason, ok := c.revokedProviders[providerID]; ok {
			return fmt.Errorf("provider %s revoked: %s", providerID, reason)
		}
	}
	if sessionID := desc.Source.SessionID; sessionID != "" {
		if reason, ok := c.revokedSessions[sessionID]; ok {
			return fmt.Errorf("session %s revoked: %s", sessionID, reason)
		}
	}
	if c.spec == nil {
		return nil
	}
	if limit := c.spec.MaxCallsPerCapability; limit > 0 && desc.ID != "" {
		if c.capabilityCalls[desc.ID] >= limit {
			return fmt.Errorf("capability %s blocked: call budget exceeded", desc.ID)
		}
	}
	if limit := c.spec.MaxCallsPerProvider; limit > 0 && desc.Source.ProviderID != "" {
		if c.providerCalls[desc.Source.ProviderID] >= limit {
			return fmt.Errorf("provider %s blocked: call budget exceeded", desc.Source.ProviderID)
		}
	}
	if desc.ID != "" {
		c.capabilityCalls[desc.ID]++
	}
	if desc.Source.ProviderID != "" {
		c.providerCalls[desc.Source.ProviderID]++
	}
	return nil
}

func (c *runtimeSafetyController) RecordResult(desc CapabilityDescriptor, result *ports.ToolResult) error {
	if c == nil || c.spec == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sessionID := runtimeSafetySessionID(desc, result)
	if sessionID == "" {
		return nil
	}
	bytes := estimatePayloadBytes(result)
	tokens := estimatePayloadTokens(result)
	if limit := c.spec.MaxBytesPerSession; limit > 0 && c.sessionBytes[sessionID]+bytes > limit {
		return fmt.Errorf("session %s blocked: byte budget exceeded", sessionID)
	}
	if limit := c.spec.MaxOutputTokensSession; limit > 0 && c.sessionTokens[sessionID]+tokens > limit {
		return fmt.Errorf("session %s blocked: output token budget exceeded", sessionID)
	}
	c.sessionBytes[sessionID] += bytes
	c.sessionTokens[sessionID] += tokens
	return nil
}

func (c *runtimeSafetyController) RecordSessionSubprocess(sessionID string, count int) error {
	if c == nil || count <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.consumeSessionBudgetLocked(sessionID, count, c.specMaxSubprocesses, c.sessionSubprocesses, "subprocess budget exceeded")
}

func (c *runtimeSafetyController) RecordSessionNetworkRequest(sessionID string, count int) error {
	if c == nil || count <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.consumeSessionBudgetLocked(sessionID, count, c.specMaxNetworkRequests, c.sessionNetworkReqs, "network request budget exceeded")
}

func (c *runtimeSafetyController) consumeSessionBudgetLocked(sessionID string, count int, limitFn func(*safety.RuntimeSafetySpec) int, bucket map[string]int, message string) error {
	if sessionID == "" || c.spec == nil {
		return nil
	}
	limit := limitFn(c.spec)
	if limit > 0 && bucket[sessionID]+count > limit {
		return fmt.Errorf("session %s blocked: %s", sessionID, message)
	}
	bucket[sessionID] += count
	return nil
}

func (c *runtimeSafetyController) specMaxSubprocesses(spec *safety.RuntimeSafetySpec) int {
	if spec == nil {
		return 0
	}
	return spec.MaxSubprocessesPerSession
}

func (c *runtimeSafetyController) specMaxNetworkRequests(spec *safety.RuntimeSafetySpec) int {
	if spec == nil {
		return 0
	}
	return spec.MaxNetworkRequestsSession
}

func runtimeSafetySessionID(desc CapabilityDescriptor, result *ports.ToolResult) string {
	if desc.Source.SessionID != "" {
		return desc.Source.SessionID
	}
	if result == nil || result.Data == nil {
		return ""
	}
	raw, ok := result.Data["session_id"]
	if !ok || raw == nil {
		return ""
	}
	if sessionID, ok := raw.(string); ok {
		return sessionID
	}
	return fmt.Sprint(raw)
}

func cloneReasonMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func defaultReason(reason string) string {
	if reason == "" {
		return "revoked by runtime policy"
	}
	return reason
}

// RedactAny converts arbitrary structured data into a redacted representation
// suitable for persistence or export.
func RedactAny(input any) any {
	if input == nil {
		return nil
	}
	switch typed := input.(type) {
	case map[string]interface{}:
		return RedactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = redactValue(key, value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, RedactAny(item))
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue("", item))
		}
		return out
	case string:
		return redactValue("", typed)
	default:
		return input
	}
}

// RedactMetadataMap redacts sensitive values from a metadata map.
func RedactMetadataMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = redactValue(key, value)
	}
	return out
}

func redactValue(key string, value interface{}) interface{} {
	if isSensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return RedactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			out[k] = redactValue(k, v)
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case string:
		if looksSensitiveValue(typed) {
			return "[REDACTED]"
		}
		return typed
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{
		"secret", "token", "password", "cookie", "authorization", "auth", "credential", "api_key", "apikey",
	} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func looksSensitiveValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, needle := range []string{"bearer ", "ghp_", "github_pat_", "sk-", "authorization:", "session="} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func estimatePayloadBytes(values ...interface{}) int {
	total := 0
	for _, value := range values {
		total += len(strings.TrimSpace(fmt.Sprint(value)))
	}
	return total
}

func estimatePayloadTokens(values ...interface{}) int {
	return estimatePayloadBytes(values...) / 4
}

func cloneRevocationSnapshot(input RevocationSnapshot) RevocationSnapshot {
	return RevocationSnapshot{
		Capabilities: cloneStringMap(input.Capabilities),
		Providers:    cloneStringMap(input.Providers),
		Sessions:     cloneStringMap(input.Sessions),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func SortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
