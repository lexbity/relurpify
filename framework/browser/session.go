package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
)

const defaultBudgetCategory = "immediate"

// SessionConfig configures a BrowserSession wrapper around a raw backend.
type SessionConfig struct {
	Backend           Backend
	BackendName       string
	PermissionManager *runtime.PermissionManager
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

// Session adds Relurpify policy and budgeting around a browser backend.
type Session struct {
	backend           Backend
	backendName       string
	permissionManager *runtime.PermissionManager
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
	if s.permissionManager == nil {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &Error{Code: ErrInvalidURL, Backend: s.backendName, Operation: "navigate", Err: err}
	}
	host := parsed.Hostname()
	if host == "" {
		return &Error{Code: ErrInvalidURL, Backend: s.backendName, Operation: "navigate", Err: fmt.Errorf("host required")}
	}
	protocol, port, needsNetworkCheck := navigationTarget(parsed)
	if !needsNetworkCheck {
		return nil
	}
	if err := s.permissionManager.CheckNetwork(ctx, s.agentID, "egress", protocol, host, port); err != nil {
		return &Error{Code: ErrNavigationBlocked, Backend: s.backendName, Operation: "navigate", Err: err}
	}
	return nil
}

func navigationTarget(parsed *url.URL) (string, int, bool) {
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
		port := defaultPort(parsed.Scheme)
		if parsed.Port() != "" {
			if value, err := net.LookupPort("tcp", parsed.Port()); err == nil {
				port = value
			}
		}
		return "tcp", port, true
	default:
		return "", 0, false
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
