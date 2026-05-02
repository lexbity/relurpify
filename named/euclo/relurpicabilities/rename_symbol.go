package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	frameworkast "codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type RenameSymbolHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

func NewRenameSymbolHandler(env agentenv.WorkspaceEnvironment) *RenameSymbolHandler {
	return &RenameSymbolHandler{env: env}
}

func (h *RenameSymbolHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.rename_symbol",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Rename Symbol",
		Version:       "1.0.0",
		Description:   "Renames a symbol across the workspace using AST-bounded text replacement",
		Category:      "refactor_patch",
		Tags:          []string{"refactor", "rename", "ast", "write"},
		Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassDestructive},
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"from":    {Type: "string"},
				"to":      {Type: "string"},
				"file":    {Type: "string"},
				"preview": {Type: "boolean"},
			},
			Required: []string{"from", "to"},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success":        {Type: "boolean"},
				"preview":        {Type: "boolean"},
				"applied":        {Type: "boolean"},
				"files_modified": {Type: "array", Items: &core.Schema{Type: "object"}},
				"replacements":   {Type: "integer"},
			},
		},
	}
}

func (h *RenameSymbolHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	from, ok := stringArg(args, "from")
	if !ok || strings.TrimSpace(from) == "" {
		return failResult("from argument is required"), fmt.Errorf("from argument is required")
	}
	to, ok := stringArg(args, "to")
	if !ok || strings.TrimSpace(to) == "" {
		return failResult("to argument is required"), fmt.Errorf("to argument is required")
	}
	fileHint, _ := stringArg(args, "file")
	preview, _ := args["preview"].(bool)

	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), fmt.Errorf("index manager not available")
	}

	nodes, err := h.env.IndexManager.QuerySymbol(from)
	if err != nil {
		return failResult(fmt.Sprintf("symbol lookup failed: %v", err)), err
	}
	if len(nodes) == 0 {
		return failResult(fmt.Sprintf("symbol not found: %s", from)), fmt.Errorf("symbol not found: %s", from)
	}

	byFile := make(map[string][]*frameworkast.Node)
	hint, err := h.normalizedFileHint(fileHint)
	if err != nil {
		return failResult(err.Error()), err
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		path := nodeSourcePath(h.env, node)
		if path == "" {
			continue
		}
		if hint != "" && path != hint && node.FileID != hint {
			continue
		}
		byFile[path] = append(byFile[path], node)
	}
	if len(byFile) == 0 {
		return failResult("no matching symbol instances found"), fmt.Errorf("no matching symbol instances found")
	}

	paths := make([]string, 0, len(byFile))
	for path := range byFile {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if !preview {
		for _, path := range paths {
			if err := h.authorizeFileWrite(ctx, h.env, path); err != nil {
				return failResult(fmt.Sprintf("write denied: %v", err)), err
			}
		}
	}

	modified := make([]interface{}, 0, len(paths))
	total := 0
	previewFiles := make(map[string]string, len(paths))
	for _, path := range paths {
		content, resolvedPath, err := h.readWorkspaceFile(h.env, path)
		if err != nil {
			return failResult(fmt.Sprintf("read source file failed: %v", err)), err
		}
		updated, replacements, err := renameSymbolInContent(string(content), byFile[path], from, to)
		if err != nil {
			return failResult(err.Error()), err
		}
		total += replacements
		modified = append(modified, map[string]interface{}{
			"file":         resolvedPath,
			"replacements": replacements,
		})
		if preview {
			previewFiles[resolvedPath] = updated
			continue
		}
		if _, err := h.writeWorkspaceFile(h.env, resolvedPath, []byte(updated), 0o644); err != nil {
			return failResult(fmt.Sprintf("write source file failed: %v", err)), err
		}
		if h.env.IndexManager != nil {
			_ = h.env.IndexManager.RefreshFiles([]string{resolvedPath})
		}
	}

	result := map[string]interface{}{
		"success":        true,
		"from":           from,
		"to":             to,
		"preview":        preview,
		"applied":        !preview,
		"files_modified": modified,
		"replacements":   total,
	}
	if preview {
		result["updated_files"] = previewFiles
	}
	return &contracts.CapabilityExecutionResult{Success: true, Data: result}, nil
}

func (h *RenameSymbolHandler) normalizedFileHint(fileHint string) (string, error) {
	hint := strings.TrimSpace(fileHint)
	if hint == "" {
		return "", nil
	}
	return h.resolveWorkspacePath(h.env, hint)
}

func renameSymbolInContent(content string, nodes []*frameworkast.Node, from, to string) (string, int, error) {
	if len(nodes) == 0 {
		return content, 0, fmt.Errorf("no matching nodes to rename")
	}
	ordered := append([]*frameworkast.Node(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].StartLine == ordered[j].StartLine {
			return ordered[i].EndLine > ordered[j].EndLine
		}
		return ordered[i].StartLine > ordered[j].StartLine
	})
	updated := content
	total := 0
	for _, node := range ordered {
		if node == nil || node.StartLine <= 0 || node.EndLine < node.StartLine {
			continue
		}
		span, err := extractLines(updated, node.StartLine, node.EndLine)
		if err != nil {
			return "", 0, err
		}
		count := strings.Count(span, from)
		if count == 0 {
			continue
		}
		span = strings.ReplaceAll(span, from, to)
		updated, err = replaceLines(updated, node.StartLine, node.EndLine, span)
		if err != nil {
			return "", 0, err
		}
		total += count
	}
	return updated, total, nil
}
