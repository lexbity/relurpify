package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathWithinRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := resolvePathWithin(root, "../escape.txt"); err == nil {
		t.Fatal("expected traversal to fail")
	}
}

func TestApplySetupRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	_, err := applySetup(root, SetupSpec{
		Files: []SetupFileSpec{{
			Path:    "../escape.txt",
			Content: "nope",
		}},
	}, false, nil)
	if err == nil {
		t.Fatal("expected escaping setup path to fail")
	}
}

func TestApplyWorkspaceFilesRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	err := applyWorkspaceFiles(root, []SetupFileSpec{{
		Path:    "../escape.txt",
		Content: "nope",
	}})
	if err == nil {
		t.Fatal("expected escaping overlay path to fail")
	}
}

func TestResolveCaseExecutionRejectsEscapingTapePath(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), "smoke", "model")
	workspace := filepath.Join(t.TempDir(), "workspace")
	targetWorkspace := t.TempDir()
	suite := &Suite{
		SourcePath: filepath.Join(targetWorkspace, "testsuite.yaml"),
		Spec: SuiteSpec{
			Recording: RecordingSpec{Mode: "record", Tape: "../escape.jsonl"},
		},
	}

	_, err := resolveCaseExecution(suite, CaseSpec{Name: "smoke"}, ModelSpec{Name: "suite-model"}, "manifest-model", RunOptions{}, layout, targetWorkspace, workspace)
	if err == nil {
		t.Fatal("expected escaping tape path to fail")
	}
}

func TestResolveBrowserFixtureFileRejectsEscapingTargetWorkspace(t *testing.T) {
	targetWorkspace := t.TempDir()
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "page.html")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	suite := &Suite{
		SourcePath: filepath.Join(targetWorkspace, "suite.yaml"),
	}
	if _, err := resolveBrowserFixtureFile(suite, targetWorkspace, workspace, outsideFile); err == nil {
		t.Fatal("expected fixture outside target workspace to fail")
	}
}

func TestResolveBrowserFixtureFileRejectsEscapingSuiteRelativePath(t *testing.T) {
	targetWorkspace := t.TempDir()
	workspace := t.TempDir()
	suiteDir := filepath.Join(targetWorkspace, "testsuite", "agenttests")
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(targetWorkspace, "fixtures")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "page.html")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	suite := &Suite{
		SourcePath: filepath.Join(suiteDir, "suite.yaml"),
	}
	if _, err := resolveBrowserFixtureFile(suite, targetWorkspace, workspace, "../fixtures/page.html"); err == nil {
		t.Fatal("expected suite-relative fixture traversal to fail")
	}
}
