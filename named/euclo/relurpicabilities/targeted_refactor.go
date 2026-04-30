package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// TargetedRefactorHandler implements the targeted refactor capability.
type TargetedRefactorHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

// NewTargetedRefactorHandler creates a new targeted refactor handler.
func NewTargetedRefactorHandler(env agentenv.WorkspaceEnvironment) *TargetedRefactorHandler {
	return &TargetedRefactorHandler{env: env}
}

// Descriptor returns the capability descriptor for the targeted refactor handler.
func (h *TargetedRefactorHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.targeted_refactor",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Targeted Refactor",
		Version:       "1.0.0",
		Description:   "Applies a focused refactoring to a specific symbol or code block via AST-bounded text replacement",
		Category:      "refactor_patch",
		Tags:          []string{"refactor", "ast", "write"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassDestructive},
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
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
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
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
func (h *TargetedRefactorHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
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

	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), nil
	}

	nodes, err := h.env.IndexManager.QuerySymbol(symbol)
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

	content, resolvedSourcePath, err := h.readWorkspaceFile(h.env, sourcePath)
	if err != nil {
		return failResult(fmt.Sprintf("read source file failed: %v", err)), nil
	}

	if replacement == "" {
		if h.env.Model == nil {
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

	result := map[string]interface{}{
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
		return &core.CapabilityExecutionResult{Success: true, Data: result}, nil
	}

	if err := h.authorizeFileWrite(ctx, h.env, resolvedSourcePath); err != nil {
		return failResult(fmt.Sprintf("write denied: %v", err)), err
	}
	if _, err := h.writeWorkspaceFile(h.env, resolvedSourcePath, []byte(newContent), 0o644); err != nil {
		return failResult(fmt.Sprintf("write source file failed: %v", err)), err
	}
	if h.env.IndexManager != nil {
		_ = h.env.IndexManager.RefreshFiles([]string{resolvedSourcePath})
	}
	result["applied"] = true
	return &core.CapabilityExecutionResult{Success: true, Data: result}, nil
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
	if h.env.IndexManager == nil || fileHint == "" {
		return "", false
	}
	store := h.env.IndexManager.Store()
	if store == nil {
		return "", false
	}
	if meta, err := store.GetFileByPath(fileHint); err == nil && meta != nil {
		return meta.ID, true
	}
	if meta, err := store.GetFile(fileHint); err == nil && meta != nil {
		return meta.ID, true
	}
	return "", false
}

func (h *TargetedRefactorHandler) resolveTargetSource(target *ast.Node, fileHint string) (string, string, error) {
	if target == nil {
		return "", "", fmt.Errorf("target node is required")
	}
	store := h.env.IndexManager.Store()
	if store != nil {
		if meta, err := store.GetFile(target.FileID); err == nil && meta != nil && meta.Path != "" {
			if fileContent, resolvedPath, err := h.readWorkspaceFile(h.env, meta.Path); err == nil {
				if selected, err := extractLines(string(fileContent), target.StartLine, target.EndLine); err == nil {
					return resolvedPath, selected, nil
				}
			}
			// Fall through to using the hint if the stored path is unavailable.
		}
	}
	if fileHint != "" {
		if fileContent, resolvedPath, err := h.readWorkspaceFile(h.env, fileHint); err == nil {
			if selected, err := extractLines(string(fileContent), target.StartLine, target.EndLine); err == nil {
				return resolvedPath, selected, nil
			}
		}
	}
	if target.FileID != "" {
		if fileContent, resolvedPath, err := h.readWorkspaceFile(h.env, target.FileID); err == nil {
			if selected, err := extractLines(string(fileContent), target.StartLine, target.EndLine); err == nil {
				return resolvedPath, selected, nil
			}
		}
	}
	return "", "", fmt.Errorf("unable to resolve source path for symbol %q", target.Name)
}

func (h *TargetedRefactorHandler) generateReplacement(ctx context.Context, target *ast.Node, sourcePath, original, transformation string) (string, error) {
	if h.env.Model == nil {
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
	resp, err := h.env.Model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       configuredModelName(h.env.Config),
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
