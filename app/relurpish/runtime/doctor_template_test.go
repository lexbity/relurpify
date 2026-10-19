package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

func templateProbeDef(endpoint string) *model.ResolvedProvider {
	return &model.ResolvedProvider{Name: "ollama", Kind: "ollama", Endpoint: endpoint}
}

// A served template without a tool block must produce a non-blocking,
// degraded warning when the profile intent is native: tool decisions will
// surface as text instead of structured tool calls.
func TestProbeModelToolTemplateWarnsOnMissingToolSupport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"template":"{{ .Prompt }}"}`))
	}))
	defer server.Close()

	dep := probeModelToolTemplate(context.Background(), templateProbeDef(server.URL), "gemma4:e4b")
	if dep == nil {
		t.Fatal("expected a degraded model_tool_template dependency, got nil")
	}
	if dep.Blocking {
		t.Error("template warning must be non-blocking")
	}
	if !dep.Degraded || dep.Required {
		t.Errorf("expected degraded=true, required=false, got %+v", dep)
	}
	if dep.Name != "model_tool_template" {
		t.Errorf("expected dependency name model_tool_template, got %q", dep.Name)
	}
}

// A tool-capable template is healthy: no dependency row, no noise.
func TestProbeModelToolTemplateSilentOnToolCapableTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"template":"{{ if .Tools }}{{ .Tools }}{{ end }}{{ .Prompt }}"}`))
	}))
	defer server.Close()

	if dep := probeModelToolTemplate(context.Background(), templateProbeDef(server.URL), "gemma4:e4b"); dep != nil {
		t.Fatalf("expected nil for a tool-capable template, got %+v", dep)
	}
}

// The probe is ollama-specific and skips offline-synthetic models.
func TestProbeModelToolTemplateSkipsNonApplicable(t *testing.T) {
	if dep := probeModelToolTemplate(context.Background(),
		&model.ResolvedProvider{Name: "lmstudio", Kind: "lmstudio", Endpoint: "http://127.0.0.1:9"}, "m"); dep != nil {
		t.Fatalf("expected nil for a non-ollama provider, got %+v", dep)
	}
	if dep := probeModelToolTemplate(context.Background(),
		templateProbeDef("http://127.0.0.1:9"), "offline-synthetic"); dep != nil {
		t.Fatalf("expected nil for the offline-synthetic model, got %+v", dep)
	}
}
