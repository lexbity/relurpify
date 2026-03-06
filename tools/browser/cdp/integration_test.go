package cdp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/tools/browser"
	"github.com/stretchr/testify/require"
)

func TestChromiumBackendLocalhostFlow(t *testing.T) {
	chromiumPath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<body>
  <input id="name" />
  <button id="submit" onclick="document.querySelector('#result').textContent = document.querySelector('#name').value || 'clicked';">Submit</button>
  <div id="result">idle</div>
</body>
</html>`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runLocalhostFlow(t, ctx, chromiumPath)
}

func TestChromiumBackendCloseCleansUpProcessAndProfile(t *testing.T) {
	chromiumPath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	backend, err := New(ctx, Config{
		ExecutablePath: chromiumPath,
		Headless:       true,
	})
	require.NoError(t, err)
	cmd := backend.process
	userData := backend.userData

	require.NoError(t, backend.Close())
	require.NotNil(t, cmd.ProcessState)
	require.True(t, cmd.ProcessState.Exited())
	_, err = os.Stat(userData)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestChromiumBackendRepeatedLocalhostFlow(t *testing.T) {
	if testing.Short() || os.Getenv("RELURPIFY_BROWSER_STRESS") == "" {
		t.Skip("set RELURPIFY_BROWSER_STRESS=1 to run repeated browser stress tests")
	}
	chromiumPath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		runLocalhostFlow(t, ctx, chromiumPath)
		cancel()
	}
}

func runLocalhostFlow(t *testing.T, ctx context.Context, chromiumPath string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<body>
  <input id="name" />
  <button id="submit" onclick="document.querySelector('#result').textContent = document.querySelector('#name').value || 'clicked';">Submit</button>
  <div id="result">idle</div>
</body>
</html>`))
	}))
	defer server.Close()
	backend, err := New(ctx, Config{
		ExecutablePath: chromiumPath,
		Headless:       true,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, backend.Close())
	}()

	require.NoError(t, backend.Navigate(ctx, server.URL))
	require.NoError(t, backend.Type(ctx, "#name", "lex"))
	require.NoError(t, backend.Click(ctx, "#submit"))
	require.NoError(t, backend.WaitFor(ctx, browser.WaitCondition{Type: browser.WaitForText, Selector: "#result", Text: "lex"}, 5*time.Second))

	text, err := backend.GetText(ctx, "#result")
	require.NoError(t, err)
	require.Equal(t, "lex", text)

	html, err := backend.GetHTML(ctx)
	require.NoError(t, err)
	require.Contains(t, html, "id=\"submit\"")

	currentURL, err := backend.CurrentURL(ctx)
	require.NoError(t, err)
	require.Contains(t, currentURL, server.URL)

	screenshot, err := backend.Screenshot(ctx)
	require.NoError(t, err)
	require.Greater(t, len(screenshot), 4)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, screenshot[:4])
}
