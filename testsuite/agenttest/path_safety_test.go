//go:build live
// +build live

package agenttest

import (
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
	_, err := applySetup(root, root, SetupSpec{
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
	err := applyWorkspaceFiles(root, root, []SetupFileSpec{{
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
