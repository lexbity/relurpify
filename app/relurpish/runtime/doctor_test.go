package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func writeMinimalDoctorWorkspace(t *testing.T, workspace string, providerYAMLs map[string]string) {
	t.Helper()
	dirs := []string{
		filepath.Join(workspace, "relurpify_cfg", "security"),
		filepath.Join(workspace, "relurpify_cfg", "model", "provider"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, fs.PublicDirMode); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	policies := map[string]string{
		"localtool.policy.yaml":       "schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: allow\n",
		"shell.policy.yaml":           "schema: relurpify/policy/shell/v1\nrules: []\n",
		"sandbox.policy.yaml":         "schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n",
		"workspaceingestion.policy.yaml": "schema: relurpify/policy/ingestion/v1\nrules: []\n",
	}
	for name, content := range policies {
		if err := os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "security", name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for name, content := range providerYAMLs {
		if err := os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "model", "provider", name), []byte(content), 0o600); err != nil {
			t.Fatalf("write provider %s: %v", name, err)
		}
	}
}

func TestDoctorReport_ProvidersBlockPopulated(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml": "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
	})

	cfg := Config{
		Workspace:          workspace,
		InferenceProvider:  "ollama",
		InferenceEndpoint:  "http://localhost:11434",
	}

	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	if len(report.Providers) == 0 {
		t.Fatal("expected at least one provider in report.Providers")
	}
	var ollamaFound bool
	for _, p := range report.Providers {
		if p.Name == "ollama" {
			ollamaFound = true
			if p.Kind != "ollama" {
				t.Fatalf("ollama provider kind = %q, want %q", p.Kind, "ollama")
			}
			break
		}
	}
	if !ollamaFound {
		t.Fatal("expected ollama provider in report")
	}
}

func TestDoctorReport_SelectedProviderMarked(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml":  "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
		"lmstudio.provider.yaml": "schema: relurpify/model/provider/v1\nname: lmstudio\nendpoint: http://localhost:1234\nkind: lmstudio\n",
	})
	cfg := Config{
		Workspace:          workspace,
		InferenceProvider:  "lmstudio",
		InferenceEndpoint:  "http://localhost:1234",
	}
	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	var lmstudioSelected, ollamaSelected bool
	for _, p := range report.Providers {
		if p.Name == "lmstudio" && p.Selected {
			lmstudioSelected = true
		}
		if p.Name == "ollama" && !p.Selected {
			ollamaSelected = true
		}
	}
	if !lmstudioSelected {
		t.Fatal("expected lmstudio to be marked as selected")
	}
	if !ollamaSelected {
		t.Fatal("expected ollama to not be marked as selected")
	}
}

func TestDoctorReport_NoInferenceBackendDep(t *testing.T) {
	workspace := t.TempDir()
	writeMinimalDoctorWorkspace(t, workspace, map[string]string{
		"ollama.provider.yaml": "schema: relurpify/model/provider/v1\nname: ollama\nendpoint: http://localhost:11434\nkind: ollama\n",
	})
	cfg := Config{
		Workspace:          workspace,
		InferenceProvider:  "ollama",
		InferenceEndpoint:  "http://localhost:11434",
	}
	report := BuildDoctorReport(context.Background(), cfg, config.Secrets{})
	for _, dep := range report.Dependencies {
		if dep.Name == "inference_backend" {
			t.Fatal("inference_backend dependency should not appear in Dependencies (shown in dedicated block)")
		}
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "a"}, []string{"a"}},
		{[]string{"a", "b", "a"}, []string{"a", "b"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		got := uniqueStrings(tt.input)
		if len(got) != len(tt.want) {
			t.Fatalf("uniqueStrings(%v) = %v (len %d), want len %d", tt.input, got, len(got), len(tt.want))
		}
		for i, v := range got {
			if v != tt.want[i] {
				t.Fatalf("uniqueStrings(%v) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}
