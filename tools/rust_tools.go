package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	frameworktools "github.com/lexcodex/relurpify/framework/tools"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

type RustWorkspaceDetectTool struct {
	BasePath string
	manager  *runtime.PermissionManager
	agentID  string
}

func (t *RustWorkspaceDetectTool) Name() string { return "rust_workspace_detect" }
func (t *RustWorkspaceDetectTool) Description() string {
	return "Detects the nearest Rust crate/workspace manifest for a file or directory."
}
func (t *RustWorkspaceDetectTool) Category() string { return "rust" }
func (t *RustWorkspaceDetectTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: false, Default: "."},
	}
}
func (t *RustWorkspaceDetectTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}
func (t *RustWorkspaceDetectTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
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
	manifestPath, workspaceManifest := detectRustManifests(searchDir, t.BasePath)
	if manifestPath == "" {
		return &core.ToolResult{Success: false, Error: "no Cargo.toml found"}, nil
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":               resolved,
			"manifest_path":      manifestPath,
			"crate_root":         filepath.Dir(manifestPath),
			"workspace_manifest": workspaceManifest,
		},
	}, nil
}
func (t *RustWorkspaceDetectTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t *RustWorkspaceDetectTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.BasePath, core.FileSystemRead)}
}
func (t *RustWorkspaceDetectTool) Tags() []string {
	return []string{core.TagReadOnly, "lang:rust", "workspace-detect", "recovery"}
}

type RustCargoTestTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewRustCargoTestTool(basePath string) *RustCargoTestTool {
	return &RustCargoTestTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "rust_cargo_test",
			Description: "Runs cargo test and returns structured Rust test results.",
			Command:     "cargo",
			Category:    "rust",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *RustCargoTestTool) Name() string { return "rust_cargo_test" }
func (t *RustCargoTestTool) Description() string {
	return "Runs cargo test and returns structured Rust test results."
}
func (t *RustCargoTestTool) Category() string { return "rust" }
func (t *RustCargoTestTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "test_name", Type: "string", Required: false},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *RustCargoTestTool) SetCommandRunner(r runtime.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *RustCargoTestTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *RustCargoTestTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *RustCargoTestTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"test"}
	if raw, ok := args["test_name"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
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
	summary := summarizeRustCargoTest(stdout, stderr, result.Success)
	data := map[string]interface{}{
		"summary":       summary.Summary,
		"passed":        summary.Passed,
		"failed":        summary.Failed,
		"failed_tests":  summary.FailedTests,
		"first_failure": summary.FirstFailure,
		"stdout":        stdout,
		"stderr":        stderr,
	}
	return &core.ToolResult{
		Success:  result.Success,
		Error:    result.Error,
		Data:     data,
		Metadata: result.Metadata,
	}, nil
}
func (t *RustCargoTestTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *RustCargoTestTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *RustCargoTestTool) Tags() []string {
	return []string{core.TagExecute, "lang:rust", "test", "verification", "diagnostics"}
}

type RustCargoCheckTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewRustCargoCheckTool(basePath string) *RustCargoCheckTool {
	return &RustCargoCheckTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "rust_cargo_check",
			Description: "Runs cargo check and returns structured Rust compile results.",
			Command:     "cargo",
			Category:    "rust",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *RustCargoCheckTool) Name() string { return "rust_cargo_check" }
func (t *RustCargoCheckTool) Description() string {
	return "Runs cargo check and returns structured Rust compile results."
}
func (t *RustCargoCheckTool) Category() string { return "rust" }
func (t *RustCargoCheckTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
		{Name: "extra_args", Type: "array", Required: false},
	}
}
func (t *RustCargoCheckTool) SetCommandRunner(r runtime.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *RustCargoCheckTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *RustCargoCheckTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *RustCargoCheckTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	commandArgs := []interface{}{"check"}
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
	summary := summarizeRustCargoCheck(stdout, stderr, result.Success)
	return &core.ToolResult{
		Success: result.Success,
		Error:   result.Error,
		Data: map[string]interface{}{
			"summary":       summary.Summary,
			"error_count":   summary.ErrorCount,
			"warning_count": summary.WarningCount,
			"first_message": summary.FirstMessage,
			"stdout":        stdout,
			"stderr":        stderr,
		},
		Metadata: result.Metadata,
	}, nil
}
func (t *RustCargoCheckTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *RustCargoCheckTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *RustCargoCheckTool) Tags() []string {
	return []string{core.TagExecute, "lang:rust", "build", "verification", "diagnostics"}
}

type RustCargoMetadataTool struct {
	BasePath string
	inner    *clinix.CommandTool
}

func NewRustCargoMetadataTool(basePath string) *RustCargoMetadataTool {
	return &RustCargoMetadataTool{
		BasePath: basePath,
		inner: clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
			Name:        "rust_cargo_metadata",
			Description: "Runs cargo metadata and returns structured Rust workspace data.",
			Command:     "cargo",
			Category:    "rust",
			Tags:        []string{core.TagExecute},
		}),
	}
}

func (t *RustCargoMetadataTool) Name() string { return "rust_cargo_metadata" }
func (t *RustCargoMetadataTool) Description() string {
	return "Runs cargo metadata and returns structured Rust workspace data."
}
func (t *RustCargoMetadataTool) Category() string { return "rust" }
func (t *RustCargoMetadataTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "working_directory", Type: "string", Required: false, Default: "."},
	}
}
func (t *RustCargoMetadataTool) SetCommandRunner(r runtime.CommandRunner) {
	t.inner.SetCommandRunner(r)
}
func (t *RustCargoMetadataTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.inner.SetPermissionManager(manager, agentID)
}
func (t *RustCargoMetadataTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.inner.SetAgentSpec(spec, agentID)
}
func (t *RustCargoMetadataTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	workingDir := "."
	if raw, ok := args["working_directory"]; ok && raw != nil {
		workingDir = fmt.Sprint(raw)
	}
	result, err := t.inner.Execute(ctx, state, map[string]interface{}{
		"args":              []interface{}{"metadata", "--format-version", "1", "--no-deps"},
		"working_directory": workingDir,
	})
	if err != nil || result == nil {
		return result, err
	}
	stdout := fmt.Sprint(result.Data["stdout"])
	stderr := fmt.Sprint(result.Data["stderr"])
	summary, parsed := parseRustCargoMetadata(stdout)
	data := map[string]interface{}{
		"summary": summary,
		"stdout":  stdout,
		"stderr":  stderr,
	}
	for key, value := range parsed {
		data[key] = value
	}
	return &core.ToolResult{
		Success:  result.Success,
		Error:    result.Error,
		Data:     data,
		Metadata: result.Metadata,
	}, nil
}
func (t *RustCargoMetadataTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.inner.IsAvailable(ctx, state)
}
func (t *RustCargoMetadataTool) Permissions() core.ToolPermissions { return t.inner.Permissions() }
func (t *RustCargoMetadataTool) Tags() []string {
	return []string{core.TagExecute, "lang:rust", "metadata", "recovery"}
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
	return frameworktools.NormalizeStringSlice(value)
}
