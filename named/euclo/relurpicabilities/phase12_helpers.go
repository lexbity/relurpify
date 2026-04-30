package relurpicabilities

import (
	"bytes"
	"context"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	frameworkast "codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

type frameworkPolicyContext struct {
	permissionManager *authorization.PermissionManager
	agentID           string
	agentSpec         *core.AgentRuntimeSpec
	sandboxScope      *sandbox.FileScopePolicy
}

func (c *frameworkPolicyContext) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	if c == nil {
		return
	}
	c.permissionManager = manager
	c.agentID = agentID
}

func (c *frameworkPolicyContext) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	if c == nil {
		return
	}
	c.agentSpec = spec
	c.agentID = agentID
}

func (c *frameworkPolicyContext) SetSandboxScope(scope *sandbox.FileScopePolicy) {
	if c == nil {
		return
	}
	c.sandboxScope = scope
}

func (c *frameworkPolicyContext) authorizeCommand(ctx context.Context, env agentenv.WorkspaceEnvironment, req sandbox.CommandRequest, source string) error {
	if c != nil && c.permissionManager != nil {
		return authorization.AuthorizeCommand(ctx, c.permissionManager, c.agentID, c.agentSpec, authorization.CommandAuthorizationRequest{
			Command: append([]string(nil), req.Args...),
			Env:     append([]string(nil), req.Env...),
			Source:  source,
		})
	}
	if env.CommandPolicy != nil {
		return env.CommandPolicy.AllowCommand(ctx, req)
	}
	return fmt.Errorf("command denied: no framework command policy configured")
}

func (c *frameworkPolicyContext) authorizeFileWrite(ctx context.Context, env agentenv.WorkspaceEnvironment, path string) error {
	if c != nil && c.permissionManager != nil {
		return c.permissionManager.CheckFileAccess(ctx, c.agentID, core.FileSystemWrite, path)
	}
	if scope := c.fileScopePolicy(env); scope != nil {
		return scope.Check(core.FileSystemWrite, path)
	}
	return fmt.Errorf("write denied: no file scope configured")
}

func workspaceRoot(env agentenv.WorkspaceEnvironment) string {
	if env.IndexManager != nil {
		if root := strings.TrimSpace(env.IndexManager.WorkspacePath()); root != "" {
			return root
		}
	}
	return "."
}

func (c *frameworkPolicyContext) fileScopePolicy(env agentenv.WorkspaceEnvironment) *sandbox.FileScopePolicy {
	if c != nil && c.sandboxScope != nil {
		return c.sandboxScope
	}
	return env.FileScope
}

func (c *frameworkPolicyContext) resolveWorkspacePath(env agentenv.WorkspaceEnvironment, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", fmt.Errorf("path is required")
	}
	root := workspaceRoot(env)
	resolved := candidate
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, resolved)
	}
	resolved = filepath.Clean(resolved)
	if scope := c.fileScopePolicy(env); scope != nil {
		if err := scope.Check(core.FileSystemRead, resolved); err != nil {
			return "", err
		}
	}
	return resolved, nil
}

func (c *frameworkPolicyContext) readWorkspaceFile(env agentenv.WorkspaceEnvironment, candidate string) ([]byte, string, error) {
	resolved, err := c.resolveWorkspacePath(env, candidate)
	if err != nil {
		return nil, "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, "", err
	}
	return content, resolved, nil
}

func (c *frameworkPolicyContext) writeWorkspaceFile(env agentenv.WorkspaceEnvironment, candidate string, content []byte, perm os.FileMode) (string, error) {
	resolved, err := c.resolveWorkspacePath(env, candidate)
	if err != nil {
		return "", err
	}
	if scope := c.fileScopePolicy(env); scope != nil {
		if err := scope.Check(core.FileSystemWrite, resolved); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(resolved, content, perm); err != nil {
		return "", err
	}
	return resolved, nil
}

func floatArg(args map[string]interface{}, key string, defaultValue float64) (float64, bool) {
	val, ok := args[key]
	if !ok || val == nil {
		return defaultValue, false
	}
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return parsed, true
		}
	}
	return defaultValue, false
}

func parseCoverageOutput(output string) (map[string]float64, []coveragePackageRecord) {
	coverage := make(map[string]float64)
	records := make([]coveragePackageRecord, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "ok" {
			continue
		}
		pkg := fields[1]
		idx := strings.Index(line, "coverage:")
		if idx < 0 {
			continue
		}
		segment := strings.TrimSpace(line[idx+len("coverage:"):])
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "of statements"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "%"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "of statements"))
		segment = strings.TrimSpace(strings.TrimSuffix(segment, "%"))
		percent, err := strconv.ParseFloat(segment, 64)
		if err != nil {
			continue
		}
		coverage[pkg] = percent
		records = append(records, coveragePackageRecord{Package: pkg, Coverage: percent})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Package < records[j].Package })
	return coverage, records
}

type apiSignatureRecord struct {
	File      string
	Symbol    string
	Signature string
}

type apiChangeRecord struct {
	File          string
	Symbol        string
	BaseSignature string
	HeadSignature string
	Change        string
}

type coveragePackageRecord struct {
	Package  string
	Coverage float64
}

func collectExportedAPISignatures(path string, src []byte) (map[string]apiSignatureRecord, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]apiSignatureRecord)
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() {
				continue
			}
			symbol := functionSymbolName(d)
			out[signatureKey(path, symbol)] = apiSignatureRecord{
				File:      path,
				Symbol:    symbol,
				Signature: renderFuncSignature(d),
			}
		case *goast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *goast.TypeSpec:
					if s.Name == nil || !s.Name.IsExported() {
						continue
					}
					out[signatureKey(path, s.Name.Name)] = apiSignatureRecord{
						File:      path,
						Symbol:    s.Name.Name,
						Signature: renderTypeSignature(s),
					}
				case *goast.ValueSpec:
					for _, name := range s.Names {
						if name == nil || !name.IsExported() {
							continue
						}
						out[signatureKey(path, name.Name)] = apiSignatureRecord{
							File:      path,
							Symbol:    name.Name,
							Signature: renderValueSignature(strings.ToLower(d.Tok.String()), name.Name, s),
						}
					}
				}
			}
		}
	}
	return out, nil
}

func renderFuncSignature(fn *goast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(renderFieldListType(fn.Recv))
		b.WriteString(").")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString("(")
	b.WriteString(renderFieldListType(fn.Type.Params))
	b.WriteString(")")
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteString(" ")
		b.WriteString(renderResultListType(fn.Type.Results))
	}
	return strings.TrimSpace(b.String())
}

func functionSymbolName(fn *goast.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := strings.TrimSpace(renderFieldListType(fn.Recv))
	recv = strings.TrimPrefix(recv, "*")
	return strings.TrimSpace(recv) + "." + fn.Name.Name
}

func renderTypeSignature(spec *goast.TypeSpec) string {
	if spec == nil || spec.Name == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("type ")
	b.WriteString(spec.Name.Name)
	if spec.Assign.IsValid() {
		b.WriteString(" = ")
	} else {
		b.WriteString(" ")
	}
	b.WriteString(renderExpr(spec.Type))
	return strings.TrimSpace(b.String())
}

func renderValueSignature(kind, name string, spec *goast.ValueSpec) string {
	var b strings.Builder
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(name)
	if spec.Type != nil {
		b.WriteString(" ")
		b.WriteString(renderExpr(spec.Type))
	}
	return strings.TrimSpace(b.String())
}

func renderFieldListType(fields *goast.FieldList) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if field == nil {
			continue
		}
		typeText := renderExpr(field.Type)
		if typeText == "" {
			continue
		}
		if len(field.Names) > 0 {
			for range field.Names {
				parts = append(parts, typeText)
			}
			continue
		}
		parts = append(parts, typeText)
	}
	return strings.Join(parts, ", ")
}

func renderResultListType(fields *goast.FieldList) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if field == nil {
			continue
		}
		typeText := renderExpr(field.Type)
		if typeText == "" {
			continue
		}
		if len(field.Names) > 0 {
			for range field.Names {
				parts = append(parts, typeText)
			}
			continue
		}
		parts = append(parts, typeText)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func renderExpr(expr goast.Expr) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func signatureKey(file, symbol string) string {
	return strings.TrimSpace(file) + "::" + strings.TrimSpace(symbol)
}

func compareAPISignatures(base, head map[string]apiSignatureRecord) ([]apiChangeRecord, []apiChangeRecord) {
	breaking := make([]apiChangeRecord, 0)
	compatible := make([]apiChangeRecord, 0)
	keys := make([]string, 0, len(base)+len(head))
	for key := range base {
		keys = append(keys, key)
	}
	for key := range head {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := map[string]struct{}{}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		baseRec, baseOK := base[key]
		headRec, headOK := head[key]
		switch {
		case baseOK && !headOK:
			breaking = append(breaking, apiChangeRecord{File: baseRec.File, Symbol: baseRec.Symbol, BaseSignature: baseRec.Signature, Change: "removed"})
		case !baseOK && headOK:
			compatible = append(compatible, apiChangeRecord{File: headRec.File, Symbol: headRec.Symbol, HeadSignature: headRec.Signature, Change: "added"})
		case baseOK && headOK && baseRec.Signature != headRec.Signature:
			breaking = append(breaking, apiChangeRecord{File: headRec.File, Symbol: headRec.Symbol, BaseSignature: baseRec.Signature, HeadSignature: headRec.Signature, Change: "modified"})
		}
	}
	sort.SliceStable(breaking, func(i, j int) bool {
		if breaking[i].File == breaking[j].File {
			return breaking[i].Symbol < breaking[j].Symbol
		}
		return breaking[i].File < breaking[j].File
	})
	sort.SliceStable(compatible, func(i, j int) bool {
		if compatible[i].File == compatible[j].File {
			return compatible[i].Symbol < compatible[j].Symbol
		}
		return compatible[i].File < compatible[j].File
	})
	return breaking, compatible
}

func changeRecordSlice(records []apiChangeRecord) []interface{} {
	out := make([]interface{}, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]interface{}{
			"file":           record.File,
			"symbol":         record.Symbol,
			"base_signature": record.BaseSignature,
			"head_signature": record.HeadSignature,
			"change":         record.Change,
		})
	}
	return out
}

func coveragePackagesToInterfaces(packages []coveragePackageRecord) []interface{} {
	out := make([]interface{}, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, map[string]interface{}{
			"package":  pkg.Package,
			"coverage": pkg.Coverage,
		})
	}
	return out
}

func nodeSourcePath(env agentenv.WorkspaceEnvironment, node *frameworkast.Node) string {
	if node == nil {
		return ""
	}
	if env.IndexManager == nil {
		return strings.TrimSpace(node.FileID)
	}
	store := env.IndexManager.Store()
	if store == nil {
		return strings.TrimSpace(node.FileID)
	}
	if meta, err := store.GetFile(node.FileID); err == nil && meta != nil && strings.TrimSpace(meta.Path) != "" {
		return meta.Path
	}
	if meta, err := store.GetFileByPath(node.FileID); err == nil && meta != nil && strings.TrimSpace(meta.Path) != "" {
		return meta.Path
	}
	return strings.TrimSpace(node.FileID)
}

func packageLayerForPath(workspace, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	root := strings.TrimSpace(workspace)
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}
	path = filepath.ToSlash(filepath.Clean(path))
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
