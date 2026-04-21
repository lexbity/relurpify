package rewoo

import (
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type referenceTestItem struct {
	ref     core.ContextReference
	summary string
}

func (r *referenceTestItem) TokenCount() int         { return len(r.summary) / 4 }
func (r *referenceTestItem) RelevanceScore() float64 { return 1.0 }
func (r *referenceTestItem) Priority() int           { return 1 }
func (r *referenceTestItem) Compress() (core.ContextItem, error) {
	return r, nil
}
func (r *referenceTestItem) Type() core.ContextItemType { return core.ContextTypeRetrieval }
func (r *referenceTestItem) Age() time.Duration         { return time.Minute }
func (r *referenceTestItem) References() []core.ContextReference {
	return []core.ContextReference{r.ref}
}
func (r *referenceTestItem) HasInlinePayload() bool { return r.summary != "" }

func TestPlannerContextBlockIncludesSharedAndManagedReferences(t *testing.T) {
	shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(8000), &core.SimpleSummarizer{})
	if _, err := shared.AddFile("src/app.go", "package app\nfunc Run() {}\n", "go", core.DetailSummary); err != nil {
		t.Fatalf("add file: %v", err)
	}
	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Budget:         core.NewContextBudget(8000),
		ContextManager: contextmgr.NewContextManager(core.NewContextBudget(8000)),
	}, nil)
	if err := policy.ContextManager.AddItem(&core.MemoryContextItem{
		Source:  "memory:bug",
		Summary: "remember previous timeout issue",
		Reference: &core.ContextReference{
			Kind:   core.ContextReferenceRuntimeMemory,
			ID:     "bug",
			Detail: "query-results",
		},
		LastAccessed: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add memory item: %v", err)
	}
	if err := policy.ContextManager.AddItem(&core.RetrievalContextItem{
		Source:  "retrieval_evidence",
		Summary: "API contract requires auth header",
		Reference: &core.ContextReference{
			Kind:   core.ContextReferenceRetrievalEvidence,
			ID:     "chunk-1",
			URI:    "guide.md",
			Detail: "packed",
		},
		LastAccessed: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add retrieval item: %v", err)
	}

	block := plannerContextBlock(&core.Task{
		Context: map[string]any{
			"workflow_retrieval": "Known API constraint",
			"workflow_retrieval_payload": map[string]any{
				"summary": "Structured workflow retrieval summary",
			},
		},
	}, shared, policy)
	if !strings.Contains(block, "Structured workflow retrieval summary") {
		t.Fatalf("expected workflow retrieval payload summary in planner context, got %q", block)
	}
	if !strings.Contains(block, "Shared file context:") || !strings.Contains(block, "src/app.go") {
		t.Fatalf("expected shared file reference in planner context, got %q", block)
	}
	if !strings.Contains(block, "Reference context:") || !strings.Contains(block, "guide.md") || !strings.Contains(block, "bug") {
		t.Fatalf("expected managed references in planner context, got %q", block)
	}
}

func TestSharedContextPromptBlockUsesReferenceOnlyFallback(t *testing.T) {
	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Budget:         core.NewContextBudget(8000),
		ContextManager: contextmgr.NewContextManager(core.NewContextBudget(8000)),
	}, nil)
	if err := policy.ContextManager.AddItem(&referenceTestItem{
		ref: core.ContextReference{
			Kind:   core.ContextReferenceWorkflowArtifact,
			ID:     "artifact-1",
			URI:    "artifact://workflow/result",
			Detail: "metadata",
		},
	}); err != nil {
		t.Fatalf("add test item: %v", err)
	}

	block := sharedContextPromptBlock(nil, policy)
	if !strings.Contains(block, "artifact://workflow/result") || !strings.Contains(block, "reference only") {
		t.Fatalf("expected reference-only fallback, got %q", block)
	}
}

func TestContextItemPromptSummaryAndTruncationBranches(t *testing.T) {
	if got := contextItemPromptSummary(nil); got != "" {
		t.Fatalf("expected empty summary for nil item, got %q", got)
	}

	fileSummary := contextItemPromptSummary(&core.FileContextItem{Summary: "  file summary  "})
	if fileSummary != "file summary" {
		t.Fatalf("unexpected file summary: %q", fileSummary)
	}

	fileContent := contextItemPromptSummary(&core.FileContextItem{Content: "  file content  "})
	if fileContent != "file content" {
		t.Fatalf("unexpected file content summary: %q", fileContent)
	}

	memSummary := contextItemPromptSummary(&core.MemoryContextItem{Summary: "  memory summary  "})
	if memSummary != "memory summary" {
		t.Fatalf("unexpected memory summary: %q", memSummary)
	}

	retrievalContent := contextItemPromptSummary(&core.RetrievalContextItem{Content: "  retrieval content  "})
	if retrievalContent != "retrieval content" {
		t.Fatalf("unexpected retrieval summary: %q", retrievalContent)
	}

	if got := truncatePromptSnippet("   ", 4); got != "" {
		t.Fatalf("expected empty truncated snippet, got %q", got)
	}
	if got := truncatePromptSnippet("abc", 10); got != "abc" {
		t.Fatalf("expected passthrough snippet, got %q", got)
	}
	if got := truncatePromptSnippet("abcdef", 3); got != "abc..." {
		t.Fatalf("expected truncated snippet, got %q", got)
	}
	if got := truncatePromptSnippet("abcdef", 0); got != "abcdef" {
		t.Fatalf("expected no truncation when limit <= 0, got %q", got)
	}
}
