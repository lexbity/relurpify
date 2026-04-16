package pretask

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// DeferralLoader loads deferred issues from the workspace before the main
// context enrichment pipeline runs.
type DeferralLoader struct {
	WorkspaceDir string
}

// ID identifies the loader for registration and testing.
func (d DeferralLoader) ID() string {
	return "euclo:deferrals.load"
}

// Run loads unresolved deferred issues and seeds them into state.
func (d DeferralLoader) Run(_ context.Context, state *core.Context) error {
	if state == nil {
		return nil
	}
	workspaceDir := strings.TrimSpace(d.WorkspaceDir)
	if workspaceDir == "" {
		return nil
	}
	issues := eucloruntime.LoadDeferredIssuesFromWorkspace(workspaceDir)
	if len(issues) == 0 {
		state.Set("euclo.prior_deferred_issues", []eucloruntime.DeferredExecutionIssue{})
		state.Set("context.knowledge_items", []KnowledgeEvidenceItem{})
		return nil
	}

	openIssues := make([]eucloruntime.DeferredExecutionIssue, 0, len(issues))
	knowledgeItems := make([]KnowledgeEvidenceItem, 0, len(issues))
	for _, issue := range issues {
		if !isOpenDeferredIssue(issue) {
			continue
		}
		openIssues = append(openIssues, issue)
		knowledgeItems = append(knowledgeItems, ContextKnowledgeItem{
			Source:  "deferred_issue",
			Content: firstNonEmpty(strings.TrimSpace(issue.Summary), strings.TrimSpace(issue.Title)),
			Tags: []string{
				strings.TrimSpace(string(issue.Kind)),
				strings.TrimSpace(string(issue.Severity)),
			},
		}.toKnowledgeEvidenceItem(issue))
	}

	sort.SliceStable(openIssues, func(i, j int) bool {
		if severityRank(string(openIssues[i].Severity)) != severityRank(string(openIssues[j].Severity)) {
			return severityRank(string(openIssues[i].Severity)) < severityRank(string(openIssues[j].Severity))
		}
		return openIssues[i].IssueID < openIssues[j].IssueID
	})
	sort.SliceStable(knowledgeItems, func(i, j int) bool {
		return knowledgeItems[i].RefID < knowledgeItems[j].RefID
	})

	state.Set("euclo.prior_deferred_issues", openIssues)
	AddContextKnowledgeItems(state, knowledgeItems)
	return nil
}

func isOpenDeferredIssue(issue eucloruntime.DeferredExecutionIssue) bool {
	switch strings.TrimSpace(string(issue.Status)) {
	case "", string(eucloruntime.DeferredIssueStatusOpen), string(eucloruntime.DeferredIssueStatusAcknowledged), string(eucloruntime.DeferredIssueStatusReenteredArchaeology):
		return true
	case string(eucloruntime.DeferredIssueStatusResolved), string(eucloruntime.DeferredIssueStatusIgnored), string(eucloruntime.DeferredIssueStatusSuperseded):
		return false
	default:
		return true
	}
}

// AddContextKnowledgeItems appends loaded knowledge items to the state in the
// same typed shape used by the rest of the context enrichment pipeline.
func AddContextKnowledgeItems(state *core.Context, items any) {
	if state == nil {
		return
	}
	incoming := normalizeContextKnowledgeItems(items)
	if len(incoming) == 0 {
		if _, ok := state.Get("context.knowledge_items"); !ok {
			state.Set("context.knowledge_items", []any{})
		}
		return
	}
	existing := make([]any, 0, len(incoming))
	if raw, ok := state.Get("context.knowledge_items"); ok && raw != nil {
		switch typed := raw.(type) {
		case []KnowledgeEvidenceItem:
			for _, item := range typed {
				existing = append(existing, item)
			}
		case []ContextKnowledgeItem:
			for _, item := range typed {
				existing = append(existing, item)
			}
		case []any:
			existing = append(existing, typed...)
		}
	}
	state.Set("context.knowledge_items", append(existing, incoming...))
}

func normalizeContextKnowledgeItems(items any) []any {
	switch typed := items.(type) {
	case nil:
		return nil
	case []KnowledgeEvidenceItem:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []ContextKnowledgeItem:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func (k ContextKnowledgeItem) toKnowledgeEvidenceItem(issue eucloruntime.DeferredExecutionIssue) KnowledgeEvidenceItem {
	refID := fmt.Sprintf("deferred:%s", strings.TrimSpace(issue.IssueID))
	if refID == "deferred:" {
		refID = fmt.Sprintf("deferred:%s:%s", strings.TrimSpace(string(issue.Kind)), strings.TrimSpace(issue.Title))
	}
	tags := append([]string(nil), k.Tags...)
	if len(tags) == 0 {
		tags = []string{strings.TrimSpace(string(issue.Kind)), strings.TrimSpace(string(issue.Severity))}
	}
	return KnowledgeEvidenceItem{
		RefID:       refID,
		Kind:        KnowledgeKindInteraction,
		Title:       firstNonEmpty(strings.TrimSpace(issue.Title), "Deferred issue"),
		Summary:     strings.TrimSpace(k.Content),
		Source:      EvidenceSource(k.Source),
		TrustClass:  "workspace-trusted",
		RelatedRefs: uniqueStrings(tags),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func severityRank(severity string) int {
	switch strings.TrimSpace(severity) {
	case string(eucloruntime.DeferredIssueSeverityCritical):
		return 0
	case string(eucloruntime.DeferredIssueSeverityHigh):
		return 1
	case string(eucloruntime.DeferredIssueSeverityMedium):
		return 2
	case string(eucloruntime.DeferredIssueSeverityLow):
		return 3
	default:
		return 4
	}
}
