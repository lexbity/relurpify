package agenttest

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSuiteValidateRejectsEmptyBrowserFixture(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "browser"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "browser-case",
				Prompt: "open the fixture",
				BrowserFixtures: map[string]BrowserFixtureSpec{
					"home": {},
				},
			}},
		},
	}

	err := suite.Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "browser fixture[home] missing file or content")
}

func TestBrowserFixtureServerServesRoutesAndInjectsTask(t *testing.T) {
	workspace := t.TempDir()
	fixtureDir := filepath.Join(workspace, "fixtures")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixtureDir, "page.html"), []byte("<html><body>fixture page</body></html>"), 0o644))

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "browser-suite.yaml"),
	}
	caseSpec := CaseSpec{
		Name:   "browser-case",
		Prompt: "use the browser fixtures",
		BrowserFixtures: map[string]BrowserFixtureSpec{
			"root": {
				Path: "/",
				File: "fixtures/page.html",
			},
			"json_data": {
				Path:        "/data.json",
				Content:     `{"ok":true}`,
				ContentType: "application/json",
			},
		},
	}

	server, err := startBrowserFixtureServer(suite, workspace, workspace, caseSpec)
	require.NoError(t, err)
	require.NotNil(t, server)
	defer server.Close()

	resp, err := http.Get(server.urls["root"])
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	require.Contains(t, string(body), "fixture page")

	jsonResp, err := http.Get(server.urls["json_data"])
	require.NoError(t, err)
	defer jsonResp.Body.Close()
	jsonBody, err := io.ReadAll(jsonResp.Body)
	require.NoError(t, err)
	require.Equal(t, "application/json", jsonResp.Header.Get("Content-Type"))
	require.JSONEq(t, `{"ok":true}`, string(jsonBody))

	task := &core.Task{
		Context:  map[string]any{"existing": "value"},
		Metadata: map[string]string{"existing": "value"},
	}
	server.InjectTask(task)

	fixtures, ok := task.Context["browser_fixtures"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, server.server.URL, fixtures["base_url"])

	urls, ok := fixtures["urls"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, server.urls["root"], urls["root"])
	require.Equal(t, server.urls["json_data"], urls["json_data"])

	require.Equal(t, server.server.URL, task.Metadata["browser.fixture.base_url"])
	require.Equal(t, server.urls["json_data"], task.Metadata["browser.fixture.json_data.url"])
}
