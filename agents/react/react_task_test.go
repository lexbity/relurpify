package react

import (
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

func TestFailureAnalysisReasonPrefersStderrThenStdoutThenError(t *testing.T) {
	t.Run("stderr first", func(t *testing.T) {
		reason := failureAnalysisReason(ToolObservation{
			Data: map[string]interface{}{
				"stderr": "stderr line\nmore",
				"stdout": "stdout line",
				"error":  "error line",
			},
		})
		assert.Equal(t, "stderr line", reason)
	})

	t.Run("stdout fallback", func(t *testing.T) {
		reason := failureAnalysisReason(ToolObservation{
			Data: map[string]interface{}{
				"stderr": "",
				"stdout": "stdout line\nmore",
				"error":  "error line",
			},
		})
		assert.Equal(t, "stdout line", reason)
	})

	t.Run("error fallback", func(t *testing.T) {
		reason := failureAnalysisReason(ToolObservation{
			Data: map[string]interface{}{
				"stderr": "",
				"stdout": "",
				"error":  "error line\nmore",
			},
		})
		assert.Equal(t, "error line", reason)
	})
}

func TestFailureAnalysisToolEligibleMatchesTestBuildAndCargoOnly(t *testing.T) {
	assert.True(t, failureAnalysisToolEligible("cli_cargo"))
	assert.True(t, failureAnalysisToolEligible("cli_go_test"))
	assert.True(t, failureAnalysisToolEligible("cli_build"))
	assert.False(t, failureAnalysisToolEligible("file_read"))
}

func TestAnalysisSummaryHelpersShareFailureReasonExtraction(t *testing.T) {
	analysisTask := &core.Task{Instruction: "Inspect the build failure"}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "cli_cargo",
			Success: true,
			Data: map[string]interface{}{
				"stderr": "error: failed to parse manifest\ncaused by: invalid syntax",
			},
		},
	})

	analysisSummary, ok := analysisSummaryFromFailure(analysisTask, state, map[string]interface{}{"error": "failed"})
	assert.True(t, ok)
	assert.True(t, strings.Contains(analysisSummary, "cli_cargo failed"))
	assert.True(t, strings.Contains(analysisSummary, "error: failed to parse manifest"))

	repeatedSummary, ok := repeatedFailureAnalysis(analysisTask, state, map[string]interface{}{"error": "failed"})
	assert.True(t, ok)
	assert.True(t, strings.Contains(repeatedSummary, "cli_cargo failed repeatedly"))
	assert.True(t, strings.Contains(repeatedSummary, "error: failed to parse manifest"))
}
