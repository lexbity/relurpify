package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/manifest"
)

func TestPrepareRunWritesDescriptor(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ${workspace}/**
        justification: read workspace
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(manifest.DirName, "agent.manifest.yaml")),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-1")
	prepared, err := PrepareRun(suite, CaseSpec{Name: "smoke", Prompt: "hello"}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if prepared == nil || prepared.Descriptor == nil {
		t.Fatal("expected prepared descriptor")
	}
	if _, err := os.Stat(prepared.Artifacts.DescriptorPath()); err != nil {
		t.Fatalf("descriptor not written: %v", err)
	}
	if prepared.Descriptor.SetupDir == "" || prepared.Descriptor.ExecutionDir == "" {
		t.Fatalf("expected run-scoped directories, got %+v", prepared.Descriptor)
	}
}

func TestPrepareRunMaterializesCaseSetupFiles(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ${workspace}/**
        justification: read workspace
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(manifest.DirName, "agent.manifest.yaml")),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-1")
	prepared, err := PrepareRun(suite, CaseSpec{
		Name:   "smoke",
		Prompt: "hello",
		Setup: SetupSpec{
			Files: []SetupFileSpec{{
				Path:    "testsuite/agenttest_fixtures/gosuite/hello/hello.go",
				Content: "package hello\n\nfunc Hello() string {\n  return \"hello\"\n}\n",
			}},
		},
	}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Descriptor.DerivedWorkspaceRoot, "testsuite", "agenttest_fixtures", "gosuite", "hello", "hello.go")); err != nil {
		t.Fatalf("expected case setup file in derived workspace: %v", err)
	}
}
