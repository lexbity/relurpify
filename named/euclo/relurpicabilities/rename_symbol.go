package relurpicabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	frameworkast "codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

type RenameSymbolHandler struct {
	querier   SymbolQuerier
	store     EdgeStore
	files     WorkspaceFiles
	refresher IndexRefresher
}

func NewRenameSymbolHandler(querier SymbolQuerier, store EdgeStore, files WorkspaceFiles, refresher IndexRefresher) *RenameSymbolHandler {
	return &RenameSymbolHandler{querier: querier, store: store, files: files, refresher: refresher}
}

func (h *RenameSymbolHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.rename_symbol",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Rename Symbol",
		Version:       "1.0.0",
		Description:   "Renames a symbol across the workspace using AST-bounded text replacement",
		Category:      "refactor_patch",
		Tags:          []string{"refactor", "rename", "ast", "write"},
		Source:        descriptor.CapabilitySource{Scope: classification.CapabilityScopeBuiltin},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"from":    {Type: "string"},
				"to":      {Type: "string"},
				"file":    {Type: "string"},
				"preview": {Type: "boolean"},
			},
			Required: []string{"from", "to"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success":        {Type: "boolean"},
				"preview":        {Type: "boolean"},
				"applied":        {Type: "boolean"},
				"files_modified": {Type: "array", Items: &schemacoerce.Schema{Type: "object"}},
				"replacements":   {Type: "integer"},
			},
		},
	}
}

func (h *RenameSymbolHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
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

	if h.querier == nil {
		return failResult("symbol service not available"), fmt.Errorf("symbol service not available")
	}

	nodes, err := h.querier.QuerySymbol(from)
	if err != nil {
		return failResult(fmt.Sprintf("symbol lookup failed: %v", err)), err
	}
	if len(nodes) == 0 {
		return failResult(fmt.Sprintf("symbol not found: %s", from)), fmt.Errorf("symbol not found: %s", from)
	}

	byFile := make(map[string][]*frameworkast.Node)
	hint := strings.TrimSpace(fileHint)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		path := filePathFromNode(h.store, node)
		if path == "" {
			continue
		}
		if hint != "" && path != hint {
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

	modified := make([]any, 0, len(paths))
	total := 0
	previewFiles := make(map[string]string, len(paths))
	for _, path := range paths {
		content, resolvedPath, err := h.files.Read(path)
		if err != nil {
			return failResult(fmt.Sprintf("read source file failed: %v", err)), err
		}
		updated, replacements, err := renameSymbolInContent(string(content), byFile[path], from, to)
		if err != nil {
			return failResult(err.Error()), err
		}
		total += replacements
		modified = append(modified, map[string]any{
			"file":         resolvedPath,
			"replacements": replacements,
		})
		if preview {
			previewFiles[resolvedPath] = updated
			continue
		}
		if _, err := h.files.Write(resolvedPath, []byte(updated), 0o644); err != nil {
			return failResult(fmt.Sprintf("write source file failed: %v", err)), err
		}
		if h.refresher != nil {
			_ = h.refresher.RefreshFiles(ctx, []string{resolvedPath})
		}
	}

	result := map[string]any{
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
	return &ports.ToolResult{Success: true, Data: result}, nil
}

func filePathFromNode(store EdgeStore, node *frameworkast.Node) string {
	if node == nil {
		return ""
	}
	if store != nil {
		if meta, err := store.GetFile(node.FileID); err == nil && meta != nil && meta.Path != "" {
			return meta.Path
		}
	}
	return node.FileID
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
