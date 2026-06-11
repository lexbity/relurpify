package js

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	lang "codeburg.org/lexbit/relurpify/platform/lang"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

var nodeProjectMarkers = []string{
	"package.json",
	"package-lock.json",
	"pnpm-lock.yaml",
	"yarn.lock",
	"tsmanifest.json",
	"jsmanifest.json",
}

type NodeWorkspaceDetectTool struct {
	BasePath string
}

func (t *NodeWorkspaceDetectTool) Name() string { return "node_workspace_detect" }
func (t *NodeWorkspaceDetectTool) Description() string {
	return "Detects the nearest Node.js/JavaScript project root and marker files for a file or directory."
}
func (t *NodeWorkspaceDetectTool) Category() string { return "node" }
func (t *NodeWorkspaceDetectTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{{Name: "path", Type: "string", Required: false, Default: "."}}
}
func (t *NodeWorkspaceDetectTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
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
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	searchDir := resolved
	if !info.IsDir() {
		searchDir = filepath.Dir(resolved)
	}
	projectRoot, manifestPath, markers := detectNodeProject(searchDir, t.BasePath)
	if projectRoot == "" {
		return &ports.ToolResult{Success: false, Error: "no Node project markers found"}, nil
	}
	pm := inferNodePackageManager(markers)
	summary := fmt.Sprintf("Node project detected at %s", projectRoot)
	if manifestPath != "" {
		summary += " using " + filepath.Base(manifestPath)
	}
	if pm != "" {
		summary += " package_manager=" + pm
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"path":            resolved,
			"project_root":    projectRoot,
			"manifest_path":   manifestPath,
			"marker_files":    markers,
			"package_manager": pm,
			"summary":         summary,
		},
	}, nil
}
func (t *NodeWorkspaceDetectTool) IsAvailable(ctx context.Context) bool { return true }
func (t *NodeWorkspaceDetectTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *NodeWorkspaceDetectTool) Tags() []string {
	return []string{ports.TagReadOnly, "lang:node", "workspace-detect", "recovery"}
}

type NodeProjectMetadataTool struct {
	BasePath string
}

func (t *NodeProjectMetadataTool) Name() string { return "node_project_metadata" }
func (t *NodeProjectMetadataTool) Description() string {
	return "Reads Node project markers and returns structured package metadata."
}
func (t *NodeProjectMetadataTool) Category() string { return "node" }
func (t *NodeProjectMetadataTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{{Name: "path", Type: "string", Required: false, Default: "."}}
}
func (t *NodeProjectMetadataTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
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
	projectRoot, manifestPath, markers := detectNodeProject(resolved, t.BasePath)
	if projectRoot == "" {
		return &ports.ToolResult{Success: false, Error: "no Node project markers found"}, nil
	}
	meta, err := parseNodeProjectMetadata(projectRoot, manifestPath, markers)
	if err != nil {
		return nil, err
	}
	return &ports.ToolResult{Success: true, Data: meta}, nil
}
func (t *NodeProjectMetadataTool) IsAvailable(ctx context.Context) bool { return true }
func (t *NodeProjectMetadataTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *NodeProjectMetadataTool) Tags() []string {
	return []string{ports.TagReadOnly, "lang:node", "metadata", "recovery"}
}

type NodeNPMTestTool struct {
	BasePath string
	runner   ports.CommandRunner
}

func NewNodeNPMTestTool(basePath string) *NodeNPMTestTool {
	return &NodeNPMTestTool{BasePath: basePath}
}

func (t *NodeNPMTestTool) Name() string { return "node_npm_test" }
func (t *NodeNPMTestTool) Description() string {
	return "Runs npm test and returns structured JavaScript/Node test results."
}
func (t *NodeNPMTestTool) Category() string { return "node" }
func (t *NodeNPMTestTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *NodeNPMTestTool) SetCommandRunner(r ports.CommandRunner) { t.runner = r }
func (t *NodeNPMTestTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []string{"npm", "test"}
	if raw, ok := args["extra_args"]; ok && raw != nil {
		if extra, err := toStringSliceValue(raw); err == nil {
			for _, entry := range extra {
				commandArgs = append(commandArgs, "--", entry)
			}
		}
	}
	spec := subprocess.RunSpec{
		Command: commandArgs,
		Workdir: workingDir,
	}
	runResult, runErr := subprocess.Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, runErr
	}
	stdout := runResult.Stdout
	stderr := runResult.Stderr
	summary := summarizeNodeNPMTest(stdout, stderr, runResult.Success)
	return &ports.ToolResult{
		Success: runResult.Success,
		Error:   runResult.Error,
		Data: map[string]any{
			"summary":       summary.Summary,
			"passed":        summary.Passed,
			"failed":        summary.Failed,
			"first_failure": summary.FirstFailure,
			"runner":        summary.Runner,
			"stdout":        stdout,
			"stderr":        stderr,
		},
	}, nil
}
func (t *NodeNPMTestTool) IsAvailable(ctx context.Context) bool { return t.runner != nil }
func (t *NodeNPMTestTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "npm"}},
	}}
}
func (t *NodeNPMTestTool) Tags() []string {
	return []string{ports.TagExecute, "lang:node", "test", "verification", "diagnostics"}
}

type NodeSyntaxCheckTool struct {
	BasePath string
	runner   ports.CommandRunner
}

func NewNodeSyntaxCheckTool(basePath string) *NodeSyntaxCheckTool {
	return &NodeSyntaxCheckTool{BasePath: basePath}
}

func (t *NodeSyntaxCheckTool) Name() string { return "node_syntax_check" }
func (t *NodeSyntaxCheckTool) Description() string {
	return "Runs node --check on a JavaScript file and returns structured syntax-check results."
}
func (t *NodeSyntaxCheckTool) Category() string { return "node" }
func (t *NodeSyntaxCheckTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "path", Type: "string", Required: true},
	}
}
func (t *NodeSyntaxCheckTool) SetCommandRunner(r ports.CommandRunner) {
	t.runner = r
}
func (t *NodeSyntaxCheckTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	target := strings.TrimSpace(fmt.Sprint(args["path"]))
	if target == "" || target == "<nil>" {
		return &ports.ToolResult{Success: false, Error: "path is required"}, nil
	}
	spec := subprocess.RunSpec{
		Command: []string{"node", "--check", target},
		Workdir: workingDir,
	}
	runResult, runErr := subprocess.Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, runErr
	}
	stdout := runResult.Stdout
	stderr := runResult.Stderr
	summary := summarizeNodeSyntaxCheck(stdout, stderr, runResult.Success)
	return &ports.ToolResult{
		Success: runResult.Success,
		Error:   runResult.Error,
		Data: map[string]any{
			"summary":       summary.Summary,
			"error_count":   summary.ErrorCount,
			"first_message": summary.FirstMessage,
			"stdout":        stdout,
			"stderr":        stderr,
		},
	}, nil
}
func (t *NodeSyntaxCheckTool) IsAvailable(ctx context.Context) bool { return t.runner != nil }
func (t *NodeSyntaxCheckTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "node"}},
	}}
}
func (t *NodeSyntaxCheckTool) Tags() []string {
	return []string{ports.TagExecute, "lang:node", "syntax-check", "verification", "diagnostics"}
}

type nodeTestSummary struct {
	Summary      string
	Passed       int
	Failed       int
	FirstFailure string
	Runner       string
}

type nodeCheckSummary struct {
	Summary      string
	ErrorCount   int
	FirstMessage string
}

var (
	nodeVitestPattern    = regexp.MustCompile(`(?m)\bTests?\s+(\d+)\s+failed(?:\s*\|\s*(\d+)\s+passed)?`)
	nodeJestPattern      = regexp.MustCompile(`(?m)Tests:\s+(\d+)\s+failed,\s+(\d+)\s+passed`)
	nodeMochaPattern     = regexp.MustCompile(`(?m)\b(\d+)\s+passing\b`)
	nodeMochaFailPattern = regexp.MustCompile(`(?m)\b(\d+)\s+failing\b`)
	nodeFailedSpec       = regexp.MustCompile(`(?m)^\s*[xX]?\s*(?:FAIL|FAILED)\s+(.+)$`)
	nodeSyntaxPattern    = regexp.MustCompile(`(?m)^(SyntaxError:.*|.*Unexpected token.*|.*missing.*)$`)
)

func summarizeNodeNPMTest(stdout, stderr string, success bool) nodeTestSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := nodeTestSummary{Summary: "npm test completed", Runner: inferNodeTestRunner(combined)}
	switch summary.Runner {
	case "vitest":
		if match := nodeVitestPattern.FindStringSubmatch(combined); len(match) > 2 {
			summary.Failed = atoiSafe(match[1])
			summary.Passed = atoiSafe(match[2])
		}
	case "jest":
		if match := nodeJestPattern.FindStringSubmatch(combined); len(match) > 2 {
			summary.Failed = atoiSafe(match[1])
			summary.Passed = atoiSafe(match[2])
		}
	default:
		if match := nodeMochaPattern.FindStringSubmatch(combined); len(match) > 1 {
			summary.Passed = atoiSafe(match[1])
		}
		if match := nodeMochaFailPattern.FindStringSubmatch(combined); len(match) > 1 {
			summary.Failed = atoiSafe(match[1])
		}
	}
	if match := nodeFailedSpec.FindStringSubmatch(combined); len(match) > 1 {
		summary.FirstFailure = strings.TrimSpace(match[1])
	}
	if summary.FirstFailure == "" {
		line := lang.FirstNonEmptyLine(stderr)
		if line == "" {
			line = lang.FirstNonEmptyLine(stdout)
		}
		summary.FirstFailure = line
	}
	if success {
		summary.Summary = fmt.Sprintf("npm test passed: %d passed, %d failed", summary.Passed, summary.Failed)
		return summary
	}
	if summary.FirstFailure != "" {
		summary.Summary = "npm test failed: " + summary.FirstFailure
	}
	return summary
}

func summarizeNodeSyntaxCheck(stdout, stderr string, success bool) nodeCheckSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := nodeCheckSummary{Summary: "node syntax check completed"}
	for _, match := range nodeSyntaxPattern.FindAllStringSubmatch(combined, -1) {
		if len(match) < 2 {
			continue
		}
		summary.ErrorCount++
		if summary.FirstMessage == "" {
			summary.FirstMessage = strings.TrimSpace(match[1])
		}
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = lang.FirstNonEmptyLine(stderr)
	}
	if summary.FirstMessage == "" {
		summary.FirstMessage = lang.FirstNonEmptyLine(stdout)
	}
	if success {
		summary.Summary = fmt.Sprintf("node syntax check passed: %d errors", summary.ErrorCount)
		return summary
	}
	if summary.FirstMessage != "" {
		summary.Summary = "node syntax check failed: " + summary.FirstMessage
	}
	return summary
}

func detectNodeProject(startDir, basePath string) (string, string, []string) {
	basePath = filepath.Clean(basePath)
	current := filepath.Clean(startDir)
	for {
		markers := findNodeMarkers(current)
		if len(markers) > 0 {
			manifest := ""
			for _, marker := range markers {
				if marker == "package.json" {
					manifest = filepath.Join(current, marker)
					break
				}
			}
			if manifest == "" {
				manifest = filepath.Join(current, markers[0])
			}
			return current, manifest, markers
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

func findNodeMarkers(dir string) []string {
	markers := make([]string, 0, len(nodeProjectMarkers))
	for _, marker := range nodeProjectMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			markers = append(markers, marker)
		}
	}
	sort.Strings(markers)
	return markers
}

func inferNodePackageManager(markers []string) string {
	switch {
	case containsString(markers, "pnpm-lock.yaml"):
		return "pnpm"
	case containsString(markers, "yarn.lock"):
		return "yarn"
	case containsString(markers, "package-lock.json"):
		return "npm"
	default:
		return ""
	}
}

func parseNodeProjectMetadata(projectRoot, manifestPath string, markers []string) (map[string]any, error) {
	type packageJSON struct {
		Name         string            `json:"name"`
		Type         string            `json:"type"`
		Private      bool              `json:"private"`
		Scripts      map[string]string `json:"scripts"`
		PackageMGR   string            `json:"packageManager"`
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
	}
	result := map[string]any{
		"project_root":    projectRoot,
		"manifest_path":   manifestPath,
		"marker_files":    markers,
		"package_manager": inferNodePackageManager(markers),
	}
	if manifestPath == "" || filepath.Base(manifestPath) != "package.json" {
		result["summary"] = fmt.Sprintf("Node project at %s", projectRoot)
		return result, nil
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var pkg packageJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, err
	}
	scriptNames := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		scriptNames = append(scriptNames, name)
	}
	sort.Strings(scriptNames)
	preferredTestTool := "node_npm_test"
	if _, ok := pkg.Scripts["test"]; !ok {
		preferredTestTool = ""
	}
	result["project_name"] = pkg.Name
	result["package_type"] = pkg.Type
	result["private"] = pkg.Private
	result["package_manager"] = cmp.Or(pkg.PackageMGR, inferNodePackageManager(markers))
	result["scripts"] = pkg.Scripts
	result["script_names"] = scriptNames
	result["has_test_script"] = hasScript(pkg.Scripts, "test")
	result["has_build_script"] = hasScript(pkg.Scripts, "build")
	result["has_lint_script"] = hasScript(pkg.Scripts, "lint")
	result["has_typecheck_script"] = hasTypecheck(pkg.Scripts, markers)
	result["preferred_test_tool"] = preferredTestTool
	result["is_typescript"] = containsString(markers, "tsmanifest.json") || containsString(markers, "jsmanifest.json")
	result["dependency_count"] = len(pkg.Dependencies) + len(pkg.DevDeps)
	result["summary"] = fmt.Sprintf("Node project at %s name=%s scripts=%d", projectRoot, pkg.Name, len(scriptNames))
	return result, nil
}

func hasScript(scripts map[string]string, name string) bool {
	if len(scripts) == 0 {
		return false
	}
	_, ok := scripts[name]
	return ok
}

func hasTypecheck(scripts map[string]string, markers []string) bool {
	if hasScript(scripts, "typecheck") {
		return true
	}
	return containsString(markers, "tsmanifest.json")
}

func inferNodeTestRunner(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "vitest"):
		return "vitest"
	case strings.Contains(lower, "jest") || strings.Contains(lower, "test suites:") || strings.Contains(lower, "tests:"):
		return "jest"
	case strings.Contains(lower, "mocha"):
		return "mocha"
	default:
		return "unknown"
	}
}
