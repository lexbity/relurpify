package shell

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestExecutionHelpersAreExplicitlyShaped(t *testing.T) {
	cases := []struct {
		name string
		tool interface {
			Name() string
			Description() string
			Category() string
			Parameters() []core.ToolParameter
			Tags() []string
			IsAvailable(context.Context, *core.Context) bool
		}
		wantName string
		wantDesc string
		wantTags []string
		wantArgs []core.ToolParameter
	}{
		{
			name:     "tests",
			tool:     &RunTestsTool{},
			wantName: "exec_run_tests",
			wantDesc: "Runs project tests.",
			wantTags: []string{core.TagExecute, "test", "verification"},
			wantArgs: []core.ToolParameter{{Name: "pattern", Type: "string", Required: false}},
		},
		{
			name:     "code",
			tool:     &ExecuteCodeTool{},
			wantName: "exec_run_code",
			wantDesc: "Executes arbitrary code snippets in a sandbox.",
			wantTags: []string{core.TagExecute, "code"},
			wantArgs: []core.ToolParameter{{Name: "code", Type: "string", Required: true}},
		},
		{
			name:     "linter",
			tool:     &RunLinterTool{},
			wantName: "exec_run_linter",
			wantDesc: "Runs linting tools.",
			wantTags: []string{core.TagExecute, "lint", "verification"},
			wantArgs: []core.ToolParameter{{Name: "path", Type: "string", Required: false}},
		},
		{
			name:     "build",
			tool:     &RunBuildTool{},
			wantName: "exec_run_build",
			wantDesc: "Runs builds or compiles the project.",
			wantTags: []string{core.TagExecute, "build", "verification"},
			wantArgs: []core.ToolParameter{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantName, tc.tool.Name())
			require.Equal(t, tc.wantDesc, tc.tool.Description())
			require.Equal(t, "execution", tc.tool.Category())
			require.Equal(t, tc.wantTags, tc.tool.Tags())
			require.Equal(t, tc.wantArgs, tc.tool.Parameters())
			require.False(t, tc.tool.IsAvailable(context.Background(), core.NewContext()))
		})
	}
}
