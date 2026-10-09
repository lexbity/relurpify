package runtime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

func readySnapshot() StatusSnapshot {
	return StatusSnapshot{
		Environment: EnvironmentReport{
			Workspace: "/workspaces/demo",
			Agent:     "euclo",
			Inference: InferenceBackendReport{
				Provider:      "ollama",
				Endpoint:      "http://127.0.0.1:11434",
				State:         llm.BackendHealthReady,
				SelectedModel: "gemma4:e4b",
			},
			Sandbox: SandboxReport{Verified: true},
		},
		Ready:               true,
		ServerActive:        true,
		ManifestFingerprint: "abcd1234",
		RecipesReady:        true,
	}
}

func TestStatusRenderTextReady(t *testing.T) {
	var out bytes.Buffer
	readySnapshot().RenderText(&out)
	text := out.String()
	for _, want := range []string{
		"workspace:  /workspaces/demo",
		"agent:      euclo",
		"config:     abcd1234",
		"provider:   ollama (http://127.0.0.1:11434)",
		"model:      gemma4:e4b state=ready",
		"sandbox:    verified=true",
		"services:   server-active=true",
		"recipes:    ready=true",
		"ready:      true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered status missing %q:\n%s", want, text)
		}
	}
}

func TestStatusRenderTextDegradedShowsDoctorHint(t *testing.T) {
	snap := readySnapshot()
	snap.Ready = false
	snap.RecipesReady = false
	snap.RecipesError = "no canonical recipes found"
	snap.Environment.Inference.State = llm.BackendHealthUnhealthy
	snap.Environment.Inference.SelectedModel = ""
	snap.ManifestFingerprint = ""

	var out bytes.Buffer
	snap.RenderText(&out)
	text := out.String()
	for _, want := range []string{
		"ready:      false",
		"model:      none state=unhealthy",
		"config:     none",
		"recipes:    ready=false",
		"no canonical recipes found",
		"relurpish doctor --fix",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered status missing %q:\n%s", want, text)
		}
	}
}

func TestStatusRenderTextShowsKnowledgeDegraded(t *testing.T) {
	snap := readySnapshot()
	snap.KnowledgeDegraded = true
	snap.KnowledgeReason = "store degraded: engine wedged"

	var out bytes.Buffer
	snap.RenderText(&out)
	text := out.String()
	if !strings.Contains(text, "knowledge:  degraded: store degraded: engine wedged") {
		t.Fatalf("rendered status missing knowledge health line:\n%s", text)
	}
}

func TestStatusRenderJSONSchema(t *testing.T) {
	var out bytes.Buffer
	if err := readySnapshot().RenderJSON(&out); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var decoded struct {
		Environment struct {
			Workspace string `json:"Workspace"`
			Inference struct {
				State string `json:"State"`
			} `json:"Inference"`
		} `json:"Environment"`
		Ready bool `json:"Ready"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("snapshot JSON does not parse: %v", err)
	}
	if !decoded.Ready || decoded.Environment.Workspace != "/workspaces/demo" {
		t.Fatalf("decoded snapshot = %+v", decoded)
	}
}
