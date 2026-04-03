package reflection

import (
	"context"
	"strings"
	"testing"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

type recordingLLM struct {
	lastPrompt string
	response   *core.LLMResponse
}

func (r *recordingLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	r.lastPrompt = prompt
	if r.response != nil {
		return r.response, nil
	}
	return &core.LLMResponse{Text: `{"issues":[],"approve":true}`}, nil
}

func (r *recordingLLM) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, nil
}

func (r *recordingLLM) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}

func (r *recordingLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}

func TestReflectionReviewNodeBoundsLargeResultPayloadInPrompt(t *testing.T) {
	reviewer := &recordingLLM{}
	agent := &ReflectionAgent{
		Reviewer: reviewer,
		Config:   &core.Config{Model: "test-model"},
	}
	node := &reflectionReviewNode{
		id:    "reflection_review",
		agent: agent,
		task:  &core.Task{Instruction: "Review the drafted answer"},
	}
	state := core.NewContext()
	hugeFiles := make([]any, 0, 200)
	for i := 0; i < 200; i++ {
		hugeFiles = append(hugeFiles, strings.Repeat("very/long/path/", 20))
	}
	state.Set("reflection.last_result", &core.Result{
		NodeID:  "reflection_execute",
		Success: false,
		Data: map[string]any{
			"final_output": map[string]any{
				"result": map[string]any{
					"file_list": map[string]any{
						"data": map[string]any{
							"files": hugeFiles,
						},
					},
				},
			},
		},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, reviewer.lastPrompt)
	assert.Less(t, len(reviewer.lastPrompt), 10000)
	assert.Contains(t, reviewer.lastPrompt, "...(truncated)")
}

func TestReflectionReviewGuidanceIncludesSkillPolicy(t *testing.T) {
	agent := &ReflectionAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						Criteria:  []string{"correctness", "completeness"},
						FocusTags: []string{"verification"},
						ApprovalRules: core.AgentReviewApprovalRules{
							RequireVerificationEvidence: true,
							RejectOnUnresolvedErrors:    true,
						},
						SeverityWeights: map[string]float64{"high": 1.0, "medium": 0.4, "low": 0.1},
					},
				},
			},
		},
	}
	task := &core.Task{Instruction: "Review result"}

	guidance := reflectionReviewGuidance(agent, task)
	assert.Contains(t, guidance, "Review criteria: correctness, completeness")
	assert.Contains(t, guidance, "Focus tags: verification")
	assert.Contains(t, guidance, "Require verification evidence before approval.")
	assert.Contains(t, guidance, "Reject outputs with unresolved errors.")
	assert.Contains(t, guidance, "Severity weights: high=1.00, medium=0.40, low=0.10")
}

func TestReflectionApprovalPassesRequiresVerificationEvidence(t *testing.T) {
	agent := &ReflectionAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						ApprovalRules: core.AgentReviewApprovalRules{
							RequireVerificationEvidence: true,
						},
					},
				},
			},
		},
	}
	state := core.NewContext()

	ok := reflectionApprovalPasses(agent, state, reviewPayload{Approve: true})
	assert.False(t, ok)
}

func TestReflectionApprovalPassesRejectsHighSeverityIssues(t *testing.T) {
	agent := &ReflectionAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						ApprovalRules: core.AgentReviewApprovalRules{
							RejectOnUnresolvedErrors: true,
						},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.tool_observations", []reactpkg.ToolObservation{
		{Tool: "go_test", Success: true},
	})
	review := reviewPayload{Approve: true}
	review.Issues = append(review.Issues, struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	}{
		Severity:    "high",
		Description: "failing case",
		Suggestion:  "fix it",
	})

	ok := reflectionApprovalPasses(agent, state, review)
	assert.False(t, ok)
}

func TestReflectionAssessmentBlocksMediumIssueAboveThreshold(t *testing.T) {
	agent := &ReflectionAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						SeverityWeights: map[string]float64{
							"high":   1.0,
							"medium": 0.6,
							"low":    0.2,
						},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.tool_observations", []reactpkg.ToolObservation{
		{Tool: "go_test", Success: true},
	})
	review := reviewPayload{Approve: true}
	review.Issues = append(review.Issues, struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	}{
		Severity:    "medium",
		Description: "missing edge case",
		Suggestion:  "add test",
	})

	assessment := reflectionAssessmentForReview(agent, state, review)

	assert.False(t, assessment.Allowed)
	assert.Equal(t, 0.6, assessment.IssueScore)
	assert.Equal(t, 0.2, assessment.ApprovalThreshold)
	assert.Contains(t, assessment.BlockingReasons[0], "weighted issue score")
}

func TestReflectionAssessmentAllowsSingleLowIssueWithinThreshold(t *testing.T) {
	agent := &ReflectionAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Review: core.AgentReviewPolicy{
						SeverityWeights: map[string]float64{
							"high":   1.0,
							"medium": 0.5,
							"low":    0.2,
						},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.tool_observations", []reactpkg.ToolObservation{
		{Tool: "go_test", Success: true},
	})
	review := reviewPayload{Approve: true}
	review.Issues = append(review.Issues, struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	}{
		Severity:    "low",
		Description: "minor naming nit",
		Suggestion:  "rename helper",
	})

	assessment := reflectionAssessmentForReview(agent, state, review)

	assert.True(t, assessment.Allowed)
	assert.Equal(t, 0.2, assessment.IssueScore)
	assert.Equal(t, 0.2, assessment.ApprovalThreshold)
}
