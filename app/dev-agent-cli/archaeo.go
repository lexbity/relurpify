package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/ayenitd"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
)

type archaeoInspector interface {
	RequestHistory(context.Context, string) (*eucloexec.RequestHistoryView, error)
	ActivePlan(context.Context, string) (*eucloexec.ActivePlanView, error)
	LearningQueue(context.Context, string) (*eucloexec.LearningQueueView, error)
	TensionsByWorkflow(context.Context, string) ([]eucloexec.TensionView, error)
}

var newArchaeoInspectorFn = func(ws *ayenitd.Workspace) archaeoInspector {
	if ws == nil {
		return nil
	}
	return newWorkspaceArchaeoInspector(ws)
}

type workspaceArchaeoInspector struct {
	projections *archaeoprojections.Service
	tensions    archaeotensions.Service
}

func newWorkspaceArchaeoInspector(ws *ayenitd.Workspace) archaeoInspector {
	if ws == nil {
		return nil
	}
	return &workspaceArchaeoInspector{
		projections: &archaeoprojections.Service{Store: ws.Environment.WorkflowStore},
		tensions:    archaeotensions.Service{Store: ws.Environment.WorkflowStore},
	}
}

func (a *workspaceArchaeoInspector) RequestHistory(ctx context.Context, workflowID string) (*eucloexec.RequestHistoryView, error) {
	if a == nil || a.projections == nil {
		return nil, nil
	}
	history, err := a.projections.RequestHistory(ctx, workflowID)
	if err != nil || history == nil {
		return nil, err
	}
	return requestHistoryView(history), nil
}

func (a *workspaceArchaeoInspector) ActivePlan(ctx context.Context, workflowID string) (*eucloexec.ActivePlanView, error) {
	if a == nil || a.projections == nil {
		return nil, nil
	}
	proj, err := a.projections.ActivePlan(ctx, workflowID)
	if err != nil || proj == nil {
		return nil, err
	}
	return activePlanView(proj), nil
}

func (a *workspaceArchaeoInspector) LearningQueue(ctx context.Context, workflowID string) (*eucloexec.LearningQueueView, error) {
	if a == nil || a.projections == nil {
		return nil, nil
	}
	queue, err := a.projections.LearningQueue(ctx, workflowID)
	if err != nil || queue == nil {
		return nil, err
	}
	return learningQueueView(queue), nil
}

func (a *workspaceArchaeoInspector) TensionsByWorkflow(ctx context.Context, workflowID string) ([]eucloexec.TensionView, error) {
	if a == nil {
		return nil, nil
	}
	tensions, err := a.tensions.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return tensionViews(tensions), nil
}

func newArchaeoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archaeo",
		Short: "Inspect workflow archaeology projections",
	}
	cmd.PersistentFlags().Bool("json", false, "Emit machine-readable JSON")
	cmd.AddCommand(
		newArchaeoPlanCmd(),
		newArchaeoTensionsCmd(),
		newArchaeoHistoryCmd(),
		newArchaeoLearningCmd(),
		newArchaeoWorkflowsCmd(),
	)
	return cmd
}

func newArchaeoPlanCmd() *cobra.Command {
	var workflowID string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show the active plan projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArchaeoWorkflowView(cmd, workflowID, func(ctx context.Context, inspector archaeoInspector) (any, error) {
				view, err := inspector.ActivePlan(ctx, workflowID)
				if err != nil {
					return nil, err
				}
				if view == nil {
					return nil, fmt.Errorf("no active plan found for workflow %s", workflowID)
				}
				if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
					return view, nil
				}
				printArchaeoActivePlan(cmd, view)
				return nil, nil
			})
		},
	}
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Workflow ID to inspect")
	return cmd
}

func newArchaeoTensionsCmd() *cobra.Command {
	var workflowID string
	cmd := &cobra.Command{
		Use:   "tensions",
		Short: "List workflow tensions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArchaeoWorkflowView(cmd, workflowID, func(ctx context.Context, inspector archaeoInspector) (any, error) {
				views, err := inspector.TensionsByWorkflow(ctx, workflowID)
				if err != nil {
					return nil, err
				}
				if len(views) == 0 {
					if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
						return []eucloexec.TensionView{}, nil
					}
					fmt.Fprintf(cmd.OutOrStdout(), "No tensions found for workflow %s.\n", workflowID)
					return nil, nil
				}
				if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
					return views, nil
				}
				printArchaeoTensions(cmd, views)
				return nil, nil
			})
		},
	}
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Workflow ID to inspect")
	return cmd
}

func newArchaeoHistoryCmd() *cobra.Command {
	var workflowID string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show request history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArchaeoWorkflowView(cmd, workflowID, func(ctx context.Context, inspector archaeoInspector) (any, error) {
				view, err := inspector.RequestHistory(ctx, workflowID)
				if err != nil {
					return nil, err
				}
				if view == nil {
					return nil, fmt.Errorf("no request history found for workflow %s", workflowID)
				}
				if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
					return view, nil
				}
				printArchaeoHistory(cmd, view)
				return nil, nil
			})
		},
	}
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Workflow ID to inspect")
	return cmd
}

func newArchaeoLearningCmd() *cobra.Command {
	var workflowID string
	cmd := &cobra.Command{
		Use:   "learning",
		Short: "Show learning queue state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runArchaeoWorkflowView(cmd, workflowID, func(ctx context.Context, inspector archaeoInspector) (any, error) {
				view, err := inspector.LearningQueue(ctx, workflowID)
				if err != nil {
					return nil, err
				}
				if view == nil {
					return nil, fmt.Errorf("no learning queue found for workflow %s", workflowID)
				}
				if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
					return view, nil
				}
				printArchaeoLearning(cmd, view)
				return nil, nil
			})
		},
	}
	cmd.Flags().StringVar(&workflowID, "workflow", "", "Workflow ID to inspect")
	return cmd
}

func newArchaeoWorkflowsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "List known workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ws, err := openWorkspaceForInspection(ctx, ensureWorkspace())
			if err != nil {
				return err
			}
			defer func() { _ = ws.Close() }()
			if ws.ServiceManager != nil {
				if err := ws.ServiceManager.StartAll(ctx); err != nil {
					return err
				}
			}
			if ws.Environment.WorkflowStore == nil {
				if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
					return json.NewEncoder(cmd.OutOrStdout()).Encode([]workflowListItem{})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "No workflow store available.")
				return nil
			}
			records, err := ws.Environment.WorkflowStore.ListWorkflows(ctx, 1000)
			if err != nil {
				return err
			}
			items := make([]workflowListItem, 0, len(records))
			for _, rec := range records {
				items = append(items, workflowListItem{
					WorkflowID:   rec.WorkflowID,
					TaskID:       rec.TaskID,
					Status:       string(rec.Status),
					Instruction:  rec.Instruction,
					CursorStepID: rec.CursorStepID,
					Version:      rec.Version,
					CreatedAt:    rec.CreatedAt,
					UpdatedAt:    rec.UpdatedAt,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
					return items[i].WorkflowID < items[j].WorkflowID
				}
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			})
			if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No workflows found.")
				return nil
			}
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s | status=%s | created=%s | updated=%s\n",
					item.WorkflowID, item.Status, item.CreatedAt.Format(timeFormat), item.UpdatedAt.Format(timeFormat))
			}
			return nil
		},
	}
	return cmd
}

type workflowListItem struct {
	WorkflowID   string    `json:"workflow_id,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	Status       string    `json:"status,omitempty"`
	Instruction  string    `json:"instruction,omitempty"`
	CursorStepID string    `json:"cursor_step_id,omitempty"`
	Version      int64     `json:"version,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

const timeFormat = "2006-01-02 15:04:05"

func runArchaeoWorkflowView(cmd *cobra.Command, workflowID string, fn func(context.Context, archaeoInspector) (any, error)) error {
	if strings.TrimSpace(workflowID) == "" {
		return fmt.Errorf("--workflow is required")
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := openWorkspaceForInspection(ctx, ensureWorkspace())
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	if ws.ServiceManager != nil {
		if err := ws.ServiceManager.StartAll(ctx); err != nil {
			return err
		}
	}
	inspector := newArchaeoInspectorFn(ws)
	if inspector == nil {
		return fmt.Errorf("archaeo inspector unavailable")
	}
	payload, err := fn(ctx, inspector)
	if err != nil {
		return err
	}
	if payload == nil {
		return nil
	}
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}
	return nil
}

func printArchaeoActivePlan(cmd *cobra.Command, view *eucloexec.ActivePlanView) {
	if view == nil {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "workflow: %s\n", view.WorkflowID)
	fmt.Fprintf(cmd.OutOrStdout(), "phase: %s\n", view.Phase)
	if view.ActivePlan != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "plan: %s v%d (%s)\n", view.ActivePlan.PlanID, view.ActivePlan.Version, view.ActivePlan.Status)
		if view.ActiveStepID != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "active step: %s\n", view.ActiveStepID)
		}
		if len(view.ActivePlan.Plan.StepOrder) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "steps:")
			for _, stepID := range view.ActivePlan.Plan.StepOrder {
				step := view.ActivePlan.Plan.Steps[stepID]
				status := ""
				if step != nil {
					status = string(step.Status)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", stepID, status)
			}
		}
	}
}

func printArchaeoTensions(cmd *cobra.Command, views []eucloexec.TensionView) {
	for _, tension := range views {
		fmt.Fprintf(cmd.OutOrStdout(), "%s | kind=%s | severity=%s | status=%s\n",
			tension.ID, tension.Kind, tension.Severity, tension.Status)
		if tension.Description != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", tension.Description)
		}
	}
}

func printArchaeoHistory(cmd *cobra.Command, view *eucloexec.RequestHistoryView) {
	fmt.Fprintf(cmd.OutOrStdout(), "workflow: %s\n", view.WorkflowID)
	fmt.Fprintf(cmd.OutOrStdout(), "pending=%d running=%d completed=%d failed=%d canceled=%d\n",
		view.Pending, view.Running, view.Completed, view.Failed, view.Canceled)
	for _, req := range view.Requests {
		fmt.Fprintf(cmd.OutOrStdout(), "%s | kind=%s | status=%s | %s\n", req.ID, req.Kind, req.Status, req.Summary)
	}
}

func printArchaeoLearning(cmd *cobra.Command, view *eucloexec.LearningQueueView) {
	fmt.Fprintf(cmd.OutOrStdout(), "workflow: %s\n", view.WorkflowID)
	if len(view.PendingGuidanceIDs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "pending guidance: %s\n", strings.Join(view.PendingGuidanceIDs, ", "))
	}
	if len(view.BlockingLearning) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "blocking: %s\n", strings.Join(view.BlockingLearning, ", "))
	}
	for _, item := range view.PendingLearning {
		fmt.Fprintf(cmd.OutOrStdout(), "%s | status=%s | blocking=%v | %s\n", item.ID, item.Status, item.Blocking, item.Prompt)
	}
}

func requestHistoryView(history *archaeoprojections.RequestHistoryProjection) *eucloexec.RequestHistoryView {
	view := &eucloexec.RequestHistoryView{
		WorkflowID: history.WorkflowID,
		Pending:    history.Pending,
		Running:    history.Running,
		Completed:  history.Completed,
		Failed:     history.Failed,
		Canceled:   history.Canceled,
		Requests:   make([]eucloexec.RequestRecordView, 0, len(history.Requests)),
	}
	for _, request := range history.Requests {
		view.Requests = append(view.Requests, eucloexec.RequestRecordView{
			ID:        request.ID,
			Kind:      string(request.Kind),
			Scope:     strings.Join(request.SubjectRefs, ","),
			Status:    string(request.Status),
			Summary:   firstArchaeoNonEmptyStringValue(strings.TrimSpace(request.Title), strings.TrimSpace(request.Description)),
			CreatedAt: request.RequestedAt,
			UpdatedAt: request.UpdatedAt,
		})
	}
	return view
}

func activePlanView(proj *archaeoprojections.ActivePlanProjection) *eucloexec.ActivePlanView {
	view := &eucloexec.ActivePlanView{WorkflowID: proj.WorkflowID}
	if proj.PhaseState != nil {
		view.Phase = string(proj.PhaseState.CurrentPhase)
	}
	if proj.ActivePlanVersion != nil {
		view.ActivePlan = versionedPlanView(*proj.ActivePlanVersion)
		for _, stepID := range proj.ActivePlanVersion.Plan.StepOrder {
			if step := proj.ActivePlanVersion.Plan.Steps[stepID]; step != nil && step.Status == frameworkplan.PlanStepInProgress {
				view.ActiveStepID = stepID
				break
			}
		}
	}
	return view
}

func learningQueueView(queue *archaeoprojections.LearningQueueProjection) *eucloexec.LearningQueueView {
	view := &eucloexec.LearningQueueView{
		WorkflowID:         queue.WorkflowID,
		PendingGuidanceIDs: append([]string(nil), queue.PendingGuidanceIDs...),
		BlockingLearning:   append([]string(nil), queue.BlockingLearning...),
		PendingLearning:    make([]eucloexec.LearningInteractionView, 0, len(queue.PendingLearning)),
	}
	for _, item := range queue.PendingLearning {
		evidence := make([]string, 0, len(item.Evidence))
		for _, ref := range item.Evidence {
			evidence = append(evidence, strings.TrimSpace(ref.RefID))
		}
		view.PendingLearning = append(view.PendingLearning, eucloexec.LearningInteractionView{
			ID:        item.ID,
			Status:    string(item.Status),
			Blocking:  item.Blocking,
			Prompt:    firstArchaeoNonEmptyStringValue(strings.TrimSpace(item.Title), strings.TrimSpace(item.Description)),
			SubjectID: item.SubjectID,
			Evidence:  evidence,
		})
	}
	return view
}

func tensionViews(tensions []archaeodomain.Tension) []eucloexec.TensionView {
	out := make([]eucloexec.TensionView, 0, len(tensions))
	for _, tension := range tensions {
		out = append(out, eucloexec.TensionView{
			ID:                 tension.ID,
			Kind:               tension.Kind,
			Description:        tension.Description,
			Severity:           tension.Severity,
			Status:             string(tension.Status),
			PatternIDs:         append([]string(nil), tension.PatternIDs...),
			AnchorRefs:         append([]string(nil), tension.AnchorRefs...),
			SymbolScope:        append([]string(nil), tension.SymbolScope...),
			RelatedPlanStepIDs: append([]string(nil), tension.RelatedPlanStepIDs...),
			BasedOnRevision:    tension.BasedOnRevision,
		})
	}
	return out
}

func versionedPlanView(version archaeodomain.VersionedLivingPlan) *eucloexec.VersionedPlanView {
	return &eucloexec.VersionedPlanView{
		ID:                     version.ID,
		WorkflowID:             version.WorkflowID,
		PlanID:                 version.Plan.ID,
		Version:                version.Version,
		Status:                 string(version.Status),
		DerivedFromExploration: version.DerivedFromExploration,
		BasedOnRevision:        version.BasedOnRevision,
		SemanticSnapshotRef:    version.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), version.PatternRefs...),
		AnchorRefs:             append([]string(nil), version.AnchorRefs...),
		TensionRefs:            append([]string(nil), version.TensionRefs...),
		Plan:                   version.Plan,
	}
}

func firstArchaeoNonEmptyStringValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
