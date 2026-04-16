package pretask

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestDeferralLoaderRunLoadsOnlyOpenIssues(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "relurpify_cfg", "artifacts", "euclo", "deferred")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeDeferredIssue := func(name, status, title string) {
		t.Helper()
		content := "---\n" +
			"issue_id: \"" + name + "\"\n" +
			"workflow_id: \"wf-1\"\n" +
			"kind: \"ambiguity\"\n" +
			"severity: \"high\"\n" +
			"status: \"" + status + "\"\n" +
			"title: \"" + title + "\"\n" +
			"summary: \"" + title + " summary\"\n" +
			"---\n\n# " + title + "\n"
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
			t.Fatalf("write issue: %v", err)
		}
	}
	writeDeferredIssue("open-1", string(eucloruntime.DeferredIssueStatusOpen), "Open issue")
	writeDeferredIssue("resolved-1", string(eucloruntime.DeferredIssueStatusResolved), "Resolved issue")

	state := core.NewContext()
	loader := DeferralLoader{WorkspaceDir: workspace}
	if err := loader.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rawIssues, ok := state.Get("euclo.prior_deferred_issues")
	if !ok {
		t.Fatal("expected prior deferred issues in state")
	}
	issues, ok := rawIssues.([]eucloruntime.DeferredExecutionIssue)
	if !ok {
		t.Fatalf("unexpected issue type: %#v", rawIssues)
	}
	if len(issues) != 1 || issues[0].IssueID != "open-1" {
		t.Fatalf("expected one open issue, got %#v", issues)
	}

	rawKnowledge, ok := state.Get("context.knowledge_items")
	if !ok {
		t.Fatal("expected knowledge items in state")
	}
	knowledge, ok := rawKnowledge.([]any)
	if !ok {
		t.Fatalf("unexpected knowledge type: %#v", rawKnowledge)
	}
	if len(knowledge) != 1 {
		t.Fatalf("expected one knowledge item, got %#v", knowledge)
	}
	if item, ok := knowledge[0].(KnowledgeEvidenceItem); !ok {
		t.Fatalf("unexpected knowledge entry type: %#v", knowledge[0])
	} else if got := item.Source; got != EvidenceSource("deferred_issue") {
		t.Fatalf("unexpected knowledge source: %q", got)
	}
}

func TestDeferralLoaderRunWithEmptyWorkspaceIsNoop(t *testing.T) {
	state := core.NewContext()
	loader := DeferralLoader{}
	if err := loader.Run(context.Background(), state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := state.Get("euclo.prior_deferred_issues"); ok {
		t.Fatal("expected no prior deferred issues")
	}
}
