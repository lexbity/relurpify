package local

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkpipeline "codeburg.org/lexbit/relurpify/framework/pipeline"
)

type migrationPreCheckStage struct{ step map[string]any }
type migrationExecuteStage struct{ step map[string]any }
type migrationPostCheckStage struct{ step map[string]any }

func (s *migrationPreCheckStage) Name() string  { return "migration_precheck" }
func (s *migrationExecuteStage) Name() string   { return "migration_execute" }
func (s *migrationPostCheckStage) Name() string { return "migration_postcheck" }

func (s *migrationPreCheckStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{Name: "migration_precheck", Description: "Verify migration step preconditions", Metadata: frameworkpipeline.ContractMetadata{InputKey: "migration.step", OutputKey: "migration.precheck_result", SchemaVersion: "v1"}}
}
func (s *migrationExecuteStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{Name: "migration_execute", Description: "Apply migration step changes", Metadata: frameworkpipeline.ContractMetadata{InputKey: "migration.precheck_result", OutputKey: "migration.step_changes", SchemaVersion: "v1"}}
}
func (s *migrationPostCheckStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{Name: "migration_postcheck", Description: "Verify migration step postconditions", Metadata: frameworkpipeline.ContractMetadata{InputKey: "migration.step_changes", OutputKey: "migration.postcheck_result", SchemaVersion: "v1"}}
}

func (s *migrationPreCheckStage) BuildPrompt(_ *core.Context) (string, error) {
	return fmt.Sprintf("Verify these migration preconditions hold: %s", strings.Join(stringSlice(s.step["preconditions"]), "; ")), nil
}
func (s *migrationExecuteStage) BuildPrompt(_ *core.Context) (string, error) {
	return fmt.Sprintf("Execute this migration step: %s", stringValue(s.step["description"])), nil
}
func (s *migrationPostCheckStage) BuildPrompt(_ *core.Context) (string, error) {
	return fmt.Sprintf("Verify these migration postconditions: %s", strings.Join(stringSlice(s.step["postconditions"]), "; ")), nil
}

func (s *migrationPreCheckStage) Decode(_ *core.LLMResponse) (any, error) {
	failures := migrationStageFailures(s.step, "precheck")
	return map[string]any{"all_met": len(failures) == 0, "failures": failures, "step_id": stringValue(s.step["id"])}, nil
}
func (s *migrationExecuteStage) Decode(_ *core.LLMResponse) (any, error) {
	return map[string]any{"step_id": stringValue(s.step["id"]), "summary": stringValue(s.step["description"]), "files": normalizeStringSlice(s.step["files"]), "status": "applied"}, nil
}
func (s *migrationPostCheckStage) Decode(_ *core.LLMResponse) (any, error) {
	failures := migrationStageFailures(s.step, "postcheck")
	return map[string]any{"all_met": len(failures) == 0, "failures": failures, "step_id": stringValue(s.step["id"]), "test_results": map[string]any{"status": ifElse(len(failures) == 0, "pass", "fail")}}, nil
}

func (s *migrationPreCheckStage) Validate(output any) error {
	record, _ := output.(map[string]any)
	if record != nil && boolValue(record["all_met"]) {
		return nil
	}
	return &frameworkpipeline.ValidationError{Message: "migration preconditions not met"}
}
func (s *migrationExecuteStage) Validate(output any) error {
	record, _ := output.(map[string]any)
	if record == nil || stringValue(record["summary"]) == "" {
		return &frameworkpipeline.ValidationError{Message: "migration execution produced no summary"}
	}
	return nil
}
func (s *migrationPostCheckStage) Validate(output any) error {
	record, _ := output.(map[string]any)
	if record != nil && boolValue(record["all_met"]) {
		return nil
	}
	return &frameworkpipeline.ValidationError{Message: "migration postconditions not met"}
}

func (s *migrationPreCheckStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("migration.precheck_result", output)
	return nil
}
func (s *migrationExecuteStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("migration.step_changes", output)
	return nil
}
func (s *migrationPostCheckStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("migration.postcheck_result", output)
	return nil
}

func newMigrationStages(step map[string]any) []frameworkpipeline.Stage {
	return []frameworkpipeline.Stage{&migrationPreCheckStage{step: step}, &migrationExecuteStage{step: step}, &migrationPostCheckStage{step: step}}
}

func migrationStageFailures(step map[string]any, phase string) []string {
	stepID := stringValue(step["id"])
	switch phase {
	case "precheck":
		if boolValue(step["force_precheck_fail"]) {
			return []string{fmt.Sprintf("precheck failed for %s", stepID)}
		}
	case "postcheck":
		if boolValue(step["force_postcheck_fail"]) {
			return []string{fmt.Sprintf("postcheck failed for %s", stepID)}
		}
	}
	return nil
}

func boolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func normalizeStringSlice(raw any) []string { return uniqueStrings(stringSlice(raw)) }

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func ifElse[T any](condition bool, whenTrue, whenFalse T) T {
	if condition {
		return whenTrue
	}
	return whenFalse
}
