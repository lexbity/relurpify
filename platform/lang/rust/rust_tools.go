package rust

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

type RustWorkspaceDetectTool struct {
	BasePath string
}

func (t *RustWorkspaceDetectTool) Name() string { return "rust_workspace_detect" }
func (t *RustWorkspaceDetectTool) Description() string {
	return "Detects the nearest Rust crate/workspace manifest for a file or directory."
}
func (t *RustWorkspaceDetectTool) Category() string { return "rust" }
func (t *RustWorkspaceDetectTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "path", Type: "string", Required: false, Default: "."},
	}
}
func (t *RustWorkspaceDetectTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
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
		return &ports.ToolResult{Success: false, Error: err.Error()}, nil
	}
	searchDir := resolved
	if !info.IsDir() {
		searchDir = filepath.Dir(resolved)
	}
	manifestPath, workspaceManifest := detectRustManifests(searchDir, t.BasePath)
	if manifestPath == "" {
		return &ports.ToolResult{Success: false, Error: "no Cargo.toml found"}, nil
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":               resolved,
			"manifest_path":      manifestPath,
			"crate_root":         filepath.Dir(manifestPath),
			"workspace_manifest": workspaceManifest,
		},
	}, nil
}
func (t *RustWorkspaceDetectTool) IsAvailable(ctx context.Context) bool { return true }
func (t *RustWorkspaceDetectTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *RustWorkspaceDetectTool) Tags() []string {
	return []string{ports.TagReadOnly, "lang:rust", "workspace-detect", "recovery"}
}

type RustCargoTestTool struct {
	BasePath string
	runner   ports.CommandRunner
}

func NewRustCargoTestTool(basePath string) *RustCargoTestTool {
	return &RustCargoTestTool{BasePath: basePath}
}

func (t *RustCargoTestTool) Name() string { return "rust_cargo_test" }
func (t *RustCargoTestTool) Description() string {
	return "Runs cargo test and returns structured Rust test results."
}
func (t *RustCargoTestTool) Category() string { return "rust" }
func (t *RustCargoTestTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "test_name", Type: "string", Required: false},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *RustCargoTestTool) SetCommandRunner(r ports.CommandRunner) {
	t.runner = r
}
func (t *RustCargoTestTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []string{"cargo", "test"}
	if raw, ok := args["test_name"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		commandArgs = append(commandArgs, fmt.Sprint(raw))
	}
	if raw, ok := args["extra_args"]; ok && raw != nil {
		if extra, err := toStringSliceValue(raw); err == nil {
			commandArgs = append(commandArgs, extra...)
		}
	}
	spec := subprocess.RunSpec{
		Command:             commandArgs,
		Workdir:             workingDir,
		ApplyCargoIsolation: true,
		SourcePath:          t.BasePath,
	}
	runResult, runErr := subprocess.Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, runErr
	}
	stdout := runResult.Stdout
	stderr := runResult.Stderr
	summary := summarizeRustCargoTest(stdout, stderr, runResult.Success)
	data := map[string]interface{}{
		"summary":       summary.Summary,
		"passed":        summary.Passed,
		"failed":        summary.Failed,
		"failed_tests":  summary.FailedTests,
		"first_failure": summary.FirstFailure,
		"stdout":        stdout,
		"stderr":        stderr,
	}
	return &ports.ToolResult{
		Success: runResult.Success,
		Error:   runResult.Error,
		Data:    data,
	}, nil
}
func (t *RustCargoTestTool) IsAvailable(ctx context.Context) bool { return t.runner != nil }
func (t *RustCargoTestTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "cargo"}},
	}}
}
func (t *RustCargoTestTool) Tags() []string {
	return []string{ports.TagExecute, "lang:rust", "test", "verification", "diagnostics"}
}

type RustCargoCheckTool struct {
	BasePath string
	runner   ports.CommandRunner
}

func NewRustCargoCheckTool(basePath string) *RustCargoCheckTool {
	return &RustCargoCheckTool{BasePath: basePath}
}

func (t *RustCargoCheckTool) Name() string { return "rust_cargo_check" }
func (t *RustCargoCheckTool) Description() string {
	return "Runs cargo check and returns structured Rust compile results."
}
func (t *RustCargoCheckTool) Category() string { return "rust" }
func (t *RustCargoCheckTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *RustCargoCheckTool) SetCommandRunner(r ports.CommandRunner) {
	t.runner = r
}
func (t *RustCargoCheckTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []string{"cargo", "check"}
	if raw, ok := args["extra_args"]; ok && raw != nil {
		if extra, err := toStringSliceValue(raw); err == nil {
			commandArgs = append(commandArgs, extra...)
		}
	}
	spec := subprocess.RunSpec{
		Command:             commandArgs,
		Workdir:             workingDir,
		ApplyCargoIsolation: true,
		SourcePath:          t.BasePath,
	}
	runResult, runErr := subprocess.Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, runErr
	}
	stdout := runResult.Stdout
	stderr := runResult.Stderr
	summary := summarizeRustCargoCheck(stdout, stderr, runResult.Success)
	return &ports.ToolResult{
		Success: runResult.Success,
		Error:   runResult.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"error_count":   summary.ErrorCount,
			"warning_count": summary.WarningCount,
			"first_message": summary.FirstMessage,
			"stdout":        stdout,
			"stderr":        stderr,
		},
	}, nil
}
func (t *RustCargoCheckTool) IsAvailable(ctx context.Context) bool { return t.runner != nil }
func (t *RustCargoCheckTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "cargo"}},
	}}
}
func (t *RustCargoCheckTool) Tags() []string {
	return []string{ports.TagExecute, "lang:rust", "build", "verification", "diagnostics"}
}

type RustCargoMetadataTool struct {
	BasePath string
	runner   ports.CommandRunner
}

func NewRustCargoMetadataTool(basePath string) *RustCargoMetadataTool {
	return &RustCargoMetadataTool{BasePath: basePath}
}

func (t *RustCargoMetadataTool) Name() string { return "rust_cargo_metadata" }
func (t *RustCargoMetadataTool) Description() string {
	return "Runs cargo metadata and returns structured Rust workspace data."
}
func (t *RustCargoMetadataTool) Category() string { return "rust" }
func (t *RustCargoMetadataTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
	}
}
func (t *RustCargoMetadataTool) SetCommandRunner(r ports.CommandRunner) {
	t.runner = r
}
func (t *RustCargoMetadataTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	spec := subprocess.RunSpec{
		Command:             []string{"cargo", "metadata", "--format-version", "1", "--no-deps"},
		Workdir:             workingDir,
		ApplyCargoIsolation: true,
		SourcePath:          t.BasePath,
	}
	runResult, runErr := subprocess.Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, runErr
	}
	stdout := runResult.Stdout
	stderr := runResult.Stderr
	summary, parsed := parseRustCargoMetadata(stdout)
	data := map[string]interface{}{
		"summary": summary,
		"stdout":  stdout,
		"stderr":  stderr,
	}
	for key, value := range parsed {
		data[key] = value
	}
	return &ports.ToolResult{
		Success: runResult.Success,
		Error:   runResult.Error,
		Data:    data,
	}, nil
}
func (t *RustCargoMetadataTool) IsAvailable(ctx context.Context) bool {
	return t.runner != nil
}
func (t *RustCargoMetadataTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{{Binary: "cargo"}},
	}}
}
func (t *RustCargoMetadataTool) Tags() []string {
	return []string{ports.TagExecute, "lang:rust", "metadata", "recovery"}
}

type rustCargoSummary struct {
	Summary      string
	Passed       int
	Failed       int
	FailedTests  []string
	FirstFailure string
}

type rustCargoCheckSummary struct {
	Summary      string
	ErrorCount   int
	WarningCount int
	FirstMessage string
}

var (
	rustFailedTestPattern = regexp.MustCompile(`(?m)^----\s+(.+?)\s+stdout\s+----$`)
	rustTestCountPattern  = regexp.MustCompile(`(?m)test result:\s+(ok|FAILED)\.\s+(\d+)\s+passed;\s+(\d+)\s+failed;`)
)

func summarizeRustCargoTest(stdout, stderr string, success bool) rustCargoSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := rustCargoSummary{
		Summary: "cargo test completed",
	}
	matches := rustTestCountPattern.FindAllStringSubmatch(combined, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		summary.Passed += atoiSafe(match[2])
		summary.Failed += atoiSafe(match[3])
	}
	for _, match := range rustFailedTestPattern.FindAllStringSubmatch(combined, -1) {
		if len(match) < 2 {
			continue
		}
		summary.FailedTests = append(summary.FailedTests, strings.TrimSpace(match[1]))
	}
	if len(summary.FailedTests) > 0 {
		summary.FirstFailure = summary.FailedTests[0]
	}
	if success {
		summary.Summary = fmt.Sprintf("cargo test passed: %d passed, %d failed", summary.Passed, summary.Failed)
		return summary
	}
	if summary.FirstFailure != "" {
		summary.Summary = fmt.Sprintf("cargo test failed: %s", summary.FirstFailure)
		return summary
	}
	line := firstNonEmptyLine(stderr)
	if line == "" {
		line = firstNonEmptyLine(stdout)
	}
	if line != "" {
		summary.Summary = "cargo test failed: " + line
	}
	return summary
}

func summarizeRustCargoCheck(stdout, stderr string, success bool) rustCargoCheckSummary {
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	summary := rustCargoCheckSummary{Summary: "cargo check completed"}
	for _, line := range strings.Split(combined, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		lower := strings.ToLower(text)
		if strings.HasPrefix(lower, "error") {
			summary.ErrorCount++
			if summary.FirstMessage == "" {
				summary.FirstMessage = text
			}
		}
		if strings.HasPrefix(lower, "warning") {
			summary.WarningCount++
			if summary.FirstMessage == "" {
				summary.FirstMessage = text
			}
		}
	}
	if success {
		summary.Summary = fmt.Sprintf("cargo check passed: %d errors, %d warnings", summary.ErrorCount, summary.WarningCount)
		return summary
	}
	if summary.FirstMessage != "" {
		summary.Summary = "cargo check failed: " + summary.FirstMessage
		return summary
	}
	line := firstNonEmptyLine(stderr)
	if line == "" {
		line = firstNonEmptyLine(stdout)
	}
	if line != "" {
		summary.Summary = "cargo check failed: " + line
	}
	return summary
}

func parseRustCargoMetadata(stdout string) (string, map[string]interface{}) {
	type cargoPackage struct {
		Name         string `json:"name"`
		ManifestPath string `json:"manifest_path"`
	}
	type cargoMetadata struct {
		WorkspaceRoot string         `json:"workspace_root"`
		Packages      []cargoPackage `json:"packages"`
	}
	var payload cargoMetadata
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return "cargo metadata completed", map[string]interface{}{}
	}
	packageNames := make([]string, 0, len(payload.Packages))
	manifestPaths := make([]string, 0, len(payload.Packages))
	for _, pkg := range payload.Packages {
		packageNames = append(packageNames, pkg.Name)
		manifestPaths = append(manifestPaths, pkg.ManifestPath)
	}
	summary := fmt.Sprintf("cargo metadata: %d packages", len(packageNames))
	if payload.WorkspaceRoot != "" {
		summary += " workspace=" + payload.WorkspaceRoot
	}
	return summary, map[string]interface{}{
		"workspace_root": payload.WorkspaceRoot,
		"package_names":  packageNames,
		"manifest_paths": manifestPaths,
		"package_count":  len(packageNames),
	}
}

func detectRustManifests(startDir, basePath string) (string, string) {
	basePath = filepath.Clean(basePath)
	current := filepath.Clean(startDir)
	nearest := ""
	workspace := ""
	for {
		manifestPath := filepath.Join(current, "Cargo.toml")
		if _, err := os.Stat(manifestPath); err == nil {
			if nearest == "" {
				nearest = manifestPath
			}
			workspace = manifestPath
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
	return nearest, workspace
}

func atoiSafe(value string) int {
	var total int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return total
		}
		total = total*10 + int(ch-'0')
	}
	return total
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func toStringSliceValue(value interface{}) ([]string, error) {
	return registry.NormalizeStringSlice(value)
}
