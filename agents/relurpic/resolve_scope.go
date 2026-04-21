package relurpic

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

type ResolutionError struct {
	Scope      string
	Reason     string
	Candidates []string
}

func (e *ResolutionError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("resolve scope %q: %s", e.Scope, e.Reason)
	}
	return fmt.Sprintf("resolve scope %q: %s (%s)", e.Scope, e.Reason, strings.Join(e.Candidates, ", "))
}

func resolveSymbolScope(ctx context.Context, scope string, index *ast.IndexManager, registry *capability.Registry) (resolvedSymbolScope, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return resolvedSymbolScope{}, fmt.Errorf("scope required")
	}
	if resolved, ok := resolveFileScope(ctx, scope, index, registry); ok {
		return resolved, nil
	}
	if index == nil {
		return resolvedSymbolScope{}, fmt.Errorf("scope %q requires an index manager or an existing file path", scope)
	}
	if resolved, ok, err := resolvePackageScope(ctx, scope, index, registry); err != nil {
		return resolvedSymbolScope{}, err
	} else if ok {
		return resolved, nil
	}
	return resolveNamedSymbolScope(ctx, scope, index, registry)
}

func resolveFileScope(ctx context.Context, path string, index *ast.IndexManager, registry *capability.Registry) (resolvedSymbolScope, bool) {
	excerpt, err := excerptForFile(ctx, registry, path)
	if err != nil {
		return resolvedSymbolScope{}, false
	}
	resolved := resolvedSymbolScope{
		Input:     path,
		FilePaths: []string{path},
		Excerpts:  []resolvedExcerpt{excerpt},
	}
	if index == nil {
		return resolved, true
	}
	fileMeta, err := index.Store().GetFileByPath(path)
	if err != nil || fileMeta == nil {
		return resolved, true
	}
	nodes, err := index.Store().GetNodesByFile(fileMeta.ID)
	if err != nil {
		return resolved, true
	}
	resolved.SymbolIDs = appendNodeIDs(nil, nodes)
	return resolved, true
}

func resolvePackageScope(ctx context.Context, scope string, index *ast.IndexManager, registry *capability.Registry) (resolvedSymbolScope, bool, error) {
	files, err := index.Store().ListFiles("")
	if err != nil {
		return resolvedSymbolScope{}, false, err
	}
	scope = filepath.ToSlash(strings.Trim(scope, "/"))
	matched := make([]*ast.FileMetadata, 0)
	for _, file := range files {
		if file == nil {
			continue
		}
		rel := filepath.ToSlash(strings.Trim(file.RelativePath, "/"))
		dir := filepath.ToSlash(strings.Trim(filepath.Dir(rel), "/"))
		if rel == scope || dir == scope || filepath.Base(dir) == scope {
			matched = append(matched, file)
		}
	}
	if len(matched) == 0 {
		return resolvedSymbolScope{}, false, nil
	}
	filePaths := make([]string, 0, len(matched))
	symbolIDs := make([]string, 0)
	excerpts := make([]resolvedExcerpt, 0, len(matched))
	for _, file := range matched {
		filePaths = append(filePaths, file.Path)
		nodes, err := index.Store().GetNodesByFile(file.ID)
		if err == nil {
			symbolIDs = appendNodeIDs(symbolIDs, nodes)
		}
		excerpt, err := excerptForFile(ctx, registry, file.Path)
		if err == nil {
			excerpts = append(excerpts, excerpt)
		}
	}
	sort.Strings(filePaths)
	return resolvedSymbolScope{
		Input:     scope,
		FilePaths: filePaths,
		SymbolIDs: symbolIDs,
		Excerpts:  excerpts,
	}, true, nil
}

func resolveNamedSymbolScope(ctx context.Context, scope string, index *ast.IndexManager, registry *capability.Registry) (resolvedSymbolScope, error) {
	nodes, err := index.SearchNodes(ast.NodeQuery{NamePattern: scope, Limit: 64})
	if err != nil {
		return resolvedSymbolScope{}, err
	}
	if len(nodes) == 0 {
		return resolvedSymbolScope{}, fmt.Errorf("no symbols found for scope %q", scope)
	}

	nameMatches := make([]*ast.Node, 0, len(nodes))
	candidateSet := make(map[string]struct{})
	for _, node := range nodes {
		if node == nil {
			continue
		}
		candidateSet[node.Name] = struct{}{}
		if strings.EqualFold(strings.TrimSpace(node.Name), scope) {
			nameMatches = append(nameMatches, node)
		}
	}
	if len(nameMatches) == 0 {
		nameMatches = nodes
	}

	fileSet := make(map[string]struct{})
	for _, node := range nameMatches {
		if node == nil || node.FileID == "" {
			continue
		}
		fileMeta, err := index.Store().GetFile(node.FileID)
		if err != nil || fileMeta == nil {
			continue
		}
		fileSet[fileMeta.Path] = struct{}{}
	}
	if len(fileSet) > 1 {
		return resolvedSymbolScope{}, &ResolutionError{
			Scope:      scope,
			Reason:     "ambiguous symbol scope",
			Candidates: sortedKeys(fileSet),
		}
	}

	excerpts := make([]resolvedExcerpt, 0, len(nameMatches))
	symbolSet := make(map[string]struct{})
	for _, node := range nameMatches {
		if node == nil || node.FileID == "" {
			continue
		}
		fileMeta, err := index.Store().GetFile(node.FileID)
		if err != nil || fileMeta == nil {
			continue
		}
		symbolSet[node.ID] = struct{}{}
		excerpt, err := excerptForLines(ctx, registry, fileMeta.Path, node.StartLine, node.EndLine)
		if err != nil {
			continue
		}
		excerpts = append(excerpts, excerpt)
	}
	if len(excerpts) == 0 && len(fileSet) == 0 {
		return resolvedSymbolScope{}, fmt.Errorf("no file content found for scope %q", scope)
	}
	return resolvedSymbolScope{
		Input:     scope,
		FilePaths: sortedKeys(fileSet),
		SymbolIDs: sortedKeys(symbolSet),
		Excerpts:  excerpts,
	}, nil
}
