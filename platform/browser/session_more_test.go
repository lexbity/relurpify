package browser

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type testBackend struct {
	currentURL       string
	text             string
	textErr          error
	html             string
	htmlErr          error
	accessibility    string
	accessibilityErr error
	scriptFunc       func(string) (any, error)
	scriptErr        error
	screenshotData   []byte
	screenshotErr    error
	clicks           []string
	types            []struct {
		selector string
		text     string
	}
	waits         []WaitCondition
	navigateErr   error
	clickErr      error
	typeErr       error
	waitErr       error
	currentURLErr error
	closeErr      error
	closed        int
}

func (b *testBackend) Navigate(_ context.Context, url string) error {
	if b.navigateErr != nil {
		return b.navigateErr
	}
	b.currentURL = url
	return nil
}

func (b *testBackend) Click(_ context.Context, selector string) error {
	if b.clickErr != nil {
		return b.clickErr
	}
	b.clicks = append(b.clicks, selector)
	return nil
}

func (b *testBackend) Type(_ context.Context, selector, text string) error {
	if b.typeErr != nil {
		return b.typeErr
	}
	b.types = append(b.types, struct {
		selector string
		text     string
	}{selector: selector, text: text})
	return nil
}

func (b *testBackend) GetText(context.Context, string) (string, error) {
	return b.text, b.textErr
}

func (b *testBackend) GetAccessibilityTree(context.Context) (string, error) {
	return b.accessibility, b.accessibilityErr
}

func (b *testBackend) GetHTML(context.Context) (string, error) {
	return b.html, b.htmlErr
}

func (b *testBackend) ExecuteScript(_ context.Context, script string) (any, error) {
	if b.scriptErr != nil {
		return nil, b.scriptErr
	}
	if b.scriptFunc != nil {
		return b.scriptFunc(script)
	}
	return nil, nil
}

func (b *testBackend) Screenshot(context.Context) ([]byte, error) {
	return b.screenshotData, b.screenshotErr
}

func (b *testBackend) WaitFor(_ context.Context, condition WaitCondition, _ time.Duration) error {
	if b.waitErr != nil {
		return b.waitErr
	}
	b.waits = append(b.waits, condition)
	return nil
}

func (b *testBackend) CurrentURL(context.Context) (string, error) {
	return b.currentURL, b.currentURLErr
}

func (b *testBackend) Close() error {
	b.closed++
	return b.closeErr
}

type capabilityBackend struct {
	testBackend
	caps Capabilities
}

func (b *capabilityBackend) Capabilities() Capabilities {
	return b.caps
}

func newTestSession(t *testing.T, backend Backend, opts ...func(*SessionConfig)) *Session {
	t.Helper()
	cfg := SessionConfig{Backend: backend, BackendName: "fake"}
	for _, opt := range opts {
		opt(&cfg)
	}
	session, err := NewSession(cfg)
	require.NoError(t, err)
	return session
}

func newTestBudget(maxTokens int, categories map[string]float64) *core.ArtifactBudget {
	budget := core.NewArtifactBudgetWithPolicy(maxTokens, &core.AllocationPolicy{
		SystemReserved:     0,
		Allocations:        categories,
		AllowBorrowing:     false,
		MinimumPerCategory: 0,
	})
	budget.SetReservations(0, 0, 0)
	return budget
}

func TestErrorHelpers(t *testing.T) {
	var nilErr *Error
	require.Equal(t, "", nilErr.Error())
	require.NoError(t, nilErr.Unwrap())

	base := errors.New("boom")
	err := &Error{
		Code:      ErrTimeout,
		Backend:   "cdp",
		Operation: "navigate",
		Err:       base,
	}

	require.Equal(t, "browser navigate failed (cdp): timeout: boom", err.Error())
	require.Same(t, base, err.Unwrap())
	require.True(t, IsErrorCode(err, ErrTimeout))
	require.False(t, IsErrorCode(err, ErrNoSuchElement))
	require.False(t, IsErrorCode(nil, ErrTimeout))

	wrapped := wrapError("webdriver", "click", base)
	require.True(t, IsErrorCode(wrapped, ErrUnknownOperation))
	var browserErr *Error
	require.ErrorAs(t, wrapped, &browserErr)
	require.Equal(t, "webdriver", browserErr.Backend)
	require.Equal(t, "click", browserErr.Operation)

	existing := &Error{Code: ErrNoSuchElement, Err: base}
	same := wrapError("bidi", "get_text", existing)
	require.Same(t, existing, same)
	require.Equal(t, "bidi", existing.Backend)
	require.Equal(t, "get_text", existing.Operation)
}

func TestStructuredPageDataMarshalJSON(t *testing.T) {
	data := StructuredPageData{
		URL:      "https://example.com",
		Title:    "Example",
		Headings: []string{"Intro"},
		Links:    []StructuredLink{{Text: "Docs", Href: "https://example.com/docs"}},
		Inputs:   []StructuredInput{{Name: "q", Type: "search", Placeholder: "Search"}},
		Buttons:  []string{"Submit"},
		Code:     []string{"fmt.Println(\"ok\")"},
	}

	raw, err := data.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(raw), `"url":"https://example.com"`)
	require.Contains(t, string(raw), `"title":"Example"`)
	require.Contains(t, string(raw), `"buttons":["Submit"]`)
}

func TestNewSessionValidationAndDefaults(t *testing.T) {
	_, err := NewSession(SessionConfig{})
	require.Error(t, err)

	session := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.BackendName = "  fake-backend  "
		cfg.BudgetCategory = "  "
	})

	require.Equal(t, "fake-backend", session.backendName)
	require.Equal(t, defaultBudgetCategory, session.budgetCategory)
}

func TestSessionCapabilities(t *testing.T) {
	var nilSession *Session
	require.Equal(t, Capabilities{}, nilSession.Capabilities())

	session := newTestSession(t, &testBackend{})
	require.Equal(t, Capabilities{ArbitraryEval: true}, session.Capabilities())

	reporting := &capabilityBackend{caps: Capabilities{AccessibilityTree: true, NetworkIntercept: true}}
	session = newTestSession(t, reporting)
	require.Equal(t, Capabilities{AccessibilityTree: true, NetworkIntercept: true}, session.Capabilities())
}

func TestSessionWrappersAndErrorWrapping(t *testing.T) {
	backend := &testBackend{
		text:           "hello",
		html:           "<html><body>hi</body></html>",
		accessibility:  "{\"role\":\"document\"}",
		screenshotData: []byte{0x89, 0x50, 0x4e, 0x47},
		scriptFunc: func(script string) (any, error) {
			if strings.Contains(script, "document.title") {
				return "title", nil
			}
			return map[string]any{"ok": true}, nil
		},
	}
	session := newTestSession(t, backend)

	gotText, err := session.GetText(context.Background(), "#result")
	require.NoError(t, err)
	require.Equal(t, "hello", gotText)

	gotHTML, err := session.GetHTML(context.Background())
	require.NoError(t, err)
	require.Equal(t, "<html><body>hi</body></html>", gotHTML)

	gotAX, err := session.GetAccessibilityTree(context.Background())
	require.NoError(t, err)
	require.Equal(t, "{\"role\":\"document\"}", gotAX)

	result, err := session.ExecuteScript(context.Background(), "return 1")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)

	screenshot, err := session.Screenshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, screenshot)

	require.NoError(t, session.WaitFor(context.Background(), WaitCondition{Type: WaitForSelector, Selector: "#ready"}, time.Second))
	require.Equal(t, []WaitCondition{{Type: WaitForSelector, Selector: "#ready"}}, backend.waits)

	url, err := session.CurrentURL(context.Background())
	require.NoError(t, err)
	require.Empty(t, url)

	errBackend := &testBackend{
		textErr:          &Error{Code: ErrNoSuchElement, Err: errors.New("missing")},
		htmlErr:          errors.New("html failed"),
		accessibilityErr: errors.New("ax failed"),
		scriptErr:        errors.New("script failed"),
		screenshotErr:    errors.New("screenshot failed"),
		waitErr:          errors.New("wait failed"),
		currentURLErr:    errors.New("current url failed"),
		closeErr:         errors.New("close failed"),
	}
	session = newTestSession(t, errBackend)

	_, err = session.GetText(context.Background(), "#missing")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNoSuchElement))

	_, err = session.GetHTML(context.Background())
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	_, err = session.GetAccessibilityTree(context.Background())
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	_, err = session.ExecuteScript(context.Background(), "return 1")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	_, err = session.Screenshot(context.Background())
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	err = session.WaitFor(context.Background(), WaitCondition{Type: WaitForLoad}, time.Second)
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	_, err = session.CurrentURL(context.Background())
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))
}

func TestSessionCapturePageStateAndStructuredExtraction(t *testing.T) {
	backend := &testBackend{
		currentURL: "https://example.com/page",
		text:       "   one   two   three   four   five   six   ",
		scriptFunc: func(script string) (any, error) {
			switch {
			case strings.Contains(script, "code: Array.from"):
				return map[string]any{
					"url":      "https://example.com/page",
					"title":    "Example Title",
					"headings": []string{"Intro"},
					"links":    []map[string]any{{"text": "Docs", "href": "https://example.com/docs"}},
					"inputs":   []map[string]any{{"name": "q", "type": "search", "placeholder": "Search"}},
					"buttons":  []string{"Submit"},
					"code":     []string{"fmt.Println(\"ok\")"},
				}, nil
			case strings.Contains(script, "document.links.length"):
				return map[string]any{
					"links":   int32(2),
					"forms":   float64(3),
					"inputs":  int64(4),
					"buttons": float32(5),
				}, nil
			case strings.Contains(script, "document.title"):
				return "  Example Title  ", nil
			default:
				return map[string]any{"ok": true}, nil
			}
		},
	}
	session := newTestSession(t, backend)

	state, err := session.CapturePageState(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://example.com/page", state.URL)
	require.Equal(t, "Example Title", state.Title)
	require.Equal(t, 2, state.LinkCount)
	require.Equal(t, 3, state.FormCount)
	require.Equal(t, 4, state.InputCount)
	require.Equal(t, 5, state.ButtonCount)
	require.Equal(t, "one two three four five six", state.Preview)
	require.WithinDuration(t, time.Now().UTC(), state.ObservedAt, time.Second)

	data, extraction, err := session.ExtractStructured(context.Background())
	require.NoError(t, err)
	require.NotNil(t, extraction)
	require.False(t, extraction.Truncated)
	require.Equal(t, "https://example.com/page", data.URL)
	require.Equal(t, "Example Title", data.Title)
	require.Equal(t, []string{"Intro"}, data.Headings)
	require.Equal(t, "Docs", data.Links[0].Text)
	require.Equal(t, "https://example.com/docs", data.Links[0].Href)
	require.Equal(t, "q", data.Inputs[0].Name)
	require.Equal(t, "search", data.Inputs[0].Type)
	require.Equal(t, "Search", data.Inputs[0].Placeholder)
	require.Equal(t, []string{"Submit"}, data.Buttons)
	require.Equal(t, []string{"fmt.Println(\"ok\")"}, data.Code)
	require.Contains(t, extraction.Content, `"title":"Example Title"`)
}

func TestSessionExtractionBudgetHelpers(t *testing.T) {
	budget := newTestBudget(100, map[string]float64{defaultBudgetCategory: 1.0})
	session := newTestSession(t, &testBackend{html: strings.Repeat("abcd", 20)}, func(cfg *SessionConfig) {
		cfg.Budget = budget
	})

	extraction, err := session.ExtractHTML(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, extraction.Content)
	initialRemaining := budget.GetRemainingBudget(defaultBudgetCategory)
	require.Less(t, initialRemaining, 100)

	extraction2, err := session.allocateExtraction("html", "xyz")
	require.NoError(t, err)
	require.Equal(t, "xyz", extraction2.Content)
	require.Equal(t, 99, budget.GetRemainingBudget(defaultBudgetCategory))
	require.Less(t, initialRemaining, 99)

	err = session.Close()
	require.NoError(t, err)
	require.Equal(t, 1, session.backend.(*testBackend).closed)
	require.Equal(t, 100, budget.GetRemainingBudget(defaultBudgetCategory))

	err = session.Close()
	require.NoError(t, err)
	require.Equal(t, 1, session.backend.(*testBackend).closed)

	closedSession := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.Budget = budget
	})
	closedSession.closed = true
	_, err = closedSession.allocateExtraction("text:#x", "value")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	emptyBudget := newTestBudget(10, map[string]float64{defaultBudgetCategory: 1.0})
	require.NoError(t, emptyBudget.Allocate(defaultBudgetCategory, 10, nil))
	exhaustedSession := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.Budget = emptyBudget
	})
	truncated, err := exhaustedSession.allocateExtraction("text:#x", "content")
	require.NoError(t, err)
	require.True(t, truncated.Truncated)
	require.Empty(t, truncated.Content)
	require.Equal(t, 0, truncated.FinalTokens)
}

func TestSessionNavigationHelpers(t *testing.T) {
	perms := &core.PermissionSet{
		Network: []core.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "allowed.example", Port: 443}},
	}
	manager, err := authorization.NewPermissionManager("", perms, nil, nil)
	require.NoError(t, err)

	session := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.PermissionManager = manager
		cfg.AgentID = "agent-1"
	})

	require.NoError(t, session.Navigate(context.Background(), "https://allowed.example/path"))
	require.Equal(t, "https://allowed.example/path", session.backend.(*testBackend).currentURL)

	denied := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.PermissionManager = manager
		cfg.AgentID = "agent-1"
	})
	err = denied.Navigate(context.Background(), "https://denied.example/path")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNavigationBlocked))

	blocked := newTestSession(t, &testBackend{})
	err = blocked.Navigate(context.Background(), "file:///etc/passwd")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrNavigationBlocked))

	invalid := newTestSession(t, &testBackend{})
	err = invalid.Navigate(context.Background(), "http://[::1")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrInvalidURL))

	missingHost := newTestSession(t, &testBackend{}, func(cfg *SessionConfig) {
		cfg.PermissionManager = manager
		cfg.AgentID = "agent-1"
	})
	err = missingHost.Navigate(context.Background(), "https:///path")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrInvalidURL))

}

func TestNavigationAndParsingHelpers(t *testing.T) {
	u := mustParseURL(t, "https://example.com:8443/path")
	protocol, port, needsNetwork, allowed := navigationTarget(u)
	require.Equal(t, "tcp", protocol)
	require.Equal(t, 8443, port)
	require.True(t, needsNetwork)
	require.True(t, allowed)

	u = mustParseURL(t, "about:blank")
	_, _, needsNetwork, allowed = navigationTarget(u)
	require.False(t, needsNetwork)
	require.True(t, allowed)

	u = mustParseURL(t, "about:srcdoc")
	_, _, needsNetwork, allowed = navigationTarget(u)
	require.False(t, needsNetwork)
	require.False(t, allowed)

	require.Equal(t, 443, defaultPort("https"))
	require.Equal(t, 80, defaultPort("ws"))
	require.Equal(t, 0, defaultPort("gopher"))
}

func TestHelperFunctions(t *testing.T) {
	require.Equal(t, "", truncateToTokens("abc", 0))
	require.Equal(t, "abc", truncateToTokens("abc", 1))
	require.Equal(t, "abcd", truncateToTokens("abcd", 1))
	require.Equal(t, "abcd", truncateToTokens("abcd", 2))
	require.Equal(t, "abcdefghi...", truncateToTokens("abcdefghijklmnop", 3))

	require.Equal(t, "one two three", compactPreview("one    two\nthree"))
	long := strings.Repeat("x", 300)
	require.Len(t, compactPreview(long), 243)

	require.Equal(t, 7, intFromAny(7))
	require.Equal(t, 8, intFromAny(int32(8)))
	require.Equal(t, 9, intFromAny(int64(9)))
	require.Equal(t, 10, intFromAny(float32(10)))
	require.Equal(t, 11, intFromAny(float64(11)))
	require.Equal(t, 0, intFromAny("nope"))

	data, err := normalizeStructuredPageData(map[string]any{"url": "https://example.com"})
	require.NoError(t, err)
	require.Equal(t, "https://example.com", data.URL)

	_, err = normalizeStructuredPageData(make(chan int))
	require.Error(t, err)

	decoded, err := decodeStructuredPageData(`{"url":"https://example.com","title":"Example"}`)
	require.NoError(t, err)
	require.Equal(t, "Example", decoded.Title)

	decoded, err = decodeStructuredPageData("   ")
	require.NoError(t, err)
	require.Empty(t, decoded.URL)

	_, err = decodeStructuredPageData("{")
	require.Error(t, err)
}

func TestSessionCloseWrapsBackendError(t *testing.T) {
	backend := &testBackend{closeErr: errors.New("close failed")}
	session := newTestSession(t, backend)

	err := session.Close()
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))
}

func TestSessionWrapsExplicitBackendErrors(t *testing.T) {
	backend := &testBackend{
		clickErr:      errors.New("click failed"),
		typeErr:       errors.New("type failed"),
		waitErr:       errors.New("wait failed"),
		currentURLErr: errors.New("current url failed"),
	}
	session := newTestSession(t, backend)

	err := session.Click(context.Background(), "#submit")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	err = session.Type(context.Background(), "#name", "lex")
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	err = session.WaitFor(context.Background(), WaitCondition{Type: WaitForLoad}, time.Second)
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))

	_, err = session.CurrentURL(context.Background())
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrUnknownOperation))
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
