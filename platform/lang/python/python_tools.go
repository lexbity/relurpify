package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

var pythonProjectMarkers = []string{
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
	"requirements.txt",
	"pytest.ini",
	"tox.ini",
	"Pipfile",
}

type PythonWorkspaceDetectTool struct {
	BasePath string
	manager  *authorization.PermissionManager
	agentID  string
}

func (t *PythonWorkspaceDetectTool) Name() string { return "python_workspace_detect" }
func (t *PythonWorkspaceDetectTool) Description() string {
	return "Detects the nearest Python project root and marker files for a file or directory."
}
func (t *PythonWorkspaceDetectTool) Category() string { return "python" }
func (t *PythonWorkspaceDetectTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: false, Default: "."},
	}
}
func (t *PythonWorkspaceDetectTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}
func (t *PythonWorkspaceDetectTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
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
	projectRoot, manifestPath, markers := detectPythonProject(searchDir, t.BasePath)
	if projectRoot == "" {
		return &core.ToolResult{Success: false, Error: "no Python project markers found"}, nil
	}
	summary := fmt.Sprintf("Python project detected at %s", projectRoot)
	if manifestPath != "" {
		summary += " using " + filepath.Base(manifestPath)
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":          resolved,
			"project_root":  projectRoot,
			"manifest_path": manifestPath,
			"marker_files":  markers,
			"summary":       summary,
		},
	}, nil
}
func (t *PythonWorkspaceDetectTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *PythonWorkspaceDetectTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.BasePath, core.FileSystemRead)}
}
func (t *PythonWorkspaceDetectTool) Tags() []string {
	return []string{core.TagReadOnly, "lang:python", "workspace-detect", "recovery"}
}

type PythonProjectMetadataTool struct {
	BasePath string
	manager  *authorization.PermissionManager
	agentID  string
}

func (t *PythonProjectMetadataTool) Name() string { return "python_project_metadata" }
func (t *PythonProjectMetadataTool) Description() string {
	return "Reads Python project markers and returns structured project metadata."
}
func (t *PythonProjectMetadataTool) Category() string { return "python" }
func (t *PythonProjectMetadataTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: false, Default: "."},
	}
}
func (t *PythonProjectMetadataTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}
func (t *PythonProjectMetadataTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
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
	projectRoot, manifestPath, markers := detectPythonProject(resolved, t.BasePath)
	if projectRoot == "" {
		return &core.ToolResult{Success: false, Error: "no Python project markers found"}, nil
	}
	files := make(map[string]string)
	for _, marker := range markers {
		path := filepath.Join(projectRoot, marker)
		if t.manager != nil {
			if err := t.manager.CheckFileAccess(ctx, t.agentID, core.FileSystemRead, path); err != nil {
				return nil, err
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		files[marker] = string(content)
	}
	projectName, requiresPython := parsePythonProjectIdentity(files)
	testTool := inferPythonTestTool(files, markers)
	dependencyFiles := pythonDependencyFiles(markers)
	summary := fmt.Sprintf("Python project at %s", projectRoot)
	if projectName != "" {
		summary += " name=" + projectName
	}
	if testTool != "" {
		summary += " test_tool=" + testTool
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"summary":              summary,
			"project_root":         projectRoot,
			"manifest_path":        manifestPath,
			"marker_files":         markers,
			"project_name":         projectName,
			"requires_python":      requiresPython,
			"dependency_files":     dependencyFiles,
			"preferred_test_tool":  testTool,
			"has_pytest_config":    pythonHasPytestConfig(files, markers),
			"has_requirements_txt": containsString(markers, "requirements.txt"),
		},
	}, nil
}
func (t *PythonProjectMetadataTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *PythonProjectMetadataTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.BasePath, core.FileSystemRead)}
}
func (t *PythonProjectMetadataTool) Tags() []string {
	return []string{core.TagReadOnly, "lang:python", "metadata", "recovery"}
}

type PythonPytestTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewPythonPytestTool(basePath string) *PythonPytestTool {
	return &PythonPytestTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "python_pytest",
			Description: "Runs python -m pytest and returns structured Python test results.",
			Command:     "python3",
			Category:    "python",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *PythonPytestTool) Name() string { return "python_pytest" }
func (t *PythonPytestTool) Description() string {
	return "Runs python -m pytest and returns structured Python test results."
}
func (t *PythonPytestTool) Category() string { return "python" }
func (t *PythonPytestTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "test_path", Type: "string", Required: false},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *PythonPytestTool) SetCommandRunner(r sandbox.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *PythonPytestTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *PythonPytestTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *PythonPytestTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"-m", "pytest", "-q"}
	if raw, ok := args["test_path"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		commandArgs = append(commandArgs, fmt.Sprint(raw))
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
	summary := summarizePythonPytest(stdout, stderr, result.Success)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"passed":        summary.Passed,
			"failed":        summary.Failed,
			"errors":        summary.Errors,
			"first_failure": summary.FirstFailure,
			"stdout":        stdout,
			"stderr":        stderr,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *PythonPytestTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *PythonPytestTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *PythonPytestTool) Tags() []string {
	return []string{core.TagExecute, "lang:python", "test", "verification", "diagnostics"}
}

type PythonUnittestTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewPythonUnittestTool(basePath string) *PythonUnittestTool {
	return &PythonUnittestTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "python_unittest",
			Description: "Runs python -m unittest discover and returns structured Python test results.",
			Command:     "python3",
			Category:    "python",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *PythonUnittestTool) Name() string { return "python_unittest" }
func (t *PythonUnittestTool) Description() string {
	return "Runs python -m unittest discover and returns structured Python test results."
}
func (t *PythonUnittestTool) Category() string { return "python" }
func (t *PythonUnittestTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "start_directory", Type: "string", Required: false},
		{Name: "pattern", Type: "string", Required: false},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *PythonUnittestTool) SetCommandRunner(r sandbox.CommandRunner) { t.inner.SetCommandRunner(r) }
func (t *PythonUnittestTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *PythonUnittestTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *PythonUnittestTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"-m", "unittest", "discover"}
	if raw, ok := args["start_directory"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		commandArgs = append(commandArgs, "-s", fmt.Sprint(raw))
	}
	if raw, ok := args["pattern"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		commandArgs = append(commandArgs, "-p", fmt.Sprint(raw))
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
	summary := summarizePythonUnittest(stdout, stderr, result.Success)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"passed":        summary.Passed,
			"failed":        summary.Failed,
			"errors":        summary.Errors,
			"first_failure": summary.FirstFailure,
			"stdout":        stdout,
			"stderr":        stderr,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *PythonUnittestTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *PythonUnittestTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *PythonUnittestTool) Tags() []string {
	return []string{core.TagExecute, "lang:python", "test", "verification", "diagnostics"}
}

type PythonCompileCheckTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewPythonCompileCheckTool(basePath string) *PythonCompileCheckTool {
	return &PythonCompileCheckTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "python_compile_check",
			Description: "Runs python -m compileall and returns structured Python syntax-check results.",
			Command:     "python3",
			Category:    "python",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *PythonCompileCheckTool) Name() string { return "python_compile_check" }
func (t *PythonCompileCheckTool) Description() string {
	return "Runs python -m compileall and returns structured Python syntax-check results."
}
func (t *PythonCompileCheckTool) Category() string { return "python" }
func (t *PythonCompileCheckTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "target", Type: "string", Required: false, Default: "."},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *PythonCompileCheckTool) SetCommandRunner(r sandbox.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *PythonCompileCheckTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *PythonCompileCheckTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *PythonCompileCheckTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	target := "."
	if raw, ok := args["target"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		target = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"-m", "compileall", "-q", target}
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
	summary := summarizePythonCompileCheck(stdout, stderr, result.Success)
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
func (t *PythonCompileCheckTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *PythonCompileCheckTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *PythonCompileCheckTool) Tags() []string {
	return []string{core.TagExecute, "lang:python", "syntax-check", "verification", "diagnostics"}
}

type pythonTestSummary struct {
	Summary      string
	Passed       int
	Failed       int
	Errors       int
	FirstFailure string
}

type pythonCompileSummary struct {
	Summary      string
	ErrorCount   int
	FirstMessage string
}

var (
	pythonPytestFailedPattern  = regexp.MustCompile(`(?m)^FAILED\s+(\S+)`)
	pythonPytestErrorPattern   = regexp.MustCompile(`(?m)^ERROR\s+(\S+)`)
	pythonUnittestCountPattern = regexp.MustCompile(`(?m)^Ran\s+(\d+)\s+tests?\s+in\s+`)
	pythonCompileErrorPattern  = regexp.MustCompile(`(?mi)(SyntaxError:.*|IndentationError:.*|NameError:.*)`)
	pythonProjectNamePattern   = regexp.MustCompile(`(?m)^\s*name\s*=\s*["']([^"']+)["']`)
	pythonRequiresPattern      = regexp.MustCompile(`(?m)^\s*requires-python\s*=\s*["']([^"']+)["']`)
	pythonPassedPattern        = regexp.MustCompile(`(\d+)\s+passed`)
	pythonFailedPattern        = regexp.MustCompile(`(\d+)\s+failed`)
	pythonErrorsPattern        = regexp.MustCompile(`(\d+)\s+error[s]?`)
)

func summarizePythonPytest(stdout, stderr string, success bool) pythonTestSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := pythonTestSummary{Summary: "pytest completed"}
	if match := pythonPassedPattern.FindStringSubmatch(combined); len(match) > 1 {
		summary.Passed = atoiSafe(match[1])
	}
	if match := pythonFailedPattern.FindStringSubmatch(combined); len(match) > 1 {
		summary.Failed = atoiSafe(match[1])
	}
	if match := pythonErrorsPattern.FindStringSubmatch(combined); len(match) > 1 {
		summary.Errors = atoiSafe(match[1])
	}
	if matches := pythonPytestFailedPattern.FindAllStringSubmatch(combined, -1); len(matches) > 0 && len(matches[0]) > 1 {
		summary.FirstFailure = matches[0][1]
	}
	if summary.FirstFailure == "" {
		if matches := pythonPytestErrorPattern.FindAllStringSubmatch(combined, -1); len(matches) > 0 && len(matches[0]) > 1 {
			summary.FirstFailure = matches[0][1]
		}
	}
	if success {
		summary.Summary = fmt.Sprintf("pytest passed: %d passed, %d failed, %d errors", summary.Passed, summary.Failed, summary.Errors)
		return summary
	}
	line := firstNonEmptyLine(stderr)
	if line == "" {
		line = firstNonEmptyLine(stdout)
	}
	if summary.FirstFailure != "" {
		summary.Summary = "pytest failed: " + summary.FirstFailure
	} else if line != "" {
		summary.Summary = "pytest failed: " + line
	}
	return summary
}

func summarizePythonUnittest(stdout, stderr string, success bool) pythonTestSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := pythonTestSummary{Summary: "unittest completed"}
	if match := pythonUnittestCountPattern.FindStringSubmatch(combined); len(match) > 1 {
		total := atoiSafe(match[1])
		summary.Passed = total
	}
	for _, line := range strings.Split(combined, "\n") {
		text := strings.TrimSpace(line)
		lower := strings.ToLower(text)
		switch {
		case strings.HasPrefix(lower, "fail:"):
			summary.Failed++
			if summary.FirstFailure == "" {
				summary.FirstFailure = strings.TrimSpace(text[5:])
			}
		case strings.HasPrefix(lower, "error:"):
			summary.Errors++
			if summary.FirstFailure == "" {
				summary.FirstFailure = strings.TrimSpace(text[6:])
			}
		}
	}
	if summary.Passed > 0 {
		summary.Passed -= summary.Failed + summary.Errors
		if summary.Passed < 0 {
			summary.Passed = 0
		}
	}
	if success {
		summary.Summary = fmt.Sprintf("unittest passed: %d passed, %d failed, %d errors", summary.Passed, summary.Failed, summary.Errors)
		return summary
	}
	line := firstNonEmptyLine(stderr)
	if line == "" {
		line = firstNonEmptyLine(stdout)
	}
	if summary.FirstFailure != "" {
		summary.Summary = "unittest failed: " + summary.FirstFailure
	} else if line != "" {
		summary.Summary = "unittest failed: " + line
	}
	return summary
}

func summarizePythonCompileCheck(stdout, stderr string, success bool) pythonCompileSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := pythonCompileSummary{Summary: "compile check completed"}
	for _, match := range pythonCompileErrorPattern.FindAllStringSubmatch(combined, -1) {
		if len(match) < 2 {
			continue
		}
		summary.ErrorCount++
		if summary.FirstMessage == "" {
			summary.FirstMessage = strings.TrimSpace(match[1])
		}
	}
	if success {
		summary.Summary = fmt.Sprintf("compile check passed: %d errors", summary.ErrorCount)
		return summary
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = firstNonEmptyLine(stderr)
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = firstNonEmptyLine(stdout)
	}
	if summary.FirstMessage != "" {
		summary.Summary = "compile check failed: " + summary.FirstMessage
	}
	return summary
}

func detectPythonProject(startDir, basePath string) (string, string, []string) {
	basePath = filepath.Clean(basePath)
	current := filepath.Clean(startDir)
	for {
		markers := findPythonMarkers(current)
		if len(markers) > 0 {
			return current, filepath.Join(current, markers[0]), markers
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
	return "", "", nil
}

func findPythonMarkers(dir string) []string {
	markers := make([]string, 0, len(pythonProjectMarkers))
	for _, marker := range pythonProjectMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			markers = append(markers, marker)
		}
	}
	return markers
}

func parsePythonProjectIdentity(files map[string]string) (string, string) {
	if content := files["pyproject.toml"]; content != "" {
		name := firstCapture(pythonProjectNamePattern, content)
		requires := firstCapture(pythonRequiresPattern, content)
		if name != "" || requires != "" {
			return name, requires
		}
	}
	if content := files["setup.cfg"]; content != "" {
		name := firstCapture(pythonProjectNamePattern, content)
		if name != "" {
			return name, ""
		}
	}
	return "", ""
}

func inferPythonTestTool(files map[string]string, markers []string) string {
	if pythonHasPytestConfig(files, markers) {
		return "python_pytest"
	}
	return "python_unittest"
}

func pythonHasPytestConfig(files map[string]string, markers []string) bool {
	if containsString(markers, "pytest.ini") {
		return true
	}
	if content := files["pyproject.toml"]; strings.Contains(content, "[tool.pytest") {
		return true
	}
	if content := files["requirements.txt"]; strings.Contains(strings.ToLower(content), "pytest") {
		return true
	}
	return false
}

func pythonDependencyFiles(markers []string) []string {
	var files []string
	for _, marker := range markers {
		switch marker {
		case "requirements.txt", "Pipfile", "pyproject.toml", "setup.py", "setup.cfg":
			files = append(files, marker)
		}
	}
	return files
}

func firstCapture(pattern *regexp.Regexp, text string) string {
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
