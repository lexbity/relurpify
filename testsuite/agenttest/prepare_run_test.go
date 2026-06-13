//go:build live
// +build live

package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	agent_yaml    = "agent.yaml"
	hello         = "hello"
	relurpify_cfg = "relurpify_cfg"
)

func TestPrepareRunWritesDescriptor(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, relurpify_cfg, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
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
`), fs.PublicFileMode); err != nil { // public: test manifest
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, agent_yaml)),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: ollama,
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, relurpify_cfg, "test_run", run1)
	prepared, err := PrepareRun(suite, CaseSpec{Name: smoke, Prompt: hello}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, run1)
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
	manifestPath := filepath.Join(workspace, relurpify_cfg, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
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
`), fs.PublicFileMode); err != nil { // public: test manifest
		t.Fatal(err)
	}
	suite := &Suite{
		SourcePath: filepath.Join(workspace, "suite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, agent_yaml)),
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Provider: ollama,
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}

	runRoot := filepath.Join(workspace, relurpify_cfg, "test_run", run1)
	prepared, err := PrepareRun(suite, CaseSpec{
		Name:   smoke,
		Prompt: hello,
		Setup: SetupSpec{
			Files: []SetupFileSpec{{
				Path:    "testsuite/agenttest_fixtures/gosuite/hello/hello.go",
				Content: "package hello\n\nfunc Hello() string {\n  return \"hello\"\n}\n",
			}},
		},
	}, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, run1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Descriptor.DerivedWorkspaceRoot, "testsuite", "agenttest_fixtures", "gosuite", hello, "hello.go")); err != nil {
		t.Fatalf("expected case setup file in derived workspace: %v", err)
	}
}
