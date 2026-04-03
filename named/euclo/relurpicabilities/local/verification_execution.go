package local

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type verificationScopeSelectCapability struct{ env agentenv.AgentEnvironment }
type verificationExecuteCapability struct{ env agentenv.AgentEnvironment }

func NewVerificationScopeSelectCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &verificationScopeSelectCapability{env: env}
}

func NewVerificationExecuteCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &verificationExecuteCapability{env: env}
}

func ExecuteVerificationFlow(ctx context.Context, env euclotypes.ExecutionEnvelope, snapshot euclotypes.CapabilitySnapshot) ([]euclotypes.Artifact, bool, error) {
	if !snapshot.HasVerificationTools && !snapshot.HasExecuteTools {
		return nil, false, nil
	}
	scopeCap := NewVerificationScopeSelectCapability(env.Environment)
	scopeResult := scopeCap.Execute(ctx, env)
	if scopeResult.Status == euclotypes.ExecutionStatusFailed {
		return nil, false, verificationFlowError(scopeResult)
	}
	artifacts := append([]euclotypes.Artifact{}, scopeResult.Artifacts...)

	if !snapshot.HasExecuteTools {
		return artifacts, false, nil
	}
	execCap := NewVerificationExecuteCapability(env.Environment)
	execResult := execCap.Execute(ctx, env)
	if execResult.Status == euclotypes.ExecutionStatusFailed {
		return artifacts, false, verificationFlowError(execResult)
	}
	if len(execResult.Artifacts) == 0 {
		return artifacts, false, nil
	}
	artifacts = append(artifacts, execResult.Artifacts...)
	return artifacts, true, nil
}

func (c *verificationScopeSelectCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:verification.scope_select",
		Name:          "Verification Scope Select",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "verification", "planning"},
		Annotations: map[string]any{
			"supported_profiles": []string{"edit_verify_repair", "reproduce_localize_patch", "test_driven_generation"},
		},
	}
}

func (c *verificationScopeSelectCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindVerificationPlan,
		},
	}
}

func (c *verificationScopeSelectCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasVerificationTools && !snapshot.HasExecuteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification scope selection requires execute or verification tools"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification scope selection requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "verification planning is available"}
}

func (c *verificationScopeSelectCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	plan := buildVerificationPlanWithContext(ctx, env)
	artifact := euclotypes.Artifact{
		ID:         "verification_scope_plan",
		Kind:       euclotypes.ArtifactKindVerificationPlan,
		Summary:    summarizePayload(plan),
		Payload:    plan,
		ProducerID: "euclo:verification.scope_select",
		Status:     "produced",
	}
	if env.State != nil {
		env.State.Set("euclo.verification_plan", plan)
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "verification scope selected", Artifacts: []euclotypes.Artifact{artifact}}
}

func (c *verificationExecuteCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:verification.execute",
		Name:          "Verification Execute",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "verification", "execution"},
		Annotations: map[string]any{
			"supported_profiles": []string{"edit_verify_repair", "reproduce_localize_patch", "test_driven_generation"},
		},
	}
}

func (c *verificationExecuteCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *verificationExecuteCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasExecuteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification execution requires execute tools"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification execution requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "verification execution is available"}
}

func (c *verificationExecuteCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	plan := verificationPlanFromState(env.State)
	if len(plan.Commands) == 0 {
		plan = buildVerificationPlanWithContext(ctx, env)
	}
	if len(plan.Commands) == 0 {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusPartial,
			Summary: "no executable verification plan was available",
		}
	}
	checks := make([]map[string]any, 0, len(plan.Commands))
	overallStatus := "pass"
	for _, cmd := range plan.Commands {
		start := time.Now().UTC()
		execCmd := exec.CommandContext(ctx, cmd.Command, cmd.Args...)
		execCmd.Dir = cmd.WorkingDirectory
		output, err := execCmd.CombinedOutput()
		exitStatus := 0
		if err != nil {
			overallStatus = "fail"
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitStatus = exitErr.ExitCode()
			} else {
				exitStatus = 1
			}
		}
		checks = append(checks, map[string]any{
			"name":                   cmd.Name,
			"command":                cmd.Command,
			"args":                   append([]string(nil), cmd.Args...),
			"working_directory":      cmd.WorkingDirectory,
			"status":                 commandStatus(err),
			"exit_status":            exitStatus,
			"duration_millis":        time.Since(start).Milliseconds(),
			"files_under_check":      append([]string(nil), plan.Files...),
			"scope_kind":             plan.ScopeKind,
			"originating_capability": "euclo:verification.execute",
			"run_id":                 env.RunID,
			"timestamp":              start.Format(time.RFC3339),
			"provenance":             "executed",
			"details":                strings.TrimSpace(string(output)),
		})
	}
	payload := map[string]any{
		"status":                  overallStatus,
		"summary":                 verificationExecutionSummary(overallStatus, plan),
		"checks":                  checks,
		"provenance":              "executed",
		"run_id":                  env.RunID,
		"timestamp":               time.Now().UTC().Format(time.RFC3339),
		"scope_kind":              plan.ScopeKind,
		"scope_source":            plan.Source,
		"planner_id":              plan.PlannerID,
		"plan_rationale":          plan.Rationale,
		"plan_audit_trail":        append([]string(nil), plan.AuditTrail...),
		"compatibility_sensitive": plan.CompatibilitySensitive,
		"planner_metadata":        clonePlannerMetadata(plan.PlannerMetadata),
	}
	artifact := euclotypes.Artifact{
		ID:         "verification_execution",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:verification.execute",
		Status:     "produced",
	}
	if env.State != nil {
		env.State.Set("pipeline.verify", payload)
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "verification executed", Artifacts: []euclotypes.Artifact{artifact}}
}

type verificationPlan struct {
	ScopeKind                   string                    `json:"scope_kind,omitempty"`
	Files                       []string                  `json:"files,omitempty"`
	TestFiles                   []string                  `json:"test_files,omitempty"`
	Commands                    []verificationCommandSpec `json:"commands,omitempty"`
	Source                      string                    `json:"source,omitempty"`
	PlannerID                   string                    `json:"planner_id,omitempty"`
	Rationale                   string                    `json:"rationale,omitempty"`
	AuditTrail                  []string                  `json:"audit_trail,omitempty"`
	CompatibilitySensitive      bool                      `json:"compatibility_sensitive"`
	PlannerMetadata             map[string]any            `json:"planner_metadata,omitempty"`
	PolicyPreferredCapabilities []string                  `json:"policy_preferred_capabilities,omitempty"`
	PolicySuccessCapabilities   []string                  `json:"policy_success_capabilities,omitempty"`
	PolicyRequiresVerification  bool                      `json:"policy_requires_verification"`
	SelectionInputs             []string                  `json:"selection_inputs,omitempty"`
}

type verificationCommandSpec struct {
	Name             string   `json:"name,omitempty"`
	Command          string   `json:"command,omitempty"`
	Args             []string `json:"args,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
}

func buildVerificationPlan(env euclotypes.ExecutionEnvelope) verificationPlan {
	return buildVerificationPlanWithContext(context.Background(), env)
}

func buildVerificationPlanWithContext(ctx context.Context, env euclotypes.ExecutionEnvelope) verificationPlan {
	files := verificationFiles(env)
	testFiles := verificationTestFiles(files)
	workspace := verificationWorkspace(env)
	policy := verificationPolicyHints(env)
	if commands := explicitVerificationCommands(env, workspace); len(commands) > 0 {
		return verificationPlan{
			ScopeKind:                   "explicit",
			Files:                       files,
			TestFiles:                   testFiles,
			Commands:                    commands,
			Source:                      verificationPlanSource("task_context", policy),
			PolicyPreferredCapabilities: policy.PreferredVerifyCapabilities,
			PolicySuccessCapabilities:   policy.VerificationSuccessCapabilities,
			PolicyRequiresVerification:  policy.RequireVerificationStep,
			SelectionInputs:             verificationSelectionInputs(files, policy, "task_context"),
		}
	}
	if plan, ok := externalVerificationPlan(ctx, env, files, testFiles, workspace, policy); ok {
		return plan
	}
	if commands, scopeKind := goVerificationCommands(files, workspace, env); len(commands) > 0 {
		return verificationPlan{
			ScopeKind:                   scopeKind,
			Files:                       files,
			TestFiles:                   testFiles,
			Commands:                    commands,
			Source:                      verificationPlanSource("heuristic_go", policy),
			PolicyPreferredCapabilities: policy.PreferredVerifyCapabilities,
			PolicySuccessCapabilities:   policy.VerificationSuccessCapabilities,
			PolicyRequiresVerification:  policy.RequireVerificationStep,
			SelectionInputs:             verificationSelectionInputs(files, policy, "heuristic_go"),
		}
	}
	return verificationPlan{
		ScopeKind:                   "unknown",
		Files:                       files,
		TestFiles:                   testFiles,
		Source:                      verificationPlanSource("no_scope", policy),
		PolicyPreferredCapabilities: policy.PreferredVerifyCapabilities,
		PolicySuccessCapabilities:   policy.VerificationSuccessCapabilities,
		PolicyRequiresVerification:  policy.RequireVerificationStep,
		SelectionInputs:             verificationSelectionInputs(files, policy, "no_scope"),
	}
}

func verificationPlanFromState(state *core.Context) verificationPlan {
	if state == nil {
		return verificationPlan{}
	}
	raw, ok := state.Get("euclo.verification_plan")
	if !ok || raw == nil {
		return verificationPlan{}
	}
	if plan, ok := raw.(verificationPlan); ok {
		return plan
	}
	if record, ok := raw.(map[string]any); ok {
		plan := verificationPlan{
			ScopeKind:                   stringValue(record["scope_kind"]),
			Files:                       uniqueStringsFromAny(record["files"]),
			TestFiles:                   uniqueStringsFromAny(record["test_files"]),
			Source:                      stringValue(record["source"]),
			PlannerID:                   stringValue(record["planner_id"]),
			Rationale:                   stringValue(record["rationale"]),
			AuditTrail:                  uniqueStringsFromAny(record["audit_trail"]),
			CompatibilitySensitive:      verificationBoolValue(record["compatibility_sensitive"]),
			PlannerMetadata:             clonePlannerMetadata(mapValue(record["planner_metadata"])),
			PolicyPreferredCapabilities: uniqueStringsFromAny(record["policy_preferred_capabilities"]),
			PolicySuccessCapabilities:   uniqueStringsFromAny(record["policy_success_capabilities"]),
			PolicyRequiresVerification:  verificationBoolValue(record["policy_requires_verification"]),
			SelectionInputs:             uniqueStringsFromAny(record["selection_inputs"]),
		}
		switch typed := record["commands"].(type) {
		case []verificationCommandSpec:
			plan.Commands = append(plan.Commands, typed...)
		case []any:
			for _, item := range typed {
				if entry, ok := item.(map[string]any); ok {
					plan.Commands = append(plan.Commands, verificationCommandSpec{
						Name:             stringValue(entry["name"]),
						Command:          stringValue(entry["command"]),
						Args:             uniqueStringsFromAny(entry["args"]),
						WorkingDirectory: stringValue(entry["working_directory"]),
					})
				}
			}
		}
		return plan
	}
	return verificationPlan{}
}

func verificationFiles(env euclotypes.ExecutionEnvelope) []string {
	files := []string{}
	for _, artifact := range euclotypes.ArtifactStateFromContext(env.State).OfKind(euclotypes.ArtifactKindEditIntent) {
		if record, ok := artifact.Payload.(map[string]any); ok {
			files = append(files, uniqueStringsFromAny(record["files"])...)
			if path := stringValue(record["path"]); path != "" {
				files = append(files, path)
			}
		}
	}
	if len(files) == 0 && env.Task != nil && env.Task.Context != nil {
		files = append(files, taskFilesFromContext(env.Task.Context["context_file_contents"])...)
	}
	return uniqueStrings(files)
}

func verificationTestFiles(files []string) []string {
	tests := make([]string, 0, len(files))
	for _, file := range files {
		path := strings.TrimSpace(file)
		if strings.Contains(path, "_test.") || strings.Contains(path, "/tests/") || strings.Contains(path, "/test/") {
			tests = append(tests, path)
		}
	}
	return uniqueStrings(tests)
}

func taskFilesFromContext(raw any) []string {
	switch typed := raw.(type) {
	case []map[string]any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if path := stringValue(item["path"]); path != "" {
				out = append(out, path)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				if path := stringValue(record["path"]); path != "" {
					out = append(out, path)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func explicitVerificationCommands(env euclotypes.ExecutionEnvelope, workspace string) []verificationCommandSpec {
	if env.Task == nil || env.Task.Context == nil {
		return nil
	}
	raw, ok := env.Task.Context["verification_commands"]
	if !ok || raw == nil {
		return nil
	}
	commands := []verificationCommandSpec{}
	for _, cmd := range uniqueStringsFromAny(raw) {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		commands = append(commands, verificationCommandSpec{
			Name:             parts[0],
			Command:          parts[0],
			Args:             parts[1:],
			WorkingDirectory: workspace,
		})
	}
	return commands
}

func verificationWorkspace(env euclotypes.ExecutionEnvelope) string {
	if env.State != nil {
		if value := strings.TrimSpace(env.State.GetString("euclo.workspace")); value != "" {
			return value
		}
	}
	if env.Task != nil && env.Task.Context != nil {
		if value := stringValue(env.Task.Context["workspace"]); value != "" {
			return value
		}
	}
	if env.Task != nil && env.Task.Context != nil {
		if value := stringValue(env.Task.Context["cwd"]); value != "" {
			return value
		}
	}
	return "."
}

func looksLikeGoScope(files []string, env euclotypes.ExecutionEnvelope) bool {
	for _, file := range files {
		if strings.HasSuffix(strings.TrimSpace(file), ".go") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(taskInstruction(env.Task)), "go")
}

func goVerificationCommands(files []string, workspace string, env euclotypes.ExecutionEnvelope) ([]verificationCommandSpec, string) {
	if !looksLikeGoScope(files, env) {
		return nil, ""
	}
	packages := goVerificationPackages(files)
	if len(packages) == 0 {
		return []verificationCommandSpec{{
			Name:             "go_test_all",
			Command:          "go",
			Args:             []string{"test", "./..."},
			WorkingDirectory: workspace,
		}}, "workspace_tests"
	}
	commands := make([]verificationCommandSpec, 0, len(packages))
	for _, pkg := range packages {
		name := "go_test_" + sanitizeVerificationName(pkg)
		commands = append(commands, verificationCommandSpec{
			Name:             name,
			Command:          "go",
			Args:             []string{"test", pkg},
			WorkingDirectory: workspace,
		})
	}
	return commands, "package_tests"
}

func goVerificationPackages(files []string) []string {
	pkgs := make([]string, 0, len(files))
	for _, file := range files {
		path := strings.TrimSpace(file)
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		switch dir {
		case "", ".":
			pkgs = append(pkgs, ".")
		default:
			pkgs = append(pkgs, "./"+strings.TrimPrefix(dir, "./"))
		}
	}
	return uniqueStrings(pkgs)
}

func sanitizeVerificationName(value string) string {
	replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	value = replacer.Replace(strings.TrimSpace(value))
	if value == "" {
		return "target"
	}
	return value
}

type verificationPolicySelection struct {
	PreferredVerifyCapabilities     []string
	VerificationSuccessCapabilities []string
	RequireVerificationStep         bool
}

func verificationPolicyHints(env euclotypes.ExecutionEnvelope) verificationPolicySelection {
	if env.State == nil {
		return verificationPolicySelection{}
	}
	raw, ok := env.State.Get("euclo.resolved_execution_policy")
	if !ok || raw == nil {
		return verificationPolicySelection{}
	}
	switch typed := raw.(type) {
	case eucloruntime.ResolvedExecutionPolicy:
		return verificationPolicySelection{
			PreferredVerifyCapabilities:     append([]string(nil), typed.PreferredVerifyCapabilities...),
			VerificationSuccessCapabilities: append([]string(nil), typed.VerificationSuccessCapabilities...),
			RequireVerificationStep:         typed.RequireVerificationStep,
		}
	case map[string]any:
		return verificationPolicySelection{
			PreferredVerifyCapabilities:     uniqueStringsFromAny(typed["preferred_verify_capabilities"]),
			VerificationSuccessCapabilities: uniqueStringsFromAny(typed["verification_success_capabilities"]),
			RequireVerificationStep:         boolValue(typed["require_verification_step"]),
		}
	default:
		return verificationPolicySelection{}
	}
}

func verificationPlanSource(base string, policy verificationPolicySelection) string {
	base = strings.TrimSpace(base)
	if len(policy.PreferredVerifyCapabilities) == 0 && len(policy.VerificationSuccessCapabilities) == 0 && !policy.RequireVerificationStep {
		return base
	}
	if base == "" {
		return "skill_policy"
	}
	return "skill_policy+" + base
}

func verificationSelectionInputs(files []string, policy verificationPolicySelection, selector string) []string {
	inputs := []string{selector}
	if len(files) > 0 {
		inputs = append(inputs, "changed_files")
	}
	if len(policy.PreferredVerifyCapabilities) > 0 {
		inputs = append(inputs, "policy_preferred_verify_capabilities")
	}
	if len(policy.VerificationSuccessCapabilities) > 0 {
		inputs = append(inputs, "policy_success_capabilities")
	}
	if policy.RequireVerificationStep {
		inputs = append(inputs, "policy_requires_verification_step")
	}
	return uniqueStrings(inputs)
}

func externalVerificationPlan(ctx context.Context, env euclotypes.ExecutionEnvelope, files, testFiles []string, workspace string, policy verificationPolicySelection) (verificationPlan, bool) {
	planner := env.Environment.VerificationPlanner
	if planner == nil {
		return verificationPlan{}, false
	}
	request := agentenv.VerificationPlanRequest{
		TaskInstruction:                 taskInstruction(env.Task),
		ModeID:                          strings.TrimSpace(env.Mode.ModeID),
		ProfileID:                       strings.TrimSpace(env.Profile.ProfileID),
		Workspace:                       workspace,
		Files:                           append([]string(nil), files...),
		TestFiles:                       append([]string(nil), testFiles...),
		PublicSurfaceChanged:            publicSurfaceChanged(env, files),
		PreferredVerifyCapabilities:     append([]string(nil), policy.PreferredVerifyCapabilities...),
		VerificationSuccessCapabilities: append([]string(nil), policy.VerificationSuccessCapabilities...),
		RequireVerificationStep:         policy.RequireVerificationStep,
	}
	planned, ok, err := planner.SelectVerificationPlan(ctx, request)
	if err != nil || !ok || len(planned.Commands) == 0 {
		return verificationPlan{}, false
	}
	commands := make([]verificationCommandSpec, 0, len(planned.Commands))
	for _, command := range planned.Commands {
		commands = append(commands, verificationCommandSpec{
			Name:             strings.TrimSpace(command.Name),
			Command:          strings.TrimSpace(command.Command),
			Args:             append([]string(nil), command.Args...),
			WorkingDirectory: strings.TrimSpace(command.WorkingDirectory),
		})
	}
	source := firstNonEmpty(strings.TrimSpace(planned.Source), "external_resolver")
	return verificationPlan{
		ScopeKind:                   firstNonEmpty(strings.TrimSpace(planned.ScopeKind), "external"),
		Files:                       uniqueStrings(append([]string(nil), planned.Files...)),
		TestFiles:                   uniqueStrings(append([]string(nil), planned.TestFiles...)),
		Commands:                    commands,
		Source:                      verificationPlanSource("external:"+source, policy),
		PlannerID:                   strings.TrimSpace(planned.PlannerID),
		Rationale:                   strings.TrimSpace(planned.Rationale),
		AuditTrail:                  uniqueStrings(append([]string(nil), planned.AuditTrail...)),
		CompatibilitySensitive:      planned.CompatibilitySensitive,
		PolicyPreferredCapabilities: policy.PreferredVerifyCapabilities,
		PolicySuccessCapabilities:   policy.VerificationSuccessCapabilities,
		PolicyRequiresVerification:  policy.RequireVerificationStep,
		SelectionInputs:             verificationSelectionInputs(files, policy, "external_resolver"),
		PlannerMetadata:             clonePlannerMetadata(planned.Metadata),
	}, true
}

func publicSurfaceChanged(env euclotypes.ExecutionEnvelope, files []string) bool {
	if env.Task != nil && env.Task.Context != nil {
		if verificationBoolValue(env.Task.Context["public_surface_changed"]) {
			return true
		}
	}
	for _, file := range files {
		path := strings.TrimSpace(strings.ToLower(file))
		if strings.Contains(path, "/public/") || strings.HasSuffix(path, "/api.go") || strings.HasSuffix(path, "/interface.go") {
			return true
		}
	}
	return false
}

func clonePlannerMetadata(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mapValue(v any) map[string]any {
	if typed, ok := v.(map[string]any); ok {
		return typed
	}
	return nil
}

func verificationBoolValue(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func commandStatus(err error) string {
	if err != nil {
		return "fail"
	}
	return "pass"
}

func verificationExecutionSummary(status string, plan verificationPlan) string {
	switch status {
	case "pass":
		return "verification executed successfully for scope " + firstNonEmpty(plan.ScopeKind, "targeted")
	default:
		return "verification failed for scope " + firstNonEmpty(plan.ScopeKind, "targeted")
	}
}

func verificationFlowError(result euclotypes.ExecutionResult) error {
	msg := strings.TrimSpace(result.Summary)
	if msg == "" && result.FailureInfo != nil {
		msg = strings.TrimSpace(result.FailureInfo.Message)
	}
	if msg == "" {
		msg = "verification capability failed"
	}
	return fmt.Errorf("%s", msg)
}
