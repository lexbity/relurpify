package agenttest

import (
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type browserFixtureServer struct {
	server *httptest.Server
	urls   map[string]string
}

type browserFixtureResponse struct {
	body        []byte
	contentType string
	status      int
	headers     map[string]string
}

func startBrowserFixtureServer(suite *Suite, targetWorkspace, workspace string, c CaseSpec) (*browserFixtureServer, error) {
	if len(c.BrowserFixtures) == 0 {
		return nil, nil
	}

	mux := http.NewServeMux()
	urls := make(map[string]string, len(c.BrowserFixtures))

	for name, fixture := range c.BrowserFixtures {
		routePath, err := normalizeBrowserFixturePath(name, fixture.Path)
		if err != nil {
			return nil, fmt.Errorf("browser fixture %q: %w", name, err)
		}
		response, err := loadBrowserFixtureResponse(suite, targetWorkspace, workspace, fixture)
		if err != nil {
			return nil, fmt.Errorf("browser fixture %q: %w", name, err)
		}

		responseCopy := response
		mux.HandleFunc(routePath, func(w http.ResponseWriter, r *http.Request) {
			for key, value := range responseCopy.headers {
				w.Header().Set(key, value)
			}
			if responseCopy.contentType != "" {
				w.Header().Set("Content-Type", responseCopy.contentType)
			}
			status := responseCopy.status
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(responseCopy.body)
		})
		urls[name] = routePath
	}

	server := httptest.NewServer(mux)
	resolvedURLs := make(map[string]string, len(urls))
	for name, routePath := range urls {
		resolvedURLs[name] = server.URL + routePath
	}
	return &browserFixtureServer{
		server: server,
		urls:   resolvedURLs,
	}, nil
}

func (s *browserFixtureServer) Close() {
	if s == nil || s.server == nil {
		return
	}
	s.server.Close()
}

func (s *browserFixtureServer) InjectTask(task *core.Task) {
	if s == nil || task == nil {
		return
	}
	localURLs := make(map[string]string, len(s.urls))
	exposedURLs := make(map[string]string, len(s.urls))
	for name, rawURL := range s.urls {
		localURLs[name] = rawURL
		exposedURLs[name] = rewriteLoopbackURLForBrowser(rawURL)
	}
	baseURL := rewriteLoopbackURLForBrowser(s.server.URL)

	if task.Context == nil {
		task.Context = make(map[string]any)
	}
	task.Context["browser_fixtures"] = map[string]any{
		"base_url":       baseURL,
		"urls":           exposedURLs,
		"local_base_url": s.server.URL,
		"local_urls":     localURLs,
	}

	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	task.Metadata["browser.fixture.base_url"] = baseURL
	task.Metadata["browser.fixture.local_base_url"] = s.server.URL
	for name, rawURL := range s.urls {
		safeName := sanitizeName(name)
		task.Metadata[fmt.Sprintf("browser.fixture.%s.url", safeName)] = rewriteLoopbackURLForBrowser(rawURL)
		task.Metadata[fmt.Sprintf("browser.fixture.%s.local_url", safeName)] = rawURL
	}
}

func rewriteLoopbackURLForBrowser(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return rawURL
	}
	host := parsed.Hostname()
	if host == "" {
		return rawURL
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		parsed.Host = "host.docker.internal"
		if port := parsed.Port(); port != "" {
			parsed.Host += ":" + port
		}
		return parsed.String()
	default:
		return rawURL
	}
}

func loadBrowserFixtureResponse(suite *Suite, targetWorkspace, workspace string, fixture BrowserFixtureSpec) (browserFixtureResponse, error) {
	body := []byte(fixture.Content)
	if strings.TrimSpace(fixture.File) != "" {
		path, err := resolveBrowserFixtureFile(suite, targetWorkspace, workspace, fixture.File)
		if err != nil {
			return browserFixtureResponse{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return browserFixtureResponse{}, err
		}
		body = data
	}
	contentType := strings.TrimSpace(fixture.ContentType)
	if contentType == "" && strings.TrimSpace(fixture.File) != "" {
		contentType = mime.TypeByExtension(filepath.Ext(fixture.File))
	}
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}

	headers := make(map[string]string, len(fixture.Headers))
	for key, value := range fixture.Headers {
		headers[key] = value
	}
	return browserFixtureResponse{
		body:        body,
		contentType: contentType,
		status:      fixture.Status,
		headers:     headers,
	}, nil
}

func resolveBrowserFixtureFile(suite *Suite, targetWorkspace, workspace, file string) (string, error) {
	resolved := file
	if suite != nil {
		resolved = suite.ResolvePath(file)
	}
	resolved = resolveAgainstWorkspace(targetWorkspace, resolved, file)
	if workspace != "" && targetWorkspace != "" && workspace != targetWorkspace {
		mapped := mapTargetPathToWorkspace(resolved, targetWorkspace, workspace)
		if _, err := os.Stat(mapped); err == nil {
			resolved = mapped
		}
	}
	if _, err := os.Stat(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func normalizeBrowserFixturePath(name, rawPath string) (string, error) {
	normalized := strings.TrimSpace(rawPath)
	if normalized == "" {
		if strings.EqualFold(name, "root") || strings.EqualFold(name, "index") {
			return "/", nil
		}
		normalized = "/" + sanitizeName(name)
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	normalized = path.Clean(normalized)
	if normalized == "." {
		normalized = "/"
	}
	if !strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("path must stay within server root")
	}
	return normalized, nil
}
