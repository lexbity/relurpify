package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
)

const defaultBudgetCategory = "immediate"

// SessionConfig configures a BrowserSession wrapper around a raw backend.
type SessionConfig struct {
	Backend           Backend
	BackendName       string
	PermissionManager *authorization.PermissionManager
	AgentID           string
	Budget            *core.ContextBudget
	BudgetCategory    string
}

// BrowserSession preserves the more explicit external name used by design docs.
type BrowserSession = Session

// Extraction captures a budget-aware browser output.
type Extraction struct {
	Content        string
	OriginalTokens int
	FinalTokens    int
	Truncated      bool
}

// PageState is a compact orientation snapshot for the active page.
type PageState struct {
	URL         string
	Title       string
	Preview     string
	LinkCount   int
	FormCount   int
	InputCount  int
	ButtonCount int
	ObservedAt  time.Time
}

// Session adds Relurpify policy and budgeting around a browser backend.
type Session struct {
	backend           Backend
	backendName       string
	permissionManager *authorization.PermissionManager
	agentID           string
	budget            *core.ContextBudget
	budgetCategory    string

	mu          sync.Mutex
	allocations map[string]string
	closed      bool
}

// NewSession constructs a browser session wrapper.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.Backend == nil {
		return nil, fmt.Errorf("browser backend required")
	}
	category := strings.TrimSpace(cfg.BudgetCategory)
	if category == "" {
		category = defaultBudgetCategory
	}
	return &Session{
		backend:           cfg.Backend,
		backendName:       strings.TrimSpace(cfg.BackendName),
		permissionManager: cfg.PermissionManager,
		agentID:           cfg.AgentID,
		budget:            cfg.Budget,
		budgetCategory:    category,
		allocations:       make(map[string]string),
	}, nil
}

func (s *Session) Navigate(ctx context.Context, rawURL string) error {
	if err := s.authorizeNavigation(ctx, rawURL); err != nil {
		return err
	}
	return wrapError(s.backendName, "navigate", s.backend.Navigate(ctx, rawURL))
}

func (s *Session) Click(ctx context.Context, selector string) error {
	return wrapError(s.backendName, "click", s.backend.Click(ctx, selector))
}

func (s *Session) Type(ctx context.Context, selector, text string) error {
	return wrapError(s.backendName, "type", s.backend.Type(ctx, selector, text))
}

func (s *Session) GetText(ctx context.Context, selector string) (string, error) {
	extraction, err := s.ExtractText(ctx, selector)
	if err != nil {
		return "", err
	}
	return extraction.Content, nil
}

func (s *Session) GetAccessibilityTree(ctx context.Context) (string, error) {
	extraction, err := s.ExtractAccessibilityTree(ctx)
	if err != nil {
		return "", err
	}
	return extraction.Content, nil
}

func (s *Session) GetHTML(ctx context.Context) (string, error) {
	extraction, err := s.ExtractHTML(ctx)
	if err != nil {
		return "", err
	}
	return extraction.Content, nil
}

func (s *Session) ExecuteScript(ctx context.Context, script string) (any, error) {
	result, err := s.backend.ExecuteScript(ctx, script)
	if err != nil {
		return nil, wrapError(s.backendName, "execute_script", err)
	}
	return result, nil
}

func (s *Session) Screenshot(ctx context.Context) ([]byte, error) {
	data, err := s.backend.Screenshot(ctx)
	if err != nil {
		return nil, wrapError(s.backendName, "screenshot", err)
	}
	return data, nil
}

func (s *Session) WaitFor(ctx context.Context, condition WaitCondition, timeout time.Duration) error {
	return wrapError(s.backendName, "wait", s.backend.WaitFor(ctx, condition, timeout))
}

func (s *Session) CurrentURL(ctx context.Context) (string, error) {
	url, err := s.backend.CurrentURL(ctx)
	if err != nil {
		return "", wrapError(s.backendName, "current_url", err)
	}
	return url, nil
}

// Capabilities returns the advertised backend feature set when available.
func (s *Session) Capabilities() Capabilities {
	if s == nil || s.backend == nil {
		return Capabilities{}
	}
	if reporter, ok := s.backend.(CapabilityReporter); ok {
		return reporter.Capabilities()
	}
	return Capabilities{ArbitraryEval: true}
}

// CapturePageState returns a compact summary of the active page using trusted
// backend-owned extraction rather than model-supplied script input.
func (s *Session) CapturePageState(ctx context.Context) (*PageState, error) {
	urlValue, err := s.CurrentURL(ctx)
	if err != nil {
		return nil, err
	}
	titleValue, err := s.backend.ExecuteScript(ctx, "document.title || ''")
	if err != nil {
		return nil, wrapError(s.backendName, "page_state", err)
	}
	countValue, err := s.backend.ExecuteScript(ctx, `(() => ({
		links: document.links.length,
		forms: document.forms.length,
		inputs: document.querySelectorAll("input, textarea, select").length,
		buttons: document.querySelectorAll("button, input[type='button'], input[type='submit']").length
	}))()`)
	if err != nil {
		return nil, wrapError(s.backendName, "page_state", err)
	}
	previewExtraction, err := s.ExtractText(ctx, "body")
	if err != nil && !IsErrorCode(err, ErrNoSuchElement) {
		return nil, err
	}
	state := &PageState{
		URL:        urlValue,
		Title:      strings.TrimSpace(fmt.Sprint(titleValue)),
		ObservedAt: time.Now().UTC(),
	}
	if previewExtraction != nil {
		state.Preview = compactPreview(previewExtraction.Content)
	}
	if counts, ok := countValue.(map[string]any); ok {
		state.LinkCount = intFromAny(counts["links"])
		state.FormCount = intFromAny(counts["forms"])
		state.InputCount = intFromAny(counts["inputs"])
		state.ButtonCount = intFromAny(counts["buttons"])
	}
	return state, nil
}

// ExtractStructured returns compact page-oriented structured data using
// backend-owned evaluation rather than model-supplied JavaScript.
func (s *Session) ExtractStructured(ctx context.Context) (*StructuredPageData, *Extraction, error) {
	value, err := s.backend.ExecuteScript(ctx, `(() => ({
		url: window.location.href,
		title: document.title || "",
		headings: Array.from(document.querySelectorAll("h1,h2,h3"))
			.map(el => (el.innerText || el.textContent || "").trim())
			.filter(Boolean)
			.slice(0, 20),
		links: Array.from(document.querySelectorAll("a[href]"))
			.map(el => ({
				text: (el.innerText || el.textContent || "").trim(),
				href: el.href || ""
			}))
			.filter(item => item.text || item.href)
			.slice(0, 50),
		inputs: Array.from(document.querySelectorAll("input, textarea, select"))
			.map(el => ({
				name: el.getAttribute("name") || "",
				type: el.getAttribute("type") || el.tagName.toLowerCase(),
				placeholder: el.getAttribute("placeholder") || ""
			}))
			.slice(0, 50),
		buttons: Array.from(document.querySelectorAll("button, input[type='button'], input[type='submit']"))
			.map(el => (el.innerText || el.getAttribute("value") || "").trim())
			.filter(Boolean)
			.slice(0, 30),
		code: Array.from(document.querySelectorAll("code"))
			.map(el => (el.innerText || el.textContent || "").trim())
			.filter(Boolean)
			.slice(0, 20)
	}))()`)
	if err != nil {
		return nil, nil, wrapError(s.backendName, "extract_structured", err)
	}
	data, err := normalizeStructuredPageData(value)
	if err != nil {
		return nil, nil, wrapError(s.backendName, "extract_structured", err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, nil, wrapError(s.backendName, "extract_structured", err)
	}
	extraction, err := s.allocateExtraction("structured", string(encoded))
	if err != nil {
		return nil, nil, err
	}
	if extraction.Truncated {
		truncatedData, decodeErr := decodeStructuredPageData(extraction.Content)
		if decodeErr == nil {
			data = truncatedData
		}
	}
	return data, extraction, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.budget != nil {
		for _, itemID := range s.allocations {
			s.budget.Free(s.budgetCategory, 0, itemID)
		}
	}
	s.closed = true
	return wrapError(s.backendName, "close", s.backend.Close())
}

func (s *Session) ExtractText(ctx context.Context, selector string) (*Extraction, error) {
	value, err := s.backend.GetText(ctx, selector)
	if err != nil {
		return nil, wrapError(s.backendName, "get_text", err)
	}
	return s.allocateExtraction("text:"+selector, value)
}

func (s *Session) ExtractHTML(ctx context.Context) (*Extraction, error) {
	value, err := s.backend.GetHTML(ctx)
	if err != nil {
		return nil, wrapError(s.backendName, "get_html", err)
	}
	return s.allocateExtraction("html", value)
}

func (s *Session) ExtractAccessibilityTree(ctx context.Context) (*Extraction, error) {
	value, err := s.backend.GetAccessibilityTree(ctx)
	if err != nil {
		return nil, wrapError(s.backendName, "get_accessibility_tree", err)
	}
	return s.allocateExtraction("ax_tree", value)
}

func (s *Session) authorizeNavigation(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &Error{Code: ErrInvalidURL, Backend: s.backendName, Operation: "navigate", Err: err}
	}
	protocol, port, needsNetworkCheck, allowed := navigationTarget(parsed)
	if !allowed {
		return &Error{
			Code:      ErrNavigationBlocked,
			Backend:   s.backendName,
			Operation: "navigate",
			Err:       fmt.Errorf("navigation scheme %q is blocked", parsed.Scheme),
		}
	}
	if !needsNetworkCheck || s.permissionManager == nil {
		return nil
	}
	host := parsed.Hostname()
	if host == "" {
		return &Error{Code: ErrInvalidURL, Backend: s.backendName, Operation: "navigate", Err: fmt.Errorf("host required")}
	}
	if err := s.permissionManager.CheckNetwork(ctx, s.agentID, "egress", protocol, host, port); err != nil {
		return &Error{Code: ErrNavigationBlocked, Backend: s.backendName, Operation: "navigate", Err: err}
	}
	return nil
}

func navigationTarget(parsed *url.URL) (string, int, bool, bool) {
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
		port := defaultPort(parsed.Scheme)
		if parsed.Port() != "" {
			if value, err := net.LookupPort("tcp", parsed.Port()); err == nil {
				port = value
			}
		}
		return "tcp", port, true, true
	case "about":
		return "", 0, false, strings.EqualFold(parsed.Opaque, "blank")
	default:
		return "", 0, false, false
	}
}

func defaultPort(scheme string) int {
	switch strings.ToLower(scheme) {
	case "https", "wss":
		return 443
	case "http", "ws":
		return 80
	default:
		return 0
	}
}

func (s *Session) allocateExtraction(key, content string) (*Extraction, error) {
	result := &Extraction{
		Content:        content,
		OriginalTokens: core.EstimateTokens(content),
	}
	if s.budget == nil {
		result.FinalTokens = result.OriginalTokens
		return result, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, wrapError(s.backendName, "extract", errors.New("browser session closed"))
	}
	if previous, ok := s.allocations[key]; ok {
		s.budget.Free(s.budgetCategory, 0, previous)
		delete(s.allocations, key)
	}
	remaining := s.budget.GetRemainingBudget(s.budgetCategory)
	if remaining <= 0 {
		result.Content = ""
		result.FinalTokens = 0
		result.Truncated = result.OriginalTokens > 0
		return result, nil
	}
	content = truncateToTokens(content, remaining)
	result.Content = content
	result.FinalTokens = core.EstimateTokens(content)
	result.Truncated = result.FinalTokens < result.OriginalTokens

	itemID := fmt.Sprintf("browser:%s:%s", s.backendName, key)
	if err := s.budget.Allocate(s.budgetCategory, result.FinalTokens, extractionBudgetItem{id: itemID, tokens: result.FinalTokens}); err != nil {
		return nil, err
	}
	s.allocations[key] = itemID
	return result, nil
}

func truncateToTokens(content string, maxTokens int) string {
	if maxTokens <= 0 || content == "" {
		return ""
	}
	if core.EstimateTokens(content) <= maxTokens {
		return content
	}
	maxChars := maxTokens * 4
	if maxChars <= 3 {
		if len(content) <= maxChars {
			return content
		}
		return content[:maxChars]
	}
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars-3] + "..."
}

func compactPreview(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	if len(content) > 240 {
		return content[:240] + "..."
	}
	return content
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func normalizeStructuredPageData(value any) (*StructuredPageData, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decodeStructuredPageData(string(raw))
}

func decodeStructuredPageData(payload string) (*StructuredPageData, error) {
	if strings.TrimSpace(payload) == "" {
		return &StructuredPageData{}, nil
	}
	var data StructuredPageData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

type extractionBudgetItem struct {
	id     string
	tokens int
}

func (i extractionBudgetItem) GetID() string                      { return i.id }
func (i extractionBudgetItem) GetTokenCount() int                 { return i.tokens }
func (i extractionBudgetItem) GetPriority() int                   { return 0 }
func (i extractionBudgetItem) CanCompress() bool                  { return false }
func (i extractionBudgetItem) Compress() (core.BudgetItem, error) { return i, nil }
func (i extractionBudgetItem) CanEvict() bool                     { return true }
