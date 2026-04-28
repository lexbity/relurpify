package fs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export FileScope types for local usage
type (
	FileScopePolicy = contracts.FileScopePolicy
	FileScopeError  = contracts.FileScopeError
)

var (
	ErrFileScopeOutsideWorkspace = contracts.ErrFileScopeOutsideWorkspace
	ErrFileScopeProtectedPath    = contracts.ErrFileScopeProtectedPath
)

func NewFileScopePolicy(workspace string, protectedPaths []string) *FileScopePolicy {
	return contracts.NewFileScopePolicy(workspace, protectedPaths)
}

// FilePermissionChecker is re-exported from contracts
type FilePermissionChecker = contracts.FilePermissionChecker

func shouldSkipGeneratedDir(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	switch name {
	case ".git", "target", "node_modules", "dist", "build":
		return true
	default:
		return false
	}
}

// ReadFileTool reads files from disk.
type ReadFileTool struct {
	BasePath string
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *ReadFileTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *ReadFileTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *ReadFileTool) Name() string        { return "file_read" }
func (t *ReadFileTool) Description() string { return "Reads a UTF-8 file from disk." }
func (t *ReadFileTool) Category() string    { return "file" }
func (t *ReadFileTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	path := t.preparePath(fmt.Sprint(args["path"]))

	if err := t.enforceSandboxScope(contracts.FileSystemRead, path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	if info.IsDir() {
		return &contracts.ToolResult{Success: false, Error: fmt.Sprintf("%s is a directory; use file_list to explore it", path)}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	if !isText(data) {
		return &contracts.ToolResult{Success: false, Error: "binary file detected; cannot read binary files"}, nil
	}
	info, err = os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"content": string(data),
			"size":    info.Size(),
			"mode":    info.Mode().String(),
		},
	}, nil
}
func (t *ReadFileTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *ReadFileTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemRead)}
}
func (t *ReadFileTool) Tags() []string {
	return []string{contracts.TagReadOnly, "file", "inspect", "recovery"}
}

// WriteFileTool writes content to disk.
type WriteFileTool struct {
	BasePath string
	Backup   bool
	spec     *contracts.AgentRuntimeSpec
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *WriteFileTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *WriteFileTool) SetAgentSpec(spec *contracts.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *WriteFileTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *WriteFileTool) Name() string        { return "file_write" }
func (t *WriteFileTool) Description() string { return "Writes content to a file with backup." }
func (t *WriteFileTool) Category() string    { return "file" }
func (t *WriteFileTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "path", Type: "string", Required: true},
		{Name: "content", Type: "string", Required: true},
	}
}
func (t *WriteFileTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	path := t.preparePath(fmt.Sprint(args["path"]))

	if err := t.enforceSandboxScope(contracts.FileSystemWrite, path); err != nil {
		return nil, err
	}
	if err := t.enforceFileMatrix(ctx, "write", path); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	content := []byte(fmt.Sprint(args["content"]))
	if t.Backup {
		if _, err := os.Stat(path); err == nil {
			backup := path + ".bak"
			// Check sandbox scope for backup path
			if err := t.enforceSandboxScope(contracts.FileSystemWrite, backup); err != nil {
				return nil, fmt.Errorf("backup blocked: %w", err)
			}
			// Apply file matrix rules based on the original path (not the ".bak" suffix).
			if err := t.enforceFileMatrix(ctx, "write", path); err != nil {
				return nil, fmt.Errorf("backup blocked: %w", err)
			}
			if err := copyFile(path, backup); err != nil {
				return nil, err
			}
		}
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"path": path}}, nil
}
func (t *WriteFileTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *WriteFileTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemWrite)}
}
func (t *WriteFileTool) Tags() []string { return []string{contracts.TagDestructive, "file", "edit"} }

// ListFilesTool lists files filtered by pattern.
type ListFilesTool struct {
	BasePath string
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *ListFilesTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *ListFilesTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *ListFilesTool) Name() string        { return "file_list" }
func (t *ListFilesTool) Description() string { return "Lists files recursively using glob filtering." }
func (t *ListFilesTool) Category() string    { return "file" }
func (t *ListFilesTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "directory", Type: "string", Required: false, Default: "."},
		{Name: "pattern", Type: "string", Required: false, Default: "*"},
	}
}
func (t *ListFilesTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	dirVal, ok := args["directory"]
	if !ok || dirVal == nil {
		dirVal = "."
	}
	dirText := strings.TrimSpace(fmt.Sprint(dirVal))
	if dirText == "" || dirText == "<nil>" {
		dirText = "."
	}
	dir := t.preparePath(dirText)
	if err := t.enforceSandboxScope(contracts.FileSystemList, dir); err != nil {
		return nil, err
	}

	pattern := fmt.Sprint(args["pattern"])
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipGeneratedDir(d.Name()) {
				return fs.SkipDir
			}
			if err := t.enforceSandboxScope(contracts.FileSystemList, path); err != nil {
				if sandboxProtectedPath(err) {
					return fs.SkipDir
				}
				return err
			}
			return nil
		}

		if err := t.enforceSandboxScope(contracts.FileSystemRead, path); err != nil {
			if sandboxProtectedPath(err) {
				return nil
			}
			return err
		}

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = filepath.Base(path)
		}
		relPath = filepath.ToSlash(relPath)
		match := contracts.MatchGlob(pattern, relPath)
		if !match {
			match = contracts.MatchGlob(pattern, filepath.Base(path))
		}
		if match {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"files": files}}, nil
}
func (t *ListFilesTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *ListFilesTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemList)}
}
func (t *ListFilesTool) Tags() []string { return []string{contracts.TagReadOnly, "file", "discover"} }

// SearchInFilesTool greps for a pattern.
type SearchInFilesTool struct {
	BasePath string
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *SearchInFilesTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *SearchInFilesTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *SearchInFilesTool) Name() string        { return "file_search" }
func (t *SearchInFilesTool) Description() string { return "Searches text inside files." }
func (t *SearchInFilesTool) Category() string    { return "file" }
func (t *SearchInFilesTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "directory", Type: "string", Required: false, Default: "."},
		{Name: "pattern", Type: "string", Required: true},
		{Name: "case_sensitive", Type: "bool", Required: false, Default: false},
	}
}
func (t *SearchInFilesTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	dirVal, ok := args["directory"]
	if !ok || dirVal == nil {
		dirVal = "."
	}
	dirText := strings.TrimSpace(fmt.Sprint(dirVal))
	if dirText == "" || dirText == "<nil>" {
		dirText = "."
	}
	dir := t.preparePath(dirText)
	if err := t.enforceSandboxScope(contracts.FileSystemRead, dir); err != nil {
		return nil, err
	}

	pattern := fmt.Sprint(args["pattern"])
	caseSensitive := toBool(args["case_sensitive"])
	if !caseSensitive {
		pattern = strings.ToLower(pattern)
	}
	type match struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}
	var matches []match
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipGeneratedDir(d.Name()) {
				return fs.SkipDir
			}
			if err := t.enforceSandboxScope(contracts.FileSystemList, path); err != nil {
				if sandboxProtectedPath(err) {
					return fs.SkipDir
				}
				return err
			}
			return nil
		}

		if err := t.enforceSandboxScope(contracts.FileSystemRead, path); err != nil {
			if sandboxProtectedPath(err) {
				return nil
			}
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable files
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, scanChunkSize), scanChunkSize)
		scanner.Split(scanLinesOrChunks(scanChunkSize))
		line := 1
		for scanner.Scan() {
			text := scanner.Text()
			compare := text
			if !caseSensitive {
				compare = strings.ToLower(text)
			}
			if strings.Contains(compare, pattern) {
				matches = append(matches, match{
					File:    path,
					Line:    line,
					Content: text,
				})
			}
			line++
		}
		// Skip files with I/O errors (e.g. permission denied mid-read).
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"matches": matches}}, nil
}
func (t *SearchInFilesTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *SearchInFilesTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemRead, contracts.FileSystemList)}
}
func (t *SearchInFilesTool) Tags() []string {
	return []string{contracts.TagReadOnly, "search", "recovery"}
}

// CreateFileTool creates a file from a template string.
type CreateFileTool struct {
	BasePath string
	spec     *contracts.AgentRuntimeSpec
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *CreateFileTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *CreateFileTool) SetAgentSpec(spec *contracts.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *CreateFileTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *CreateFileTool) Name() string        { return "file_create" }
func (t *CreateFileTool) Description() string { return "Creates a new file if it does not exist." }
func (t *CreateFileTool) Category() string    { return "file" }
func (t *CreateFileTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "path", Type: "string", Required: true},
		{Name: "content", Type: "string", Required: false},
	}
}
func (t *CreateFileTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	path := t.preparePath(fmt.Sprint(args["path"]))

	if err := t.enforceSandboxScope(contracts.FileSystemWrite, path); err != nil {
		return nil, err
	}
	if err := t.enforceFileMatrix(ctx, "write", path); err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("file %s already exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(fmt.Sprint(args["content"])), 0o644); err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"path": path}}, nil
}
func (t *CreateFileTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *CreateFileTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemWrite)}
}
func (t *CreateFileTool) Tags() []string { return []string{contracts.TagDestructive, "file", "edit"} }

// DeleteFileTool moves a file to .trash folder instead of deleting permanently.
type DeleteFileTool struct {
	BasePath string
	TrashDir string
	spec     *contracts.AgentRuntimeSpec
	manager  FilePermissionChecker
	agentID  string
	scope    *FileScopePolicy
}

func (t *DeleteFileTool) SetPermissionManager(manager FilePermissionChecker, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *DeleteFileTool) SetAgentSpec(spec *contracts.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *DeleteFileTool) SetSandboxScope(scope *FileScopePolicy) {
	t.scope = scope
}

func (t *DeleteFileTool) Name() string        { return "file_delete" }
func (t *DeleteFileTool) Description() string { return "Deletes a file after confirmation." }
func (t *DeleteFileTool) Category() string    { return "file" }
func (t *DeleteFileTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t *DeleteFileTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	path := t.preparePath(fmt.Sprint(args["path"]))

	if err := t.enforceSandboxScope(contracts.FileSystemDelete, path); err != nil {
		return nil, err
	}
	if err := t.enforceFileMatrix(ctx, "write", path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	trash := t.TrashDir
	if trash == "" {
		trash = filepath.Join(t.BasePath, ".trash")
	}
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(trash, info.Name())
	if err := os.Rename(path, dest); err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"path": dest}}, nil
}
func (t *DeleteFileTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *DeleteFileTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: contracts.NewFileSystemPermissionSet(t.BasePath, contracts.FileSystemWrite)}
}
func (t *DeleteFileTool) Tags() []string { return []string{contracts.TagDestructive, "file", "edit"} }

func (t *ReadFileTool) preparePath(path string) string  { return preparePath(t.BasePath, path) }
func (t *WriteFileTool) preparePath(path string) string { return preparePath(t.BasePath, path) }
func (t *ListFilesTool) preparePath(path string) string { return preparePath(t.BasePath, path) }
func (t *SearchInFilesTool) preparePath(path string) string {
	return preparePath(t.BasePath, path)
}
func (t *CreateFileTool) preparePath(path string) string { return preparePath(t.BasePath, path) }
func (t *DeleteFileTool) preparePath(path string) string { return preparePath(t.BasePath, path) }

func (t *WriteFileTool) enforceFileMatrix(ctx context.Context, action string, absPath string) error {
	if t == nil || t.spec == nil {
		return nil
	}
	return enforceFileMatrix(ctx, t.manager, t.agentID, t.BasePath, action, absPath, t.spec.Files)
}

func (t *CreateFileTool) enforceFileMatrix(ctx context.Context, action string, absPath string) error {
	if t == nil || t.spec == nil {
		return nil
	}
	return enforceFileMatrix(ctx, t.manager, t.agentID, t.BasePath, action, absPath, t.spec.Files)
}

func (t *DeleteFileTool) enforceFileMatrix(ctx context.Context, action string, absPath string) error {
	if t == nil || t.spec == nil {
		return nil
	}
	return enforceFileMatrix(ctx, t.manager, t.agentID, t.BasePath, action, absPath, t.spec.Files)
}

func (t *ReadFileTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func (t *WriteFileTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func (t *ListFilesTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func (t *SearchInFilesTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func (t *CreateFileTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func (t *DeleteFileTool) enforceSandboxScope(action contracts.FileSystemAction, path string) error {
	if t == nil || t.scope == nil {
		return nil
	}
	return t.scope.Check(action, path)
}

func sandboxProtectedPath(err error) bool {
	if err == nil {
		return false
	}
	var scopeErr *FileScopeError
	if errors.As(err, &scopeErr) {
		return scopeErr.Reason == ErrFileScopeProtectedPath.Error()
	}
	return false
}

func preparePath(base, path string) string {
	if base == "" {
		return filepath.Clean(path)
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func toBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := out.ReadFrom(in); err != nil {
		return err
	}
	return nil
}

func enforceFileMatrix(ctx context.Context, checker FilePermissionChecker, agentID, basePath, action, absPath string, matrix contracts.AgentFileMatrix) error {
	rel := absPath
	if basePath != "" {
		if r, err := filepath.Rel(basePath, absPath); err == nil {
			rel = r
		}
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(rel, "./") {
		rel = strings.TrimPrefix(rel, "./")
	}
	perm := matrix.Write
	if action == "edit" {
		perm = matrix.Edit
	}
	if perm.DocumentationOnly && !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return fmt.Errorf("file %s blocked: documentation_only enabled", rel)
	}
	decision, _ := DecideByPatterns(rel, perm.AllowPatterns, perm.DenyPatterns, perm.Default)
	if perm.RequireApproval {
		decision = contracts.AgentPermissionAsk
	}
	switch decision {
	case contracts.AgentPermissionAllow:
		return nil
	case contracts.AgentPermissionDeny:
		return fmt.Errorf("file %s blocked: denied by file_permissions", rel)
	case contracts.AgentPermissionAsk:
		if checker == nil {
			return fmt.Errorf("file %s blocked: approval required but permission manager missing", rel)
		}
		return checker.CheckFilePermission(ctx, agentID, basePath, action, absPath, matrix)
	default:
		return nil
	}
}

// DecideByPatterns returns allow/deny/ask based on deny-first then allow list.
// This is a local copy to avoid importing framework/authorization.
func DecideByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision contracts.AgentPermissionLevel) (contracts.AgentPermissionLevel, string) {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matched, _ := filepath.Match(pattern, target); matched {
			return contracts.AgentPermissionDeny, pattern
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matched, _ := filepath.Match(pattern, target); matched {
			return contracts.AgentPermissionAllow, pattern
		}
	}
	return defaultDecision, ""
}

// scanChunkSize is the maximum bytes returned per scanner token. Lines longer
// than this are split into consecutive chunks rather than causing a buffer
// overflow error.
const scanChunkSize = 64 * 1024

// scanLinesOrChunks returns a bufio.SplitFunc that behaves like
// bufio.ScanLines but force-splits any line that exceeds maxChunk bytes.
// This prevents bufio.Scanner from erroring on minified or generated files.
func scanLinesOrChunks(maxChunk int) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		limit := len(data)
		if limit > maxChunk {
			limit = maxChunk
		}
		if i := bytes.IndexByte(data[:limit], '\n'); i >= 0 {
			line := data[:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return i + 1, line, nil
		}
		if len(data) >= maxChunk {
			return maxChunk, data[:maxChunk], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

// FileOperations registers default file tools into a registry.
func FileOperations(basePath string) []contracts.Tool {
	return []contracts.Tool{
		&ReadFileTool{BasePath: basePath},
		&WriteFileTool{BasePath: basePath, Backup: true},
		&ListFilesTool{BasePath: basePath},
		&SearchInFilesTool{BasePath: basePath},
		&CreateFileTool{BasePath: basePath},
		&DeleteFileTool{BasePath: basePath},
	}
}
