package thoughtrecipe

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// emitStepStarted emits the step-started telemetry event.
func emitStepStarted(ctx context.Context, env *contextdata.Envelope, step ExecutionStep) {
	total, _ := contextdata.GetTyped[int](env, "euclo.execution.step_total")
	idx, _ := contextdata.GetTyped[int](env, "euclo.execution.step_index")
	contextdata.SetTyped(env, "euclo.execution.step_index", idx+1)

	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitStepStarted(ctx, reporting.EventStepStarted{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			OccurredAt: time.Now().UTC(),
		},
		StepID:          step.ID,
		ThoughtRecipeID: "",
		Paradigm:        step.Paradigm,
		Index:           idx,
		Total:           total,
	})
}

// emitStepCompleted emits the step-completed telemetry event.
func emitStepCompleted(ctx context.Context, env *contextdata.Envelope, step ExecutionStep, success bool, dur time.Duration) {
	total, _ := contextdata.GetTyped[int](env, "euclo.execution.step_total")
	idx, _ := contextdata.GetTyped[int](env, "euclo.execution.step_index")

	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitStepCompleted(ctx, reporting.EventStepCompleted{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			OccurredAt: time.Now().UTC(),
		},
		StepID:          step.ID,
		ThoughtRecipeID: "",
		Paradigm:        step.Paradigm,
		Success:         success,
		DurationMs:      dur.Milliseconds(),
		Index:           idx - 1,
		Total:           total,
	})
}

func (c *stepCore) stepMetadata() map[string]any {
	metadata := map[string]any{
		"execution_step_id":   c.step.ID,
		"execution_step_type": c.step.Kind.String(),
		"execution_paradigm":  c.step.Paradigm,
		"execution_goal":      c.step.Goal,
		"execution_question":  c.step.Question,
		"execution_mutation":  c.step.Mutation,
		"execution_hitl":      c.step.HITL,
	}
	if len(c.step.Sources) > 0 {
		metadata["execution_sources"] = append([]string(nil), c.step.Sources...)
	}
	if len(c.step.Choices) > 0 {
		metadata["execution_choices"] = append([]string(nil), c.step.Choices...)
	}
	if strings.TrimSpace(c.step.ChoiceSource) != "" {
		metadata["execution_choice_source"] = c.step.ChoiceSource
	}
	if len(c.step.Directives) > 0 {
		metadata["execution_directives"] = append([]string(nil), c.step.Directives...)
	}
	if strings.TrimSpace(c.step.CapabilityID) != "" {
		metadata[executionCapabilityIDKey] = c.step.CapabilityID
	}
	if cfg := cloneClarificationStepConfig(c.step.ClarificationConfig); cfg != nil {
		metadata["execution_clarification_type"] = c.step.Kind.String()
		metadata["execution_clarification_config"] = cfg
	}
	return metadata
}

func (c *stepCore) writeStepMetadata(env *contextdata.Envelope) {
	base := "euclo.execution.step." + c.step.ID
	contextdata.SetTyped(env, base+".id", c.step.ID)
	contextdata.SetTyped(env, base+".type", c.step.Kind.String())
	contextdata.SetTyped(env, base+".paradigm", c.step.Paradigm)
	contextdata.SetTyped(env, base+".goal", c.step.Goal)
	contextdata.SetTyped(env, base+".question", c.step.Question)
	contextdata.SetTyped(env, base+".prompt_id", c.step.PromptID)
	contextdata.SetTyped(env, base+".mutation", c.step.Mutation)
	contextdata.SetTyped(env, base+".hitl", c.step.HITL)
	if len(c.step.Sources) > 0 {
		contextdata.SetTyped(env, base+".sources", append([]string(nil), c.step.Sources...))
	}
	if len(c.step.Choices) > 0 {
		contextdata.SetTyped(env, base+".choices", append([]string(nil), c.step.Choices...))
	}
	if strings.TrimSpace(c.step.ChoiceSource) != "" {
		contextdata.SetTyped(env, base+".choice_source", c.step.ChoiceSource)
	}
	if len(c.step.Directives) > 0 {
		contextdata.SetTyped(env, base+".directives", append([]string(nil), c.step.Directives...))
	}
	if strings.TrimSpace(c.step.CapabilityID) != "" {
		contextdata.SetTyped(env, base+".capability_id", c.step.CapabilityID)
	}
	if cfg := cloneClarificationStepConfig(c.step.ClarificationConfig); cfg != nil {
		contextdata.SetTyped(env, base+".clarification_type", c.step.Kind.String())
		contextdata.SetTyped(env, base+".clarification_config", cfg)
		contextdata.SetTyped(env, base+".clarification_schema_id", cfg.OutputSchemaID)
		contextdata.SetTyped(env, base+".clarification_validation_mode", cfg.ValidationMode)
		contextdata.SetTyped(env, base+".clarification_required_fields", append([]string(nil), cfg.RequiredFields...))
		contextdata.SetTyped(env, base+".clarification_allowed_statuses", append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...))
		contextdata.SetTyped(env, base+".clarification_state_write_keys", append([]string(nil), cfg.StateWriteKeys...))
		contextdata.SetTyped(env, base+".clarification_projection_policy", cfg.ProjectionPolicy)
		contextdata.SetTyped(env, base+".clarification_requery_on_success", cfg.RequeryOnSuccess)
	}
}
