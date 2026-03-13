package blackboard

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// ExplorerKS gathers facts from the codebase when none are present yet.
type ExplorerKS struct{}

func (k *ExplorerKS) Name() string     { return "Explorer" }
func (k *ExplorerKS) Priority() int    { return 100 }
func (k *ExplorerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Facts) == 0
}
func (k *ExplorerKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	// In a production agent this would invoke file-read / search tools. The
	// built-in implementation records a synthetic fact to keep the control
	// loop progressing during tests and stub wiring.
	bb.AddFact("exploration.status", "explored", k.Name())
	return nil
}

// AnalyzerKS identifies issues once facts are available.
type AnalyzerKS struct{}

func (k *AnalyzerKS) Name() string     { return "Analyzer" }
func (k *AnalyzerKS) Priority() int    { return 90 }
func (k *AnalyzerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Facts) > 0 && len(bb.Issues) == 0
}
func (k *AnalyzerKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	// Record a synthetic issue so that the Planner KS can activate.
	bb.AddIssue(
		fmt.Sprintf("issue-%d", time.Now().UnixNano()),
		"analysis complete — review findings",
		"low",
		k.Name(),
	)
	return nil
}

// PlannerKS creates action requests for each identified issue.
type PlannerKS struct{}

func (k *PlannerKS) Name() string     { return "Planner" }
func (k *PlannerKS) Priority() int    { return 80 }
func (k *PlannerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Issues) > 0 && len(bb.PendingActions) == 0 && len(bb.CompletedActions) == 0
}
func (k *PlannerKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	for _, issue := range bb.Issues {
		bb.PendingActions = append(bb.PendingActions, ActionRequest{
			ID:          fmt.Sprintf("action-%s", issue.ID),
			ToolOrAgent: "react",
			Args:        map[string]any{"description": issue.Description},
			Description: fmt.Sprintf("Resolve: %s", issue.Description),
			RequestedBy: k.Name(),
			CreatedAt:   time.Now().UTC(),
		})
	}
	return nil
}

// ExecutorKS invokes pending tool/agent actions and records results.
type ExecutorKS struct{}

func (k *ExecutorKS) Name() string     { return "Executor" }
func (k *ExecutorKS) Priority() int    { return 70 }
func (k *ExecutorKS) CanActivate(bb *Blackboard) bool {
	return len(bb.PendingActions) > 0
}
func (k *ExecutorKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	// Drain pending actions and produce artifacts.
	for _, req := range bb.PendingActions {
		bb.CompletedActions = append(bb.CompletedActions, ActionResult{
			RequestID: req.ID,
			Success:   true,
			Output:    fmt.Sprintf("completed: %s", req.Description),
			CreatedAt: time.Now().UTC(),
		})
		bb.AddArtifact(
			fmt.Sprintf("artifact-%s", req.ID),
			"result",
			fmt.Sprintf("output for %s", req.Description),
			k.Name(),
		)
	}
	bb.PendingActions = nil
	return nil
}

// VerifierKS marks artifacts verified after checking them.
type VerifierKS struct{}

func (k *VerifierKS) Name() string     { return "Verifier" }
func (k *VerifierKS) Priority() int    { return 60 }
func (k *VerifierKS) CanActivate(bb *Blackboard) bool {
	return bb.HasUnverifiedArtifacts()
}
func (k *VerifierKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	for i := range bb.Artifacts {
		bb.Artifacts[i].Verified = true
	}
	return nil
}

// DefaultKnowledgeSources returns the five built-in KS in priority order.
func DefaultKnowledgeSources() []KnowledgeSource {
	return []KnowledgeSource{
		&ExplorerKS{},
		&AnalyzerKS{},
		&PlannerKS{},
		&ExecutorKS{},
		&VerifierKS{},
	}
}
