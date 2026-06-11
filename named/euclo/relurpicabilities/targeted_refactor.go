package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/model"
)

// TargetedRefactorHandler implements the targeted refactor capability.
type TargetedRefactorHandler struct {
	querier   SymbolQuerier
	store     EdgeStore
	files     WorkspaceFiles
	refresher IndexRefresher
	gen       model.LanguageModel
}

// NewTargetedRefactorHandler creates a new targeted refactor handler.
func NewTargetedRefactorHandler(querier SymbolQuerier, store EdgeStore, files WorkspaceFiles, refresher IndexRefresher, gen model.LanguageModel) *TargetedRefactorHandler {
	return &TargetedRefactorHandler{querier: querier, store: store, files: files, refresher: refresher, gen: gen}
}

// Descriptor returns the capability descriptor for the targeted refactor handler.
func (h *TargetedRefactorHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.targeted_refactor",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Targeted Refactor",
		Version:       "1.0.0",
		Description:   "Applies a focused refactoring to a specific symbol or code block via AST-bounded text replacement",
		Category:      "refactor_patch",
		Tags:          []string{"refactor", "ast", "write"},
		Source: descriptor.CapabilitySource{
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"symbol": {
					Type:        "string",
					Description: "Symbol name to refactor",
				},
				"file": {
					Type:        "string",
					Description: "File path hint for disambiguation",
				},
				"transformation": {
					Type:        "string",
					Description: "Description of the transformation to apply",
				},
				"replacement": {
					Type:        "string",
					Description: "Explicit replacement text for the selected symbol block",
				},
				"preview": {
					Type:        "boolean",
					Description: "Return the proposed change without writing (default: false)",
				},
			},
			Required: []string{"symbol", "transformation"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if refactor applied",
				},
				"file": {
					Type:        "string",
					Description: "File that was modified",
				},
				"symbol": {
					Type:        "string",
					Description: "Symbol that was refactored",
				},
				"preview": {
					Type:        "boolean",
					Description: "True if this is a preview only",
				},
				"applied": {
					Type:        "boolean",
					Description: "True if the write was applied to disk",
				},
				"before": {
					Type:        "string",
					Description: "Original text selected for refactoring",
				},
				"after": {
					Type:        "string",
					Description: "Replacement text selected for refactoring",
				},
			},
		},
	}
}

// Invoke locates the target symbol and applies the transformation.
func (h *TargetedRefactorHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	symbol, ok := stringArg(args, "symbol")
	if !ok || symbol == "" {
		return failResult("symbol argument is required"), nil
	}
	transformation, ok := stringArg(args, "transformation")
	if !ok || transformation == "" {
		return failResult("transformation argument is required"), nil
	}
	file, _ := stringArg(args, "file")
	replacement, _ := stringArg(args, "replacement")
	preview, _ := args["preview"].(bool)

	if h.querier == nil {
		return failResult("symbol service not available"), nil
	}

	nodes, err := h.querier.QuerySymbol(symbol)
	if err != nil {
		return failResult(fmt.Sprintf("symbol lookup failed: %v", err)), nil
	}
	if len(nodes) == 0 {
		return failResult(fmt.Sprintf("symbol not found: %s", symbol)), nil
	}

	target, err := h.selectTargetNode(nodes, file)
	if err != nil {
		return failResult(err.Error()), nil
	}

	sourcePath, original, err := h.resolveTargetSource(target, file)
	if err != nil {
		return failResult(err.Error()), nil
	}

	content, resolvedSourcePath, err := h.files.Read(sourcePath)
	if err != nil {
		return failResult(fmt.Sprintf("read source file failed: %v", err)), nil
	}

	if replacement == "" {
		if h.gen == nil {
			return failResult("replacement text required when no model is available"), nil
		}
		replacement, err = h.generateReplacement(ctx, target, resolvedSourcePath, original, transformation)
		if err != nil {
			return failResult(fmt.Sprintf("generate replacement failed: %v", err)), nil
		}
	}

	newContent, err := replaceLines(string(content), target.StartLine, target.EndLine, replacement)
	if err != nil {
		return failResult(err.Error()), nil
	}

	result := map[string]any{
		"success":        true,
		"file":           resolvedSourcePath,
		"symbol":         symbol,
		"start_line":     target.StartLine,
		"end_line":       target.EndLine,
		"transformation": transformation,
		"preview":        preview,
		"applied":        false,
		"before":         original,
		"after":          replacement,
	}

	if preview {
		result["updated_content"] = newContent
		return &ports.ToolResult{Success: true, Data: result}, nil
	}

	if _, err := h.files.Write(resolvedSourcePath, []byte(newContent), 0o644); err != nil {
		return failResult(fmt.Sprintf("write source file failed: %v", err)), err
	}
	if h.refresher != nil {
		_ = h.refresher.RefreshFiles(ctx, []string{resolvedSourcePath})
	}
	result["applied"] = true
	return &ports.ToolResult{Success: true, Data: result}, nil
}

type targetedRefactorProposal struct {
	Replacement string `json:"replacement"`
	Summary     string `json:"summary,omitempty"`
}

func (h *TargetedRefactorHandler) selectTargetNode(nodes []*ast.Node, fileHint string) (*ast.Node, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("symbol not found")
	}
	filtered := nodes
	if fileHint != "" {
		if fileID, ok := h.resolveFileID(fileHint); ok {
			filtered = filtered[:0]
			for _, node := range nodes {
				if node != nil && node.FileID == fileID {
					filtered = append(filtered, node)
				}
			}
		} else {
			var exact []*ast.Node
			for _, node := range nodes {
				if node != nil && node.FileID == fileHint {
					exact = append(exact, node)
				}
			}
			if len(exact) > 0 {
				filtered = exact
			}
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("symbol %q not found in file %q", nodes[0].Name, fileHint)
	}
	if fileHint == "" {
		fileSet := make(map[string]struct{}, len(filtered))
		for _, node := range filtered {
			if node == nil {
				continue
			}
			fileSet[node.FileID] = struct{}{}
		}
		if len(fileSet) > 1 {
			return nil, fmt.Errorf("symbol %q is ambiguous across %d files; provide a file hint", filtered[0].Name, len(fileSet))
		}
	}
	if len(filtered) == 1 {
		return filtered[0], nil
	}
	best := filtered[0]
	for _, node := range filtered[1:] {
		if node == nil {
			continue
		}
		if best == nil || spanWidth(node) < spanWidth(best) || (spanWidth(node) == spanWidth(best) && node.StartLine < best.StartLine) {
			best = node
		}
	}
	if best == nil {
		return nil, fmt.Errorf("unable to resolve target node for refactor")
	}
	return best, nil
}

func (h *TargetedRefactorHandler) resolveFileID(fileHint string) (string, bool) {
	if h.store == nil || fileHint == "" {
		return "", false
	}
	if meta, err := h.store.GetFileByPath(fileHint); err == nil && meta != nil {
		return meta.ID, true
	}
	if meta, err := h.store.GetFile(fileHint); err == nil && meta != nil {
		return meta.ID, true
	}
	return "", false
}

func (h *TargetedRefactorHandler) resolveTargetSource(target *ast.Node, fileHint string) (string, string, error) {
	if target == nil {
		return "", "", fmt.Errorf("target node is required")
	}
	// Resolve FileID to a filesystem path through the store
	if h.store != nil {
		if meta, err := h.store.GetFile(target.FileID); err == nil && meta != nil && meta.Path != "" {
			content, resolvedPath, err := h.files.Read(meta.Path)
			if err == nil {
				selected, err := extractLines(string(content), target.StartLine, target.EndLine)
				if err == nil {
					return resolvedPath, selected, nil
				}
			}
		}
	}
	if fileHint != "" {
		content, resolvedPath, err := h.files.Read(fileHint)
		if err == nil {
			selected, err := extractLines(string(content), target.StartLine, target.EndLine)
			if err == nil {
				return resolvedPath, selected, nil
			}
		}
	}
	content, resolvedPath, err := h.files.Read(target.FileID)
	if err == nil {
		selected, err := extractLines(string(content), target.StartLine, target.EndLine)
		if err == nil {
			return resolvedPath, selected, nil
		}
	}
	return "", "", fmt.Errorf("unable to resolve source path for symbol %q", target.Name)
}

func (h *TargetedRefactorHandler) generateReplacement(ctx context.Context, target *ast.Node, sourcePath, original, transformation string) (string, error) {
	if h.gen == nil {
		return "", fmt.Errorf("model unavailable")
	}
	prompt := fmt.Sprintf(`You are editing a single symbol block in %s.
Symbol: %s
Kind: %s
Lines: %d-%d
Requested transformation: %s

Original block:
%s

Return ONLY valid JSON with this shape:
{"replacement":"full replacement block text","summary":"short explanation"}
Do not include markdown fences. Do not edit outside the selected block.`,
		sourcePath, target.Name, target.Type, target.StartLine, target.EndLine, transformation, original)
	resp, err := h.gen.Generate(ctx, prompt, &model.LLMOptions{
		Temperature: 0,
		MaxTokens:   800,
	})
	if err != nil {
		return "", err
	}
	var proposal targetedRefactorProposal
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(resp.Text)), &proposal); err != nil {
		return "", err
	}
	replacement := strings.TrimSpace(proposal.Replacement)
	if replacement == "" {
		return "", fmt.Errorf("model returned empty replacement")
	}
	return replacement, nil
}

func extractLines(content string, startLine, endLine int) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "", fmt.Errorf("empty file content")
	}
	if startLine < 1 || endLine < startLine {
		return "", fmt.Errorf("invalid line range %d-%d", startLine, endLine)
	}
	if startLine > len(lines) || endLine > len(lines) {
		return "", fmt.Errorf("line range %d-%d exceeds file length %d", startLine, endLine, len(lines))
	}
	return strings.Join(lines[startLine-1:endLine], ""), nil
}

func replaceLines(content string, startLine, endLine int, replacement string) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "", fmt.Errorf("empty file content")
	}
	if startLine < 1 || endLine < startLine {
		return "", fmt.Errorf("invalid line range %d-%d", startLine, endLine)
	}
	if startLine > len(lines) || endLine > len(lines) {
		return "", fmt.Errorf("line range %d-%d exceeds file length %d", startLine, endLine, len(lines))
	}
	if replacement != "" && !strings.HasSuffix(replacement, "\n") && endLine < len(lines) {
		replacement += "\n"
	}
	prefix := strings.Join(lines[:startLine-1], "")
	suffix := strings.Join(lines[endLine:], "")
	return prefix + replacement + suffix, nil
}

func spanWidth(node *ast.Node) int {
	if node == nil {
		return int(^uint(0) >> 1)
	}
	if node.EndLine <= 0 || node.StartLine <= 0 || node.EndLine < node.StartLine {
		return int(^uint(0) >> 1)
	}
	return node.EndLine - node.StartLine
}
