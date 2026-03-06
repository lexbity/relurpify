package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

var goProjectMarkers = []string{
	"go.mod",
	"go.work",
}

type GoWorkspaceDetectTool struct {
	BasePath string
	manager  *runtime.PermissionManager
	agentID  string
}

func (t *GoWorkspaceDetectTool) Name() string { return "go_workspace_detect" }
func (t *GoWorkspaceDetectTool) Description() string {
	return "Detects the nearest Go module or workspace for a file or directory."
}
func (t *GoWorkspaceDetectTool) Category() string { return "go" }
func (t *GoWorkspaceDetectTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: false, Default: "."}}
}
func (t *GoWorkspaceDetectTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}
func (t *GoWorkspaceDetectTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	start := "."
	if raw, ok := args["path"]; ok && raw != nil {
		start = strings.TrimSpace(fmt.Sprint(raw))
		if start == "" || start == "<nil>" {
			start = "."
		}
	}
	resolved := start
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(t.BasePath, resolved)
	}
	resolved = filepath.Clean(resolved)
	if t.manager != nil {
		if err := t.manager.CheckFileAccess(ctx, t.agentID, core.FileSystemRead, resolved); err != nil {
			return nil, err
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return &core.ToolResult{Success: false, Error: err.Error()}, nil
	}
	searchDir := resolved
	if !info.IsDir() {
		searchDir = filepath.Dir(resolved)
	}
	moduleRoot, modulePath, workspacePath := detectGoProject(searchDir, t.BasePath)
	if moduleRoot == "" {
		return &core.ToolResult{Success: false, Error: "no Go module or workspace found"}, nil
	}
	summary := fmt.Sprintf("Go module detected at %s", moduleRoot)
	if workspacePath != "" {
		summary += " workspace=" + workspacePath
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":           resolved,
			"module_root":    moduleRoot,
			"module_path":    modulePath,
			"workspace_path": workspacePath,
			"summary":        summary,
		},
	}, nil
}
func (t *GoWorkspaceDetectTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *GoWorkspaceDetectTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.BasePath, core.FileSystemRead)}
}
func (t *GoWorkspaceDetectTool) Tags() []string {
	return []string{core.TagReadOnly, "lang:go", "workspace-detect", "recovery"}
}

type GoModuleMetadataTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewGoModuleMetadataTool(basePath string) *GoModuleMetadataTool {
	return &GoModuleMetadataTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "go_module_metadata",
			Description: "Runs go list -m -json and returns structured Go module metadata.",
			Command:     "go",
			Category:    "go",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *GoModuleMetadataTool) Name() string { return "go_module_metadata" }
func (t *GoModuleMetadataTool) Description() string {
	return "Runs go list -m -json and returns structured Go module metadata."
}
func (t *GoModuleMetadataTool) Category() string { return "go" }
func (t *GoModuleMetadataTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false, Default: "."}}
}
func (t *GoModuleMetadataTool) SetCommandRunner(r runtime.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *GoModuleMetadataTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *GoModuleMetadataTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *GoModuleMetadataTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args":              []interface{}{"list", "-m", "-json"},
		"working_directory": workingDir,
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	summary, parsed := parseGoModuleMetadata(stdout)
	data := map[string]interface{}{
		"summary": summary,
		"stdout":  stdout,
		"stderr":  stderr,
	}
	for key, value := range parsed {
		data[key] = value
	}
	return &core.ToolResult{Success: result.Success, Error: result.Error, Data: data, Metadata: result.Metadata}, nil
}
func (t *GoModuleMetadataTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *GoModuleMetadataTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *GoModuleMetadataTool) Tags() []string {
	return []string{core.TagExecute, "lang:go", "metadata", "recovery"}
}

type GoTestTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewGoTestTool(basePath string) *GoTestTool {
	return &GoTestTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "go_test",
			Description: "Runs go test and returns structured Go test results.",
			Command:     "go",
			Category:    "go",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *GoTestTool) Name() string { return "go_test" }
func (t *GoTestTool) Description() string {
	return "Runs go test and returns structured Go test results."
}
func (t *GoTestTool) Category() string { return "go" }
func (t *GoTestTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "package", Type: "string", Required: false, Default: "./..."},
		{Name: "run", Type: "string", Required: false},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *GoTestTool) SetCommandRunner(r runtime.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *GoTestTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *GoTestTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *GoTestTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	pkg := "./..."
	if raw, ok := args["package"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		pkg = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"test", pkg}
	if raw, ok := args["run"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		commandArgs = append(commandArgs, "-run", fmt.Sprint(raw))
	}
	if raw, ok := args["extra_args"]; ok && raw != nil {
		if extra, err := toStringSliceValue(raw); err == nil {
			for _, entry := range extra {
				commandArgs = append(commandArgs, entry)
			}
		}
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args":              commandArgs,
		"working_directory": workingDir,
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	summary := summarizeGoTest(stdout, stderr, result.Success)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"passed":        summary.Passed,
			"failed":        summary.Failed,
			"failed_tests":  summary.FailedTests,
			"first_failure": summary.FirstFailure,
			"stdout":        stdout,
			"stderr":        stderr,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *GoTestTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *GoTestTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *GoTestTool) Tags() []string {
	return []string{core.TagExecute, "lang:go", "test", "verification", "diagnostics"}
}

type GoBuildTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewGoBuildTool(basePath string) *GoBuildTool {
	return &GoBuildTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "go_build",
			Description: "Runs go build and returns structured Go build results.",
			Command:     "go",
			Category:    "go",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *GoBuildTool) Name() string { return "go_build" }
func (t *GoBuildTool) Description() string {
	return "Runs go build and returns structured Go build results."
}
func (t *GoBuildTool) Category() string { return "go" }
func (t *GoBuildTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "package", Type: "string", Required: false, Default: "./..."},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *GoBuildTool) SetCommandRunner(r runtime.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *GoBuildTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *GoBuildTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *GoBuildTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	pkg := "./..."
	if raw, ok := args["package"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		pkg = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"build", pkg}
	if raw, ok := args["extra_args"]; ok && raw != nil {
		if extra, err := toStringSliceValue(raw); err == nil {
			for _, entry := range extra {
				commandArgs = append(commandArgs, entry)
			}
		}
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args":              commandArgs,
		"working_directory": workingDir,
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	summary := summarizeGoBuild(stdout, stderr, result.Success)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"error_count":   summary.ErrorCount,
			"first_message": summary.FirstMessage,
			"stdout":        stdout,
			"stderr":        stderr,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *GoBuildTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *GoBuildTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *GoBuildTool) Tags() []string {
	return []string{core.TagExecute, "lang:go", "build", "verification", "diagnostics"}
}

type goTestSummary struct {
	Summary      string
	Passed       int
	Failed       int
	FailedTests  []string
	FirstFailure string
}

type goBuildSummary struct {
	Summary      string
	ErrorCount   int
	FirstMessage string
}

var (
	goFailedTestPattern = regexp.MustCompile(`(?m)^--- FAIL: ([^\s]+)`)
	goPassPattern       = regexp.MustCompile(`(?m)^ok\s+`)
	goErrorLinePattern  = regexp.MustCompile(`(?m)^.+:\d+(?::\d+)?:\s+.+$`)
)

func summarizeGoTest(stdout, stderr string, success bool) goTestSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := goTestSummary{Summary: "go test completed"}
	for _, match := range goFailedTestPattern.FindAllStringSubmatch(combined, -1) {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		summary.FailedTests = append(summary.FailedTests, name)
	}
	summary.Failed = len(summary.FailedTests)
	summary.Passed = len(goPassPattern.FindAllString(combined, -1))
	if len(summary.FailedTests) > 0 {
		summary.FirstFailure = summary.FailedTests[0]
	}
	if success {
		summary.Summary = fmt.Sprintf("go test passed: %d packages ok, %d failed tests", summary.Passed, summary.Failed)
		return summary
	}
	if summary.FirstFailure != "" {
		summary.Summary = fmt.Sprintf("go test failed: %s", summary.FirstFailure)
		return summary
	}
	line := firstNonEmptyLine(stderr)
	if line == "" {
		line = firstNonEmptyLine(stdout)
	}
	if line != "" {
		summary.Summary = "go test failed: " + line
	}
	return summary
}

func summarizeGoBuild(stdout, stderr string, success bool) goBuildSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := goBuildSummary{Summary: "go build completed"}
	for _, match := range goErrorLinePattern.FindAllString(combined, -1) {
		summary.ErrorCount++
		if summary.FirstMessage == "" {
			summary.FirstMessage = strings.TrimSpace(match)
		}
	}
	if success {
		summary.Summary = fmt.Sprintf("go build passed: %d errors", summary.ErrorCount)
		return summary
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = firstNonEmptyLine(stderr)
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = firstNonEmptyLine(stdout)
	}
	if summary.FirstMessage != "" {
		summary.Summary = "go build failed: " + summary.FirstMessage
	}
	return summary
}

func detectGoProject(startDir, basePath string) (string, string, string) {
	basePath = filepath.Clean(basePath)
	current := filepath.Clean(startDir)
	moduleRoot := ""
	workspacePath := ""
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil && moduleRoot == "" {
			moduleRoot = current
		}
		if _, err := os.Stat(filepath.Join(current, "go.work")); err == nil && workspacePath == "" {
			workspacePath = filepath.Join(current, "go.work")
		}
		if current == basePath {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if moduleRoot == "" && workspacePath != "" {
		moduleRoot = filepath.Dir(workspacePath)
	}
	modulePath := ""
	if moduleRoot != "" {
		modulePath = filepath.Join(moduleRoot, "go.mod")
	}
	return moduleRoot, modulePath, workspacePath
}

func parseGoModuleMetadata(stdout string) (string, map[string]interface{}) {
	type goModule struct {
		Path      string `json:"Path"`
		Dir       string `json:"Dir"`
		GoMod     string `json:"GoMod"`
		GoVersion string `json:"GoVersion"`
		Main      bool   `json:"Main"`
	}
	var payload goModule
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return "go module metadata completed", map[string]interface{}{}
	}
	summary := fmt.Sprintf("go module %s", payload.Path)
	if payload.GoVersion != "" {
		summary += " go=" + payload.GoVersion
	}
	return summary, map[string]interface{}{
		"module_name": payload.Path,
		"module_dir":  payload.Dir,
		"go_mod":      payload.GoMod,
		"go_version":  payload.GoVersion,
		"is_main":     payload.Main,
	}
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}
