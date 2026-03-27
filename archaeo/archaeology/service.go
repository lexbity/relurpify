package archaeology

import (
	"context"
	"fmt"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type PhasePersister func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep)
type GateEvaluator func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error)
type DoomLoopReset func()

type Service struct {
	Store        memory.WorkflowStateStore
	Plans        archaeoplans.Service
	PersistPhase PhasePersister
	EvaluateGate GateEvaluator
	ResetDoom    DoomLoopReset
	Learning     archaeolearning.Service
	Providers    providers.Bundle
	Requests     archaeorequests.Service
	Now          func() time.Time
	NewID        func(prefix string) string
}

type PrepareResult struct {
	Plan   *frameworkplan.LivingPlan
	Step   *frameworkplan.PlanStep
	Result *core.Result
	Err    error
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case []string:
			out[key] = append([]string(nil), typed...)
		default:
			out[key] = typed
		}
	}
	return out
}

func sameStringSet(left, right []string) bool {
	return strings.Join(uniqueStrings(left), "\x00") == strings.Join(uniqueStrings(right), "\x00")
}

func sameAnyMap(left, right map[string]any) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			return false
		}
		switch typed := leftValue.(type) {
		case []string:
			other, ok := rightValue.([]string)
			if !ok || !sameStringSet(typed, other) {
				return false
			}
		default:
			if fmt.Sprintf("%v", leftValue) != fmt.Sprintf("%v", rightValue) {
				return false
			}
		}
	}
	return true
}

func planVersionRef(plan *frameworkplan.LivingPlan) *int {
	if plan == nil || plan.Version == 0 {
		return nil
	}
	version := plan.Version
	return &version
}

func planID(plan *frameworkplan.LivingPlan) string {
	if plan == nil {
		return ""
	}
	return plan.ID
}

func planStepRefs(plan *frameworkplan.LivingPlan) []string {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	refs := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		if step != nil && strings.TrimSpace(step.ID) != "" {
			refs = append(refs, step.ID)
		}
	}
	return uniqueStrings(refs)
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func symbolScopeFromTaskState(task *core.Task, state *core.Context) string {
	if task != nil && task.Context != nil {
		for _, key := range []string{"symbol_scope", "file_path", "path"} {
			if raw, ok := task.Context[key]; ok {
				if value, ok := raw.(string); ok {
					if trimmed := strings.TrimSpace(value); trimmed != "" {
						return trimmed
					}
				}
			}
		}
	}
	if state != nil {
		for _, key := range []string{"euclo.symbol_scope", "euclo.file_path"} {
			if value := strings.TrimSpace(state.GetString(key)); value != "" {
				return value
			}
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
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}

func firstNonEmptyTensionStatus(value archaeodomain.TensionStatus, fallback archaeodomain.TensionStatus) archaeodomain.TensionStatus {
	if strings.TrimSpace(string(value)) != "" {
		return value
	}
	return fallback
}

func (s Service) persistPhase(ctx context.Context, task *core.Task, state *core.Context, phase archaeodomain.EucloPhase, reason string, step *frameworkplan.PlanStep) {
	if s.PersistPhase != nil {
		s.PersistPhase(ctx, task, state, phase, reason, step)
	}
}

func (s Service) evaluateGate(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
	if s.EvaluateGate == nil {
		return archaeoexec.PreflightOutcome{}, fmt.Errorf("gate evaluator unavailable")
	}
	return s.EvaluateGate(ctx, task, state, plan, step)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func explorationIDFromTaskState(task *core.Task, state *core.Context, workflowID string) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.active_exploration_id")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["exploration_id"])); value != "" && value != "<nil>" {
			return value
		}
		if workspace := strings.TrimSpace(fmt.Sprint(task.Context["workspace"])); workspace != "" && workspace != "<nil>" {
			return "workspace:" + workspace
		}
	}
	if workflowID != "" {
		return "workflow:" + workflowID
	}
	return "workspace:session"
}

func corpusScopeFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.corpus_scope")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["corpus_scope"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return "workspace"
}

func basedOnRevisionFromTask(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.based_on_revision")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["based_on_revision"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func workspaceIDFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.workspace")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["workspace"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func stringSliceFromState(state *core.Context, key string) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value == "" || value == "<nil>" {
				continue
			}
			out = append(out, value)
		}
		return out
	default:
		return nil
	}
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	if s.Learning.Now != nil {
		return s.Learning.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	if s.Learning.NewID != nil {
		return s.Learning.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(prefix), s.now().UnixNano())
}
