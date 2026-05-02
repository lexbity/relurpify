package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// BisectHandler implements the git bisect capability.
type BisectHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

// NewBisectHandler creates a new bisect handler.
func NewBisectHandler(env agentenv.WorkspaceEnvironment) *BisectHandler {
	return &BisectHandler{env: env}
}

// Descriptor returns the capability descriptor for the bisect handler.
func (h *BisectHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.bisect",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Bisect",
		Version:       "1.0.0",
		Description:   "Performs git bisect to find the commit that introduced a bug",
		Category:      "git",
		Tags:          []string{"git", "bisect", "read-only"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"good_ref": {
					Type:        "string",
					Description: "Git ref known to be good (e.g., commit hash, tag)",
				},
				"bad_ref": {
					Type:        "string",
					Description: "Git ref known to be bad (e.g., commit hash, tag)",
				},
				"test_command": {
					Type:        "string",
					Description: "Command to run to test if current state is good or bad",
				},
				"max_steps": {
					Type:        "integer",
					Description: "Maximum number of bisect steps (default: 30)",
				},
			},
			Required: []string{"good_ref", "bad_ref", "test_command"},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if bisect completed successfully",
				},
				"found": {
					Type:        "boolean",
					Description: "True if culprit commit was found",
				},
				"culprit_commit": {
					Type:        "string",
					Description: "The commit hash that introduced the bug",
				},
				"steps_taken": {
					Type:        "integer",
					Description: "Number of bisect steps taken",
				},
			},
		},
	}
}

// Invoke executes git bisect to find the culprit commit.
func (h *BisectHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	// Extract arguments
	goodRef, ok := stringArg(args, "good_ref")
	if !ok || goodRef == "" {
		return failResult("good_ref argument is required and must be non-empty"), nil
	}

	badRef, ok := stringArg(args, "bad_ref")
	if !ok || badRef == "" {
		return failResult("bad_ref argument is required and must be non-empty"), nil
	}

	testCommand, ok := stringArg(args, "test_command")
	if !ok || testCommand == "" {
		return failResult("test_command argument is required and must be non-empty"), nil
	}

	maxSteps, _ := intArg(args, "max_steps", 30)

	// Check for CommandRunner
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	if h.env.Model != nil {
		if result, err := h.runReactiveBisect(ctx, goodRef, badRef, testCommand, maxSteps); err == nil {
			return result, nil
		}
	}

	return h.runDeterministicBisect(ctx, goodRef, badRef, testCommand, maxSteps)
}

func (h *BisectHandler) runDeterministicBisect(ctx context.Context, goodRef, badRef, testCommand string, maxSteps int) (*contracts.CapabilityExecutionResult, error) {
	workdir := workspacePath(h.env)
	if err := h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "start"}); err != nil {
		return failResult(fmt.Sprintf("failed to start bisect: %v", err)), nil
	}
	if err := h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "good", goodRef}); err != nil {
		return failResult(fmt.Sprintf("failed to mark good ref: %v", err)), nil
	}
	if err := h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "bad", badRef}); err != nil {
		return failResult(fmt.Sprintf("failed to mark bad ref: %v", err)), nil
	}

	stepsTaken := 0
	var culpritCommit string
	found := false
	for stepsTaken < maxSteps {
		status, err := h.runBisectStatus(ctx, workdir)
		if err != nil {
			return failResult(fmt.Sprintf("failed to check bisect status: %v", err)), nil
		}
		if culprit, ok := parseBisectCulprit(status); ok {
			culpritCommit = culprit
			found = true
			break
		}

		stdout, stderr, err := h.runTestCommand(ctx, workdir, testCommand)
		if err != nil {
			return failResult(fmt.Sprintf("failed to run test command: %v", err)), nil
		}
		mark := "bad"
		if strings.TrimSpace(stderr) == "" {
			mark = "good"
		}
		resultOut, err := h.runBisectMark(ctx, workdir, mark)
		if err != nil {
			return failResult(fmt.Sprintf("failed to mark %s: %v", mark, err)), nil
		}
		if culprit, ok := parseBisectCulprit(resultOut); ok {
			culpritCommit = culprit
			found = true
			_ = stdout
			break
		}
		stepsTaken++
	}

	_ = h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "reset"})

	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":        true,
			"found":          found,
			"culprit_commit": culpritCommit,
			"steps_taken":    stepsTaken,
		},
	}, nil
}

func (h *BisectHandler) runReactiveBisect(ctx context.Context, goodRef, badRef, testCommand string, maxSteps int) (*contracts.CapabilityExecutionResult, error) {
	// The model-guided path uses the same command primitives as the
	// deterministic fallback, but lets the model decide which action to take
	// next. If the model path fails to produce a valid plan, the caller falls
	// back to the deterministic loop.
	workdir := workspacePath(h.env)
	state := bisectSessionState{
		GoodRef:     goodRef,
		BadRef:      badRef,
		TestCommand: testCommand,
		MaxSteps:    maxSteps,
	}

	if err := h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "start"}); err != nil {
		return nil, err
	}
	if _, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "good", goodRef}); err != nil {
		return nil, err
	}
	if _, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "bad", badRef}); err != nil {
		return nil, err
	}

	for state.StepsTaken < maxSteps {
		decision, err := h.nextBisectDecision(ctx, state)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(decision.Tool)) {
		case "run_test":
			stdout, stderr, err := h.runTestCommand(ctx, workdir, testCommand)
			if err != nil {
				return failResult(fmt.Sprintf("failed to run test command: %v", err)), nil
			}
			state.LastTestStdout = stdout
			state.LastTestStderr = stderr
			state.LastTestPassed = strings.TrimSpace(stderr) == ""
		case "mark_good":
			out, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "good"})
			if err != nil {
				return failResult(fmt.Sprintf("failed to mark good: %v", err)), nil
			}
			state.Log = append(state.Log, out)
			if culprit, ok := parseBisectCulprit(out); ok {
				state.CulpritCommit = culprit
				state.Found = true
			}
		case "mark_bad":
			out, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "bad"})
			if err != nil {
				return failResult(fmt.Sprintf("failed to mark bad: %v", err)), nil
			}
			state.Log = append(state.Log, out)
			if culprit, ok := parseBisectCulprit(out); ok {
				state.CulpritCommit = culprit
				state.Found = true
			}
		case "check_result":
			out, err := h.runBisectStatus(ctx, workdir)
			if err != nil {
				return failResult(fmt.Sprintf("failed to check bisect status: %v", err)), nil
			}
			state.Log = append(state.Log, out)
			if culprit, ok := parseBisectCulprit(out); ok {
				state.CulpritCommit = culprit
				state.Found = true
			}
		case "reset":
			_ = h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "reset"})
			goto done
		case "complete":
			goto done
		default:
			// If the model doesn't give us a valid tool name, fall back to a
			// deterministic bisect step so the handler still makes progress.
			if state.LastTestPassed {
				out, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "good"})
				if err != nil {
					return failResult(fmt.Sprintf("failed to mark good: %v", err)), nil
				}
				state.Log = append(state.Log, out)
				if culprit, ok := parseBisectCulprit(out); ok {
					state.CulpritCommit = culprit
					state.Found = true
				}
			} else {
				out, err := h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "bad"})
				if err != nil {
					return failResult(fmt.Sprintf("failed to mark bad: %v", err)), nil
				}
				state.Log = append(state.Log, out)
				if culprit, ok := parseBisectCulprit(out); ok {
					state.CulpritCommit = culprit
					state.Found = true
				}
			}
		}
		state.StepsTaken++
		if state.Found {
			break
		}
	}

done:
	_ = h.runBisectCommand(ctx, workdir, []string{"git", "bisect", "reset"})
	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":        true,
			"found":          state.Found,
			"culprit_commit": state.CulpritCommit,
			"steps_taken":    state.StepsTaken,
			"bisect_log":     strings.Join(state.Log, "\n"),
		},
	}, nil
}

type bisectDecision struct {
	Thought   string                 `json:"thought"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Complete  bool                   `json:"complete"`
	Summary   string                 `json:"summary"`
	Reason    string                 `json:"reason"`
}

type bisectSessionState struct {
	GoodRef        string
	BadRef         string
	TestCommand    string
	MaxSteps       int
	StepsTaken     int
	LastTestPassed bool
	LastTestStdout string
	LastTestStderr string
	CulpritCommit  string
	Found          bool
	Log            []string
}

func (h *BisectHandler) nextBisectDecision(ctx context.Context, state bisectSessionState) (bisectDecision, error) {
	if h.env.Model == nil {
		return bisectDecision{}, nil
	}
	prompt := fmt.Sprintf(`You are orchestrating git bisect.
Use only these tools: run_test, mark_good, mark_bad, check_result, reset, complete.
Current state:
- good_ref: %s
- bad_ref: %s
- test_command: %s
- steps_taken: %d
- max_steps: %d
- last_test_passed: %t
- culprit_commit: %s
- found: %t
Return JSON only:
{"thought":"...","tool":"run_test|mark_good|mark_bad|check_result|reset|complete","arguments":{},"complete":bool,"summary":"..."}`,
		state.GoodRef, state.BadRef, state.TestCommand, state.StepsTaken, state.MaxSteps, state.LastTestPassed, state.CulpritCommit, state.Found)
	resp, err := h.env.Model.Generate(ctx, prompt, &contracts.LLMOptions{
		Model:       configuredModelName(h.env.Config),
		Temperature: 0,
		MaxTokens:   256,
	})
	if err != nil {
		return bisectDecision{}, err
	}
	var decision bisectDecision
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(resp.Text)), &decision); err != nil {
		return bisectDecision{}, err
	}
	return decision, nil
}

func (h *BisectHandler) runBisectCommand(ctx context.Context, workdir string, args []string) error {
	_, err := h.runBisectCommandOutput(ctx, workdir, args)
	return err
}

func (h *BisectHandler) runBisectCommandOutput(ctx context.Context, workdir string, args []string) (string, error) {
	req := sandbox.CommandRequest{Args: args, Workdir: workdir}
	if err := h.authorizeCommand(ctx, h.env, req, "euclo bisect"); err != nil {
		return "", err
	}
	stdout, stderr, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil {
		if stderr != "" {
			return stdout + stderr, err
		}
		return stdout, err
	}
	if stderr != "" {
		return stdout + stderr, nil
	}
	return stdout, nil
}

func (h *BisectHandler) runBisectStatus(ctx context.Context, workdir string) (string, error) {
	return h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "status"})
}

func (h *BisectHandler) runBisectMark(ctx context.Context, workdir, mark string) (string, error) {
	if mark == "good" {
		return h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "good"})
	}
	return h.runBisectCommandOutput(ctx, workdir, []string{"git", "bisect", "bad"})
}

func (h *BisectHandler) runTestCommand(ctx context.Context, workdir, testCommand string) (string, string, error) {
	req := sandbox.CommandRequest{
		Args:    []string{"sh", "-c", testCommand},
		Workdir: workdir,
	}
	if err := h.authorizeCommand(ctx, h.env, req, "euclo bisect"); err != nil {
		return "", "", err
	}
	return h.env.CommandRunner.Run(ctx, req)
}

func parseBisectCulprit(output string) (string, bool) {
	if strings.TrimSpace(output) == "" {
		return "", false
	}
	if !strings.Contains(strings.ToLower(output), "first bad commit") {
		return "", false
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.Contains(strings.ToLower(line), "first bad commit") {
			continue
		}
		fields := strings.Fields(line)
		for i, field := range fields {
			if strings.EqualFold(field, "commit:") && i+1 < len(fields) {
				return strings.TrimSpace(fields[i+1]), true
			}
		}
		if len(fields) >= 4 {
			return strings.TrimSpace(fields[len(fields)-1]), true
		}
	}
	return "", false
}

func workspacePath(env agentenv.WorkspaceEnvironment) string {
	if env.IndexManager != nil {
		if path := strings.TrimSpace(env.IndexManager.WorkspacePath()); path != "" {
			return path
		}
	}
	return "."
}
