package browser

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeBackend struct {
	currentURL     string
	text           string
	html           string
	accessibility  string
	scriptResult   any
	scriptFunc     func(string) (any, error)
	screenshotData []byte
	clicks         []string
	types          []struct {
		selector string
		text     string
	}
	waits  []WaitCondition
	closed int
}

func (f *fakeBackend) Navigate(_ context.Context, url string) error {
	f.currentURL = url
	return nil
}

func (f *fakeBackend) Click(_ context.Context, selector string) error {
	f.clicks = append(f.clicks, selector)
	return nil
}

func (f *fakeBackend) Type(_ context.Context, selector, text string) error {
	f.types = append(f.types, struct {
		selector string
		text     string
	}{selector: selector, text: text})
	return nil
}

func (f *fakeBackend) GetText(context.Context, string) (string, error) { return f.text, nil }
func (f *fakeBackend) GetAccessibilityTree(context.Context) (string, error) {
	return f.accessibility, nil
}
func (f *fakeBackend) GetHTML(context.Context) (string, error) { return f.html, nil }
func (f *fakeBackend) ExecuteScript(_ context.Context, script string) (any, error) {
	if f.scriptFunc != nil {
		return f.scriptFunc(script)
	}
	return f.scriptResult, nil
}
func (f *fakeBackend) Screenshot(context.Context) ([]byte, error) { return f.screenshotData, nil }
func (f *fakeBackend) WaitFor(_ context.Context, condition WaitCondition, _ time.Duration) error {
	f.waits = append(f.waits, condition)
	return nil
}
func (f *fakeBackend) CurrentURL(context.Context) (string, error) { return f.currentURL, nil }
func (f *fakeBackend) Close() error {
	f.closed++
	return nil
}

func runBackendContractTests(t *testing.T, backend Backend) {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, backend.Navigate(ctx, "https://example.com"))
	currentURL, err := backend.CurrentURL(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com", currentURL)

	require.NoError(t, backend.Click(ctx, "#submit"))
	require.NoError(t, backend.Type(ctx, "#name", "lex"))

	text, err := backend.GetText(ctx, "#result")
	require.NoError(t, err)
	require.NotEmpty(t, text)

	html, err := backend.GetHTML(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, html)

	ax, err := backend.GetAccessibilityTree(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ax)

	result, err := backend.ExecuteScript(ctx, "return 1")
	require.NoError(t, err)
	require.NotNil(t, result)

	screenshot, err := backend.Screenshot(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, screenshot)

	require.NoError(t, backend.WaitFor(ctx, WaitCondition{Type: WaitForSelector, Selector: "#result"}, time.Second))
	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
}

func TestBackendContractWithFakeBackend(t *testing.T) {
	runBackendContractTests(t, &fakeBackend{
		text:           "hello",
		html:           "<html><body>Hello</body></html>",
		accessibility:  "{\"role\":\"document\"}",
		scriptResult:   map[string]any{"ok": true},
		screenshotData: []byte{0x89, 0x50, 0x4e, 0x47},
	})
}
